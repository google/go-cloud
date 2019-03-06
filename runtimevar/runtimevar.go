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
// configuration variables.
//
// It provides a blocking method that returns a Snapshot of the variable value
// whenever a change is detected.
//
// Subpackages contain distinct implementations of runtimevar for various
// providers, including Cloud and on-prem solutions. For example, "etcdvar"
// supports variables stored in etcd. Your application should import one of
// these provider-specific subpackages and use its exported function(s) to
// create a *Variable; do not use the New function in this package. For example:
//
//  var v *runtimevar.Variable
//  var err error
//  v, err = etcdvar.New("my variable", etcdClient, runtimevar.JSONDecode, nil)
//  ...
//
// Then, write your application code using the *Variable type. You can
// easily reconfigure your initialization code to choose a different provider.
// You can develop your application locally using filevar or constantvar, and
// deploy it to multiple Cloud providers. You may find
// http://github.com/google/wire useful for managing your initialization code.
//
//
// OpenCensus Integration
//
// OpenCensus supports tracing and metric collection for multiple languages and
// backend providers. See https://opencensus.io.
//
// This API collects an OpenCensus metric "gocloud.dev/runtimevar/value_changes",
// a count of the number of times all variables have changed values, by provider.
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
	"reflect"
	"sync"
	"time"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/internal/oc"
	"gocloud.dev/runtimevar/driver"
)

// Snapshot contains a snapshot of a variable's value and metadata about it.
// It is intended to be read-only for users.
type Snapshot struct {
	// Value contains the value of the variable.
	// The type for Value depends on the provider; for most providers, it depends
	// on the decoder used when creating Variable.
	Value interface{}

	// UpdateTime is the time when the last change was detected.
	UpdateTime time.Time

	asFunc func(interface{}) bool
}

// As converts i to provider-specific types.
//
// This function (and the other As functions in this package) are inherently
// provider-specific, and using them will make that part of your application
// non-portable, so use with care.
//
// See the documentation for the subpackage you used to instantiate Variable to
// see which type(s) are supported.
//
// Usage:
//
// 1. Declare a variable of the provider-specific type you want to access.
//
// 2. Pass a pointer to it to As.
//
// 3. If the type is supported, As will return true and copy the
// provider-specific type into your variable. Otherwise, it will return false.
//
// See
// https://github.com/google/go-cloud/blob/master/internal/docs/design.md#as
// for more background.
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
			Description: "Count of variable value changes by provider.",
			TagKeys:     []tag.Key{oc.ProviderKey},
			Aggregation: view.Count(),
		},
	}
)

type snapshotPlusError struct {
	snapshot Snapshot
	err      error
}

// Variable provides an easy and portable way to watch runtime configuration
// variables. To create a Variable, use constructors found in provider-specific
// subpackages.
type Variable struct {
	dw       driver.Watcher
	provider string // for metric collection

	// For cancelling the background goroutine, and noticing when it has exited.
	backgroundCancel  func()
	backgroundCloseCh chan bool

	// haveGoodCh is closed and haveGood is set to true when we get the first
	// good value for the variable.
	haveGoodCh chan bool
	haveGood   bool

	// Holds the snapshot/error to return from the next call to Watch.
	// The buffer size is 1 so that we can store something in it even when there's
	// not a Watch in flight.
	nextWatchCh chan *snapshotPlusError

	mu     sync.Mutex
	latest *snapshotPlusError // the snapshot/error to return from Latest
	closed bool
}

// New is intended for use by provider implementations.
var New = newVar

// newVar creates a new *Variable based on a specific driver implementation.
func newVar(w driver.Watcher) *Variable {
	ctx, cancel := context.WithCancel(context.Background())
	v := &Variable{
		dw:                w,
		provider:          oc.ProviderName(w),
		backgroundCancel:  cancel,
		backgroundCloseCh: make(chan bool),
		haveGoodCh:        make(chan bool),
		nextWatchCh:       make(chan *snapshotPlusError, 1),
		latest:            &snapshotPlusError{err: errors.New("no value yet")},
	}
	go v.background(ctx)
	return v
}

// ErrClosed is returned from Watch when the Variable has been Closed.
var ErrClosed = errors.New("Variable has been closed")

// Watch returns when there is a new Snapshot of the current value of the
// variable.
//
// The first call to Watch will block while reading the variable from the
// provider, and will return the resulting Snapshot or error. If an error is
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
	c.mu.Lock()
	if c.closed {
		return Snapshot{}, ErrClosed
	}
	c.mu.Unlock()

	select {
	case wr := <-c.nextWatchCh:
		return wr.snapshot, wr.err
	case <-ctx.Done():
		return Snapshot{}, ctx.Err()
	}
}

func (c *Variable) background(ctx context.Context) {
	var curState, prevState driver.State
	var wait time.Duration
	for {
		select {
		case <-ctx.Done():
			// We're shutting down; exit the goroutine.
			close(c.backgroundCloseCh)
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

		var spe snapshotPlusError
		if val, err := curState.Value(); err == nil {
			spe.snapshot = Snapshot{
				Value:      val,
				UpdateTime: curState.UpdateTime(),
				asFunc:     curState.As,
			}
		} else {
			spe.err = wrapError(c.dw, err)
		}

		// Update c.latest under the lock.
		c.mu.Lock()
		// Save the new value if it's good, or if it's an error and we don't
		// have a good value -- that way we always return the latest error.
		if spe.err == nil || !c.haveGood {
			c.latest = &spe
		}
		if spe.err == nil && !c.haveGood {
			// This is the first good value! Close haveGoodCh so that Latest doesn't
			// block anymore.
			close(c.haveGoodCh)
			c.haveGood = true
		}
		c.mu.Unlock()

		// Read anything buffered on nextWatchCh, in case we wrote something before
		// but nobody noticed. The write is then guaranteed to not block since
		// buffer size = 1.
		select {
		case <-c.nextWatchCh:
		default:
		}
		c.nextWatchCh <- &spe
	}
}

// Latest is intended to be called per request, with the request context.
// It returns the latest good Snapshot of the variable value, blocking if no
// good value has ever been received. Pass an already-canceled ctx to make
// Latest not block at all.
//
// Latest returns ErrClosed if the Variable has been closed.
func (c *Variable) Latest(ctx context.Context) (Snapshot, error) {
	select {
	case <-c.haveGoodCh:
	case <-ctx.Done():
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return Snapshot{}, ErrClosed
	}
	return c.latest.snapshot, c.latest.err
}

// Close closes the Variable. The Variable is unusable after Close returns.
func (c *Variable) Close() error {
	// Record that we're closing. Subsequent calls to Watch will return ErrClosed.
	c.mu.Lock()
	if c.closed {
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	// Shut down the background goroutine.
	c.backgroundCancel()
	<-c.backgroundCloseCh

	// Close any remaining channels to wake up any callers that are waiting on them.
	c.mu.Lock()
	close(c.nextWatchCh)
	if !c.haveGood {
		close(c.haveGoodCh)
	}
	c.mu.Unlock()

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

// ErrorAs converts i to provider-specific types.
// ErrorAs panics if i is nil or not a pointer.
// ErrorAs returns false if err == nil.
// See Snapshot.As for more details.
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
//
// The zero value is a multiplexer with no registered schemes.
type URLMux struct {
	schemes map[string]VariableURLOpener
}

// RegisterVariable registers the opener with the given scheme. If an opener
// already exists for the scheme, RegisterVariable panics.
func (mux *URLMux) RegisterVariable(scheme string, opener VariableURLOpener) {
	if mux.schemes == nil {
		mux.schemes = make(map[string]VariableURLOpener)
	} else if _, exists := mux.schemes[scheme]; exists {
		panic(fmt.Errorf("scheme %q already registered on mux", scheme))
	}
	mux.schemes[scheme] = opener
}

// OpenVariable calls OpenVariableURL with the URL parsed from urlstr.
// OpenVariable is safe to call from multiple goroutines.
func (mux *URLMux) OpenVariable(ctx context.Context, urlstr string) (*Variable, error) {
	u, err := url.Parse(urlstr)
	if err != nil {
		return nil, fmt.Errorf("open variable: %v", err)
	}
	return mux.OpenVariableURL(ctx, u)
}

// OpenVariableURL dispatches the URL to the opener that is registered with the
// URL's scheme. OpenVariableURL is safe to call from multiple goroutines.
func (mux *URLMux) OpenVariableURL(ctx context.Context, u *url.URL) (*Variable, error) {
	if u.Scheme == "" {
		return nil, fmt.Errorf("open variable %q: no scheme in URL", u)
	}
	var opener VariableURLOpener
	if mux != nil {
		opener = mux.schemes[u.Scheme]
	}
	if opener == nil {
		return nil, fmt.Errorf("open variable %q: no provider registered for %s", u, u.Scheme)
	}
	return opener.OpenVariableURL(ctx, u)
}

var defaultURLMux = new(URLMux)

// DefaultURLMux returns the URLMux used by OpenVariable.
//
// Driver packages can use this to register their VariableURLOpener on the mux.
func DefaultURLMux() *URLMux {
	return defaultURLMux
}

// OpenVariable opens the variable identified by the URL given. URL openers must be
// registered in the DefaultURLMux, which is typically done in driver
// packages' initialization.
//
// See the URLOpener documentation in provider-specific subpackages for more
// details on supported scheme(s) and URL parameter(s).
func OpenVariable(ctx context.Context, urlstr string) (*Variable, error) {
	return defaultURLMux.OpenVariable(ctx, urlstr)
}

// Decode is a function type for unmarshaling/decoding a slice of bytes into
// an arbitrary type. Decode functions are used when creating a Decoder via
// NewDecoder. This package provides common Decode functions including
// GobDecode and JSONDecode.
type Decode func([]byte, interface{}) error

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
func (d *Decoder) Decode(b []byte) (interface{}, error) {
	nv := reflect.New(d.typ).Interface()
	if err := d.fn(b, nv); err != nil {
		return nil, err
	}
	ptr := reflect.ValueOf(nv)
	return ptr.Elem().Interface(), nil
}

var (
	// StringDecoder decodes into strings.
	StringDecoder = NewDecoder("", stringDecode)

	// BytesDecoder copies the slice of bytes.
	BytesDecoder = NewDecoder([]byte{}, bytesDecode)

	// JSONDecode can be passed to NewDecoder when decoding JSON (https://golang.org/pkg/encoding/json/).
	JSONDecode = json.Unmarshal
)

// GobDecode can be passed to NewDecoder when decoding gobs (https://golang.org/pkg/encoding/gob/).
func GobDecode(data []byte, obj interface{}) error {
	return gob.NewDecoder(bytes.NewBuffer(data)).Decode(obj)
}

func stringDecode(b []byte, obj interface{}) error {
	v := obj.(*string)
	*v = string(b)
	return nil
}

func bytesDecode(b []byte, obj interface{}) error {
	v := obj.(*[]byte)
	*v = b[:]
	return nil
}
