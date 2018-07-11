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

// Package driver provides the interface for providers of runtimevar.  This serves as a contract
// of how the runtimevar API uses a provider implementation.
package driver

import (
	"context"
	"time"
)

// Variable contains a runtime variable and additional metadata about it.
type Variable struct {
	Value      interface{}
	UpdateTime time.Time
}

// Watcher watches for updates on a variable and returns an updated Variable object if
// there are changes.  A Watcher object is associated with a variable upon construction.
//
// An application can have more than one Watcher, one for each variable.  It is typical
// to only have one Watcher per variable.
//
// A Watcher provider can dictate the type of Variable.Value if the backend service dictates
// a particular format and type.  If the backend service has the flexibility to store bytes and
// allow clients to dictate the format, it is better for a Watcher provider to allow users to
// dictate the type of Variable.Value and a decoding function.  The Watcher provider can use the
// runtimevar.Decoder to facilitate the decoding logic.
type Watcher interface {
	// WatchVariable blocks until the variable changes, the Context's Done channel closes or an
	// error occurs.
	//
	// If the variable has changed, then method should return a Variable object with the
	// new value.
	//
	// It is recommended that an implementation should avoid returning the same error of the
	// previous WatchVariable call if the implementation can detect such and treat as no changes had
	// incurred instead.  A sample case is when the variable is deleted, WatchVariable should return an
	// error upon initial detection.  A succeeding WatchVariable call may decide to block until
	// the variable source is restored.
	//
	// To stop this function from blocking, caller can passed in Context object constructed via
	// WithCancel and call the cancel function.
	WatchVariable(context.Context) (Variable, error)
	// Close cleans up any resources used by the Watcher object.
	Close() error
}
