// Copyright 2018 Google LLC
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

// Package runtimevar provides an interface for reading runtime variables and
// ability to detect changes and get updates on those variables.
package runtimevar

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/google/go-cloud/runtimevar/driver"
)

// Snapshot contains a variable and metadata about it.
type Snapshot struct {
	// Value is an object containing a runtime variable  The type of
	// this object is set by the driver and it should always be the same type for the same Variable
	// object. A driver implementation can provide the ability to variableure the object type and a
	// decoding scheme where variables are stored as bytes in the backend service.  Clients
	// should not mutate this object as it can be accessed by multiple goroutines.
	Value interface{}

	// UpdateTime is the time when the last changed was detected.
	UpdateTime time.Time
}

// Variable provides the ability to read runtime variables with its blocking Watch method.
type Variable struct {
	watcher driver.Watcher
}

// New constructs a Variable object given a driver.Watcher implementation.
func New(w driver.Watcher) *Variable {
	return &Variable{watcher: w}
}

// Watch blocks until there are variable changes, the Context's Done channel closes or an error
// occurs.
//
// If the variable, the method returns a Snapshot object containing the
// updated value.
//
// If method returns an error, the returned Snapshot object is a zero value and cannot be used.
//
// The first call to this method should return the current variable unless there are errors in
// retrieving the value.
//
// Users should not call this method from multiple goroutines as implementations may not guarantee
// safety in data access.  It is typical to use only one goroutine calling this method in a loop.
//
// To stop this function from blocking, caller can passed in Context object constructed via
// WithCancel and call the cancel function.
func (c *Variable) Watch(ctx context.Context) (Snapshot, error) {
	variable, err := c.watcher.WatchVariable(ctx)
	if err != nil {
		// Mask underlying errors.
		return Snapshot{}, fmt.Errorf("Variable.Watch: %v", err)
	}
	return Snapshot{
		Value:      variable.Value,
		UpdateTime: variable.UpdateTime,
	}, nil
}

// Close cleans up any resources used by the Variable object.
func (c *Variable) Close() error {
	return c.watcher.Close()
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
