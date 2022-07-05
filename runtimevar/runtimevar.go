// Copyright 2018 The Go Cloud Development Kit Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package runtimevar provides an easy and portable way to watch runtime
// configuration variables. Subpackages contain driver implementations of
// runtimevar for supported services.
//
// See https://gocloud.dev/howto/runtimevar/ for a detailed how-to guide.
//
// # OpenCensus Integration
//
// OpenCensus supports tracing and metric collection for multiple languages and
// backend providers. See https://opencensus.io.
//
// This API collects an OpenCensus metric "gocloud.dev/runtimevar/value_changes",
// a count of the number of times all variables have changed values, by driver.
//
// To enable metric collection in your application, see "Exporting stats" at
// https://opencensus.io/quickstart/go/metrics.
package runtimevar // import "gocloud.dev/runtimevar"

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/internal/oc"
	"gocloud.dev/internal/openurl"
	"gocloud.dev/runtimevar/driver"
	"gocloud.dev/secrets"
)

// Snapshot contains a snapshot of a variable's value and metadata about it.
// It is intended to be read-only for users.
type Snapshot struct {
	// Value contains the value of the variable.
	// The type for Value depends on the decoder used when creating the Variable.
	Value interface{}

	// UpdateTime is the time when the last change was detected.
	UpdateTime time.Time

	asFunc func(interface{}) bool
}

// As converts i to driver-specific types.
// See https://gocloud.dev/concepts/as/ for background information, the "As"
// examples in this package for examples, and the driver package
// documentation for the specific types supported for that driver.
func (s *Snapshot) As(i interface{}) bool {
	if s.asFunc == nil {
		return false
	}
	return s.asFunc(i)
}

const pkgName = "gocloud.dev/runtimevar"

var (
	changeMeasure = stats.Int64(pkgName+"/value_changes", "Count of variable value changes",
		stats.UnitDimensionless)
	// OpenCensusViews are predefined views for OpenCensus metrics.
	OpenCensusViews = []*view.View{
		{
			Name:        pkgName + "/value_changes",
			Measure:     changeMeasure,
			Description: "Count of variable value changes by driver.",
			TagKeys:     []tag.Key{oc.ProviderKey},
			Aggregation: view.Count(),
		},
	}
)

// Variable provides an easy and portable way to watch runtime configuration
// variables. To create a Variable, use constructors found in driver subpackages.
type Variable struct {
	dw       driver.Watcher
	provider string // for metric collection; refers to driver package name

	// For cancelling the background goroutine, and noticing when it has exited.
	backgroundCancel context.CancelFunc
	backgroundDone   chan struct{}

	// haveGoodCh is closed when we get the first good value for the variable.
	haveGoodCh chan struct{}
	// A reference to changed at the last time Watch was called.
	// Not protected by mu because it's only referenced in Watch, which is not
	// supposed to be called from multiple goroutines.
	lastWatch <-chan struct{}

	mu       sync.RWMutex
	changed  chan struct{} // closed when changing any of the other variables and replaced with a new channel
	last     Snapshot
	lastErr  error
	lastGood Snapshot
}

// New is intended for use by drivers only. Do not use in application code.
var New = newVar

// newVar creates a new *Variable based on a specific driver implementation.
func newVar(w driver.Watcher) *Variable {
	ctx, cancel := context.WithCancel(context.Background())
	changed := make(chan struct{})
	v := &Variable{
		dw:               w,
		provider:         oc.ProviderName(w),
		backgroundCancel: cancel,
		backgroundDone:   make(chan struct{}),
		haveGoodCh:       make(chan struct{}),
		changed:          changed,
		lastWatch:        changed,
		lastErr:          gcerr.Newf(gcerr.FailedPrecondition, nil, "no value yet"),
	}
	go v.background(ctx)
	return v
}

// ErrClosed is returned from Watch when the Variable has been Closed.
var ErrClosed = gcerr.Newf(gcerr.FailedPrecondition, nil, "Variable has been Closed")

// Watch returns when there is a new Snapshot of the current value of the
// variable.
//
// The first call to Watch will block while reading the variable from the
// driver, and will return the resulting Snapshot or error. If an error is
// returned, the returned Snapshot is a zero value and should be ignored.
// Subsequent calls will block until the variable's value changes or a different
// error occurs.
//
// Watch returns an ErrClosed error if the Variable has been closed.
//
// Watch should not be called on the same variable from multiple goroutines
// concurrently. The typical use case is to call it in a single goroutine in a
// loop.
//
// If the variable does not exist, Watch returns an error for which
// gcerrors.Code will return gcerrors.NotFound.
//
// Alternatively, use Latest to retrieve the latest good value.
func (c *Variable) Watch(ctx context.Context) (Snapshot, error) {
	// Block until there's a change since the last Watch call, signaled
	// by lastWatch being closed by the background goroutine.
	var ctxErr error
	select {
	case <-c.lastWatch:
	case <-ctx.Done():
		ctxErr = ctx.Err()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastErr == ErrClosed {
		return Snapshot{}, ErrClosed
	} else if ctxErr != nil {
		return Snapshot{}, ctxErr
	}
	c.lastWatch = c.changed
	return c.last, c.lastErr
}

func (c *Variable) background(ctx context.Context) {
	var curState, prevState driver.State
	var wait time.Duration
	for {
		select {
		case <-ctx.Done():
			// We're shutting down; exit the goroutine.
			close(c.backgroundDone)
			return
		case <-time.After(wait):
			// Continue.
		}

		curState, wait = c.dw.WatchVariable(ctx, prevState)
		if curState == nil {
			// No change.
			continue
		}

		// There's something new to return!
		prevState = curState
		_ = stats.RecordWithTags(ctx, []tag.Mutator{tag.Upsert(oc.ProviderKey, c.provider)}, changeMeasure.M(1))
		// Error from RecordWithTags is not possible.

		// Updates under the lock.
		c.mu.Lock()
		if c.lastErr == ErrClosed {
			close(c.backgroundDone)
			c.mu.Unlock()
			return
		}
		if val, err := curState.Value(); err == nil {
			// We got a good value!
			c.last = Snapshot{
				Value:      val,
				UpdateTime: curState.UpdateTime(),
				asFunc:     curState.As,
			}
			c.lastErr = nil
			c.lastGood = c.last
			// Close c.haveGoodCh if it's not already closed.
			select {
			case <-c.haveGoodCh:
			default:
				close(c.haveGoodCh)
			}
		} else {
			// We got an error value.
			c.last = Snapshot{}
			c.lastErr = wrapError(c.dw, err)
		}
		close(c.changed)
		c.changed = make(chan struct{})
		c.mu.Unlock()
	}
}

func (c *Variable) haveGood() bool {
	select {
	case <-c.haveGoodCh:
		return true
	default:
		return false
	}
}

// Latest is intended to be called per request, with the request context.
// It returns the latest good Snapshot of the variable value, blocking if no
// good value has ever been received. If ctx is Done, it returns the latest
// error indicating why no good value is available (not the ctx.Err()).
// You can pass an already-Done ctx to make Latest not block.
//
// Latest returns ErrClosed if the Variable has been closed.
func (c *Variable) Latest(ctx context.Context) (Snapshot, error) {
	haveGood := c.haveGood()
	if !haveGood {
		select {
		case <-c.haveGoodCh:
			haveGood = true
		case <-ctx.Done():
			// We don't return ctx.Err().
		}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if haveGood && c.lastErr != ErrClosed {
		return c.lastGood, nil
	}
	return Snapshot{}, c.lastErr
}

// CheckHealth returns an error unless Latest will return a good value
// without blocking.
func (c *Variable) CheckHealth() error {
	haveGood := c.haveGood()
	c.mu.RLock()
	defer c.mu.RUnlock()
	if haveGood && c.lastErr != ErrClosed {
		return nil
	}
	return c.lastErr
}

// Close closes the Variable. The Variable is unusable after Close returns.
func (c *Variable) Close() error {
	// Record that we're closing. Subsequent calls to Watch/Latest will return ErrClosed.
	c.mu.Lock()
	if c.lastErr == ErrClosed {
		c.mu.Unlock()
		return ErrClosed
	}
	c.last = Snapshot{}
	c.lastErr = ErrClosed

	// Close any remaining channels to wake up any callers that are waiting on them.
	close(c.changed)
	// If it's the first good value, close haveGoodCh so that Latest doesn't block.
	select {
	case <-c.haveGoodCh:
	default:
		close(c.haveGoodCh)
	}
	c.mu.Unlock()

	// Shut down the background goroutine.
	c.backgroundCancel()
	<-c.backgroundDone

	// Close the driver.
	err := c.dw.Close()
	return wrapError(c.dw, err)
}

func wrapError(w driver.Watcher, err error) error {
	if err == nil {
		return nil
	}
	if gcerr.DoNotWrap(err) {
		return err
	}
	return gcerr.New(w.ErrorCode(err), err, 2, "runtimevar")
}

// ErrorAs converts err to driver-specific types.
// ErrorAs panics if i is nil or not a pointer.
// ErrorAs returns false if err == nil.
// See https://gocloud.dev/concepts/as/ for background information.
func (c *Variable) ErrorAs(err error, i interface{}) bool {
	return gcerr.ErrorAs(err, i, c.dw.ErrorAs)
}

// VariableURLOpener represents types than can open Variables based on a URL.
// The opener must not modify the URL argument. OpenVariableURL must be safe to
// call from multiple goroutines.
//
// This interface is generally implemented by types in driver packages.
type VariableURLOpener interface {
	OpenVariableURL(ctx context.Context, u *url.URL) (*Variable, error)
}

// URLMux is a URL opener multiplexer. It matches the scheme of the URLs
// against a set of registered schemes and calls the opener that matches the
// URL's scheme.
// See https://gocloud.dev/concepts/urls/ for more information.
//
// The zero value is a multiplexer with no registered schemes.
type URLMux struct {
	schemes openurl.SchemeMap
}

// VariableSchemes returns a sorted slice of the registered Variable schemes.
func (mux *URLMux) VariableSchemes() []string { return mux.schemes.Schemes() }

// ValidVariableScheme returns true iff scheme has been registered for Variables.
func (mux *URLMux) ValidVariableScheme(scheme string) bool { return mux.schemes.ValidScheme(scheme) }

// RegisterVariable registers the opener with the given scheme. If an opener
// already exists for the scheme, RegisterVariable panics.
func (mux *URLMux) RegisterVariable(scheme string, opener VariableURLOpener) {
	mux.schemes.Register("runtimevar", "Variable", scheme, opener)
}

// OpenVariable calls OpenVariableURL with the URL parsed from urlstr.
// OpenVariable is safe to call from multiple goroutines.
func (mux *URLMux) OpenVariable(ctx context.Context, urlstr string) (*Variable, error) {
	opener, u, err := mux.schemes.FromString("Variable", urlstr)
	if err != nil {
		return nil, err
	}
	return opener.(VariableURLOpener).OpenVariableURL(ctx, u)
}

// OpenVariableURL dispatches the URL to the opener that is registered with the
// URL's scheme. OpenVariableURL is safe to call from multiple goroutines.
func (mux *URLMux) OpenVariableURL(ctx context.Context, u *url.URL) (*Variable, error) {
	opener, err := mux.schemes.FromURL("Variable", u)
	if err != nil {
		return nil, err
	}
	return opener.(VariableURLOpener).OpenVariableURL(ctx, u)
}

var defaultURLMux = new(URLMux)

// DefaultURLMux returns the URLMux used by OpenVariable.
//
// Driver packages can use this to register their VariableURLOpener on the mux.
func DefaultURLMux() *URLMux {
	return defaultURLMux
}

// OpenVariable opens the variable identified by the URL given.
// See the URLOpener documentation in driver subpackages for
// details on supported URL formats, and https://gocloud.dev/concepts/urls
// for more information.
func OpenVariable(ctx context.Context, urlstr string) (*Variable, error) {
	return defaultURLMux.OpenVariable(ctx, urlstr)
}

// Decode is a function type for unmarshaling/decoding a slice of bytes into
// an arbitrary type. Decode functions are used when creating a Decoder via
// NewDecoder. This package provides common Decode functions including
// GobDecode and JSONDecode.
type Decode func(context.Context, []byte, interface{}) error

// Decoder decodes a slice of bytes into a particular Go object.
//
// This package provides some common Decoders that you can use directly,
// including StringDecoder and BytesDecoder. You can also NewDecoder to
// construct other Decoders.
type Decoder struct {
	typ reflect.Type
	fn  Decode
}

// NewDecoder returns a Decoder that uses fn to decode a slice of bytes into
// an object of type obj.
//
// This package provides some common Decode functions, including JSONDecode
// and GobDecode, which can be passed to this function to create Decoders for
// JSON and gob values.
func NewDecoder(obj interface{}, fn Decode) *Decoder {
	return &Decoder{
		typ: reflect.TypeOf(obj),
		fn:  fn,
	}
}

// Decode decodes b into a new instance of the target type.
func (d *Decoder) Decode(ctx context.Context, b []byte) (interface{}, error) {
	nv := reflect.New(d.typ).Interface()
	if err := d.fn(ctx, b, nv); err != nil {
		return nil, err
	}
	ptr := reflect.ValueOf(nv)
	return ptr.Elem().Interface(), nil
}

var (
	// StringDecoder decodes into strings.
	StringDecoder = NewDecoder("", StringDecode)

	// BytesDecoder copies the slice of bytes.
	BytesDecoder = NewDecoder([]byte{}, BytesDecode)
)

// JSONDecode can be passed to NewDecoder when decoding JSON (https://golang.org/pkg/encoding/json/).
func JSONDecode(ctx context.Context, data []byte, obj interface{}) error {
	return json.Unmarshal(data, obj)
}

// GobDecode can be passed to NewDecoder when decoding gobs (https://golang.org/pkg/encoding/gob/).
func GobDecode(ctx context.Context, data []byte, obj interface{}) error {
	return gob.NewDecoder(bytes.NewBuffer(data)).Decode(obj)
}

// StringDecode decodes raw bytes b into a string.
func StringDecode(ctx context.Context, b []byte, obj interface{}) error {
	v := obj.(*string)
	*v = string(b)
	return nil
}

// BytesDecode copies the slice of bytes b into obj.
func BytesDecode(ctx context.Context, b []byte, obj interface{}) error {
	v := obj.(*[]byte)
	*v = b[:]
	return nil
}

// DecryptDecode returns a decode function that can be passed to NewDecoder when
// decoding an encrypted message (https://godoc.org/gocloud.dev/secrets).
//
// post defaults to BytesDecode. An optional decoder can be passed in to do
// further decode operation based on the decrypted message.
func DecryptDecode(k *secrets.Keeper, post Decode) Decode {
	return func(ctx context.Context, b []byte, obj interface{}) error {
		decrypted, err := k.Decrypt(ctx, b)
		if err != nil {
			return err
		}
		if post == nil {
			return BytesDecode(ctx, decrypted, obj)
		}
		return post(ctx, decrypted, obj)
	}
}

// DecoderByName returns a *Decoder based on decoderName.
//
// It is intended to be used by URL openers in driver packages.
//
// Supported values include:
//   - empty string: Returns the default from the URLOpener.Decoder, or
//     BytesDecoder if URLOpener.Decoder is nil (which is true if you're
//     using the default URLOpener).
//   - "bytes": Returns a BytesDecoder; Snapshot.Value will be of type []byte.
//   - "jsonmap": Returns a JSON decoder for a map[string]interface{};
//     Snapshot.Value will be of type *map[string]interface{}.
//   - "string": Returns StringDecoder; Snapshot.Value will be of type string.
//
// It also supports using "decrypt+<decoderName>" (or "decrypt" for default
// decoder) to decrypt the data before decoding. It uses the secrets package to
// open a keeper by the URL string stored in a environment variable
// "RUNTIMEVAR_KEEPER_URL". See https://godoc.org/gocloud.dev/secrets#OpenKeeper
// for more details.
func DecoderByName(ctx context.Context, decoderName string, dflt *Decoder) (*Decoder, error) {
	// Open a *secrets.Keeper if the decoderName contains "decrypt".
	k, decoderName, err := decryptByName(ctx, decoderName)
	if err != nil {
		return nil, err
	}

	if dflt == nil {
		dflt = BytesDecoder
	}
	switch decoderName {
	case "":
		return maybeDecrypt(ctx, k, dflt), nil
	case "bytes":
		return maybeDecrypt(ctx, k, BytesDecoder), nil
	case "jsonmap":
		var m map[string]interface{}
		return maybeDecrypt(ctx, k, NewDecoder(&m, JSONDecode)), nil
	case "string":
		return maybeDecrypt(ctx, k, StringDecoder), nil
	default:
		return nil, fmt.Errorf("unsupported decoder %q", decoderName)
	}
}

// decryptByName returns a *secrets.Keeper for decryption when decoderName
// contains "decrypt".
func decryptByName(ctx context.Context, decoderName string) (*secrets.Keeper, string, error) {
	if !strings.HasPrefix(decoderName, "decrypt") {
		return nil, decoderName, nil
	}
	keeperURL := os.Getenv("RUNTIMEVAR_KEEPER_URL")
	if keeperURL == "" {
		return nil, "", errors.New("environment variable RUNTIMEVAR_KEEPER_URL needed to open a *secrets.Keeper for decryption")
	}
	k, err := secrets.OpenKeeper(ctx, keeperURL)
	if err != nil {
		return nil, "", err
	}
	decoderName = strings.TrimPrefix(decoderName, "decrypt")
	if decoderName != "" {
		decoderName = strings.TrimLeftFunc(decoderName, func(r rune) bool {
			return r == ' ' || r == '+'
		})
	}
	// The parsed value is "decrypt <decoderName>".
	return k, decoderName, nil
}

func maybeDecrypt(ctx context.Context, k *secrets.Keeper, dec *Decoder) *Decoder {
	if k == nil {
		return dec
	}
	return NewDecoder(reflect.New(dec.typ).Elem().Interface(), DecryptDecode(k, dec.fn))
}
