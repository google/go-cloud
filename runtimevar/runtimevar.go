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
	"fmt"
	"net/url"
	"reflect"
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
// See https://godoc.org/gocloud.dev#As for background information, the "As"
// examples in this package for examples, and the provider-specific package
// documentation for the specific types supported for that provider.
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

// Variable provides an easy and portable way to watch runtime configuration
// variables. To create a Variable, use constructors found in provider-specific
// subpackages.
type Variable struct {
	watcher  driver.Watcher
	provider string // for metric collection
	nextCall time.Time
	prev     driver.State
}

// New is intended for use by provider implementations.
var New = newVar

// newVar creates a new *Variable based on a specific driver implementation.
func newVar(w driver.Watcher) *Variable {
	return &Variable{
		watcher:  w,
		provider: oc.ProviderName(w),
	}
}

// Watch returns a Snapshot of the current value of the variable.
//
// The first call to Watch will block while reading the variable from the
// provider, and will return the resulting Snapshot or error. If an error is
// returned, the returned Snapshot is a zero value and should be ignored.
//
// Subsequent calls will block until the variable's value changes or a different
// error occurs.
//
// Watch should not be called on the same variable from multiple goroutines
// concurrently. The typical use case is to call it in a single goroutine in a
// loop.
//
// If the variable does not exist, Watch returns an error for which
// gcerrors.Code will return gcerrors.NotFound.
func (c *Variable) Watch(ctx context.Context) (_ Snapshot, err error) {
	for {
		wait := c.nextCall.Sub(time.Now())
		if wait > 0 {
			select {
			case <-ctx.Done():
				return Snapshot{}, ctx.Err()
			case <-time.After(wait):
				// Continue.
			}
		}

		cur, wait := c.watcher.WatchVariable(ctx, c.prev)
		c.nextCall = time.Now().Add(wait)
		if cur == nil {
			// No change.
			continue
		}
		// Something new to return!
		c.prev = cur
		v, err := cur.Value()
		if err != nil {
			return Snapshot{}, wrapError(c.watcher, err)
		}
		_ = stats.RecordWithTags(ctx, []tag.Mutator{tag.Upsert(oc.ProviderKey, c.provider)}, changeMeasure.M(1))
		// Error from RecordWithTags is not possible.
		return Snapshot{
			Value:      v,
			UpdateTime: cur.UpdateTime(),
			asFunc:     cur.As,
		}, nil
	}
}

// Close closes the Variable; don't call Watch after this.
func (c *Variable) Close() error {
	err := c.watcher.Close()
	return wrapError(c.watcher, err)
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
// See https://godoc.org/gocloud.dev#As for background information.
func (c *Variable) ErrorAs(err error, i interface{}) bool {
	return gcerr.ErrorAs(err, i, c.watcher.ErrorAs)
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
