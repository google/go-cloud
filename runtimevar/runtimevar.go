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
package runtimevar // import "gocloud.dev/runtimevar"

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"time"

	"gocloud.dev/internal/trace"
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
// See the documentation for the subpackage used to instantiate Variable to see
// which type(s) are supported.
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

// Variable provides an easy and portable way to watch runtime configuration
// variables. To create a Variable, use constructors found in provider-specific
// subpackages.
type Variable struct {
	watcher  driver.Watcher
	nextCall time.Time
	prev     driver.State

	mu sync.Mutex
	w  *watcher
}

// New is intended for use by provider implementations.
var New = newVar

// newVar  creates a new *Variable based on a specific driver implementation.
func newVar(w driver.Watcher) *Variable {
	return &Variable{watcher: w}
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
// Watch is not goroutine-safe; typical use is to call it in a single
// goroutine in a loop.
//
// Alternatively, use Latest (and optionally InitLatest) to retrieve the latest
// good config. If Latest or InitLatest has been called, Watch panics.
func (c *Variable) Watch(ctx context.Context) (_ Snapshot, err error) {
	ctx = trace.StartSpan(ctx, "gocloud.dev/runtimevar.Watch")
	defer func() { trace.EndSpan(ctx, err) }()

	c.mu.Lock()
	if c.w != nil {
		panic("Watch called after InitLatest")
	}
	c.mu.Unlock()
	return c.watch(ctx)
}

// Implements Watch above, minus the check to ensure that it's not being
// called after Latest/InitLatest.
func (c *Variable) watch(ctx context.Context) (Snapshot, error) {
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
		return Snapshot{
			Value:      v,
			UpdateTime: cur.UpdateTime(),
			asFunc:     cur.As,
		}, nil
	}
}

// ErrNoValue is returned from Latest if no good value of the variable has
// been read.
var ErrNoValue = errors.New("runtimevar: no good value received")

// InitLatest prepares Variable for Latest to be called.
//
// It starts a goroutine that calls Watch and saves the last good value.
// Errors received from Watch are written to errCh so that they can be logged
// by the application; errCh may be nil to ignore errors. If non-nil, errCh will
// be closed as part of Close.
//
// InitLatest blocks until ctx is done or until a good value of the variable
// is read, whichever is first. ctx may be nil to avoid blocking entirely.
// Typically it is called during application initialization with a short or nil
// timeout.
func (c *Variable) InitLatest(ctx context.Context, errCh chan<- error) {
	c.mu.Lock()
	if c.w == nil {
		// First time. Create and start a background watcher.
		ctx, cancel := context.WithCancel(context.Background())
		c.w = &watcher{
			cancel:  cancel,
			closeCh: make(chan bool),
			goodCh:  make(chan bool),
		}
		go c.w.watch(ctx, c, errCh)
	}
	c.mu.Unlock()

	// If ctx is provided, block until it is done or until a good value
	// is received.
	if ctx != nil {
		select {
		case <-ctx.Done():
		case <-c.w.goodCh:
		}
	}
}

// Latest always returns the last good value of the variable, or ErrNoValue
// if no good value has been received.
//
// Latest blocks until ctx is done or until a good value of the variable
// is read, whichever is first. ctx may be nil to avoid blocking entirely.
// Typically it is called when processing a request, with the request context.
//
// Latest calls InitLatest, so it is not necessary to call InitLatest
// explicitly unless you want to prime fetching of the variable's value
// or process errors via errCh.
func (c *Variable) Latest(ctx context.Context) (Snapshot, error) {
	// InitLatest ensures c.w is set, and does any needed
	// blocking for ctx.
	c.InitLatest(ctx, nil)

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.w.latest()
}

type watcher struct {
	cancel  func()
	closeCh chan bool // a single boolean value is written during shutdown
	goodCh  chan bool // closed when a good value is received

	mu       sync.Mutex
	lastGood *Snapshot
}

// watch calls v.Watch in a loop, saving good values in lastGood.
func (w *watcher) watch(ctx context.Context, v *Variable, errCh chan<- error) {
	for ctx.Err() == nil {
		s, err := v.watch(ctx)
		if err != nil {
			if errCh != nil {
				// TODO: should this be non-blocking?
				errCh <- err
			}
			continue
		}
		// Got a good value!
		w.mu.Lock()
		if w.lastGood == nil {
			close(w.goodCh)
		}
		w.lastGood = &s
		w.mu.Unlock()
	}
	// Shutting down.
	if errCh != nil {
		close(errCh)
	}
	w.closeCh <- true
}

func (w *watcher) latest() (Snapshot, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.lastGood == nil {
		return Snapshot{}, ErrNoValue
	}
	return *w.lastGood, nil
}

func (w *watcher) close() {
	w.cancel()
	<-w.closeCh
}

// Close closes the Variable. The variable is unusable after Close returns.
func (c *Variable) Close() error {
	c.mu.Lock()
	if c.w != nil {
		c.w.close()
		c.w = nil
	}
	c.mu.Unlock()
	err := c.watcher.Close()
	return wrapError(c.watcher, err)
}

// wrappedError is used to wrap all errors returned by drivers so that users
// are not given access to provider-specific errors.
type wrappedError struct {
	err error
	w   driver.Watcher
}

func wrapError(w driver.Watcher, err error) error {
	if err == nil {
		return nil
	}
	return &wrappedError{w: w, err: err}
}

func (w *wrappedError) Error() string {
	return "runtimevar: " + w.err.Error()
}

// ErrorAs converts i to provider-specific types.
// ErrorAs panics if i is nil or not a pointer.
// See Snapshot.As for more details.
func ErrorAs(err error, i interface{}) bool {
	if err == nil {
		return false
	}
	if i == nil || reflect.TypeOf(i).Kind() != reflect.Ptr {
		panic("runtimevar: ErrorAs i must be a non-nil pointer")
	}
	if e, ok := err.(*wrappedError); ok {
		return e.w.ErrorAs(e.err, i)
	}
	return false
}

// IsNotExist returns true iff err indicates that the referenced variable does not exist.
func IsNotExist(err error) bool {
	if e, ok := err.(*wrappedError); ok {
		return e.w.IsNotExist(e.err)
	}
	return false
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
