// Copyright 2018 The Go Cloud Authors
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
// limtations under the License.

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
	"reflect"
	"time"

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
}

// New creates a new *Variable based on a specific driver implementation.
// End users should use subpackages to construct a *Variable instead of this
// function; see the package documentation for details.
func New(w driver.Watcher) *Variable {
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
func (c *Variable) Watch(ctx context.Context) (Snapshot, error) {
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

// Close closes the Variable; don't call Watch after this.
func (c *Variable) Close() error {
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
// See Snapshot.As for more details.
func ErrorAs(err error, i interface{}) bool {
	if err == nil || i == nil {
		return false
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
