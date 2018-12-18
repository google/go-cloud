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
//  v, err := etcdvar.New("my variable", etcdClient, runtimevar.JSONDecode, nil)
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
			return Snapshot{}, wrapError(err)
		}
		return Snapshot{Value: v, UpdateTime: cur.UpdateTime()}, nil
	}
}

// Close closes the Variable; don't call Watch after this.
func (c *Variable) Close() error {
	err := c.watcher.Close()
	return wrapError(err)
}

// wrappedError is used to wrap all errors returned by drivers so that users
// are not given access to provider-specific errors.
type wrappedError struct {
	err error
}

func wrapError(err error) error {
	if err == nil {
		return nil
	}
	return &wrappedError{err: err}
}

func (w *wrappedError) Error() string {
	return "runtimevar: " + w.err.Error()
}

// Decode is a function type for unmarshaling/decoding bytes into given object.
type Decode func([]byte, interface{}) error

// Decoder is a helper for decoding bytes into a particular Go type object.  The Variable objects
// produced by a particular driver.Watcher should always contain the same type for Variable.Value
// field.  A driver.Watcher can use/construct a Decoder object with an associated type (Type) and
// decoding function (Func) for decoding retrieved bytes into Variable.Value.
type Decoder struct {
	Type reflect.Type
	// Func is a Decode function.
	Func Decode
}

// NewDecoder constructs a Decoder for given object that uses the given Decode function.
func NewDecoder(obj interface{}, fn Decode) *Decoder {
	return &Decoder{
		Type: reflect.TypeOf(obj),
		Func: fn,
	}
}

// Decode decodes given bytes into an object of type Type using Func.
func (d *Decoder) Decode(b []byte) (interface{}, error) {
	nv := reflect.New(d.Type).Interface()
	if err := d.Func(b, nv); err != nil {
		return nil, err
	}
	ptr := reflect.ValueOf(nv)
	return ptr.Elem().Interface(), nil
}

// Simple Decoder objects.
var (
	StringDecoder = &Decoder{
		Type: reflect.TypeOf(""),
		Func: stringDecode,
	}

	BytesDecoder = &Decoder{
		Type: reflect.TypeOf([]byte{}),
		Func: bytesDecode,
	}
)

// Decode functions.
var (
	JSONDecode = json.Unmarshal
)

// GobDecode gob decodes bytes into given object.
func GobDecode(data []byte, obj interface{}) error {
	return gob.NewDecoder(bytes.NewBuffer(data)).Decode(obj)
}

func stringDecode(b []byte, obj interface{}) error {
	// obj is a pointer to a string.
	v := reflect.ValueOf(obj).Elem()
	v.SetString(string(b))
	return nil
}

func bytesDecode(b []byte, obj interface{}) error {
	// obj is a pointer to []byte.
	v := reflect.ValueOf(obj).Elem()
	v.SetBytes(b)
	return nil
}
