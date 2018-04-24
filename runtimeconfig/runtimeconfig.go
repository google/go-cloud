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
// limitations under the License.

// Package runtimeconfig provides an interface for reading runtime configurations and
// ability to detect changes and get updates on those configurations.
package runtimeconfig

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/google/go-cloud/runtimeconfig/driver"
)

// Snapshot contains a group of associated runtime configurations and metadata about this grouping.
type Snapshot struct {
	// Value is an object containing the group of associated runtime configurations.  The type of
	// this object is set by the driver and it should always be the same type for the same Config
	// object. A driver implementation can provide the ability to configure the object type and a
	// decoding scheme where configurations are stored as bytes in the backend service.  Clients
	// should not mutate this object as it can be accessed by multiple goroutines.
	Value interface{}

	// UpdateTime is the time when the last changed was detected.
	UpdateTime time.Time
}

// Config provides the ability to read runtime configurations with its blocking Watch method.
//
// An application can have more than one Config, one for each configuration group.  It is typical to
// only have one Config per configuration group.
type Config struct {
	watcher driver.Watcher
}

// New constructs a Config object given a driver.Watcher implementation.
func New(w driver.Watcher) *Config {
	return &Config{watcher: w}
}

// Watch blocks until there are configuration changes, the Context's Done channel closes or an error
// occurs.
//
// If the configuration group has changed, then method returns a Snapshot object containing the
// updated value.
//
// If method returns an error, the returned Snapshot object is a zero value and cannot be used.
//
// The first call to this method should return the current configuration unless there are errors in
// retrieving the value.
//
// Users should not call this method from multiple goroutines as implementations may not guarantee
// safety in data access.  It is typical to use only one goroutine calling this method in a loop.
//
// To stop this function from blocking, caller can passed in Context object constructed via
// WithCancel and call the cancel function.
func (c *Config) Watch(ctx context.Context) (Snapshot, error) {
	cfg, err := c.watcher.Watch(ctx)
	if err != nil {
		// Mask underlying errors.
		return Snapshot{}, fmt.Errorf("Config.Watch: %v", err)
	}
	return Snapshot{
		Value:      cfg.Value,
		UpdateTime: cfg.UpdateTime,
	}, nil
}

// Close cleans up any resources used by the Config object.
func (c *Config) Close() error {
	return c.watcher.Close()
}

// Decode is a function type for unmarshaling/decoding bytes into given object.
type Decode func([]byte, interface{}) error

// Decoder is a helper for decoding bytes into a particular Go type object.  The Config objects
// produced by a particular driver.Watcher should always contain the same type for Config.Value
// field.  A driver.Watcher can use/construct a Decoder object with an associated type (Type) and
// decoding function (Func) for decoding retrieved bytes into Config.Value.
type Decoder struct {
	Type reflect.Type
	// Func is a function that decodes given bytes into given pointer to an object of type Type.
	Func func([]byte, interface{}) error
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
