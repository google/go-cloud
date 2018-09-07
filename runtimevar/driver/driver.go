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
	// WatchVariable returns one of:
	//
	// 1. A new value for the variable in v, along with a provider-specific
	//    version that will be passed to the next WatchVariable call. wait is
	//    ignored and err must be nil.
	// 2. A new error. v, version, and wait are ignored.
	// 3. A nil v and version and err, indicating that the value of the variable
	//    or the error returned has not changed. WatchVariable will not be called
	//    again for wait.
	//
	// Implementations *may* block, but must return if ctx is Done. If the
	// variable has changed, then implementations *must* eventually return it.
	//
	// For example, an implementation that can detect changes in the underlying
	// variable could block until it detects a change (or until ctx is Done).
	// A polling implementation could poll on every call to WatchVariable,
	// returning nil v/version/err and a non-zero wait to set the poll interval
	// if there's no change.
	//
	// Version is provider-specific; for example, it could be an actual version
	// number, it could be the variable value, or it could be raw bytes for the
	// variable before decoding it.
	// TODO(rvangent): Consider refactoring to encapsulate State (including
	//                 prevVersion, prevErr), and "is the same as" checking)
	//                 behind an interface.
	WatchVariable(ctx context.Context, prevVersion interface{}, prevErr error) (v *Variable, version interface{}, wait time.Duration, err error)

	// Close cleans up any resources used by the Watcher object.
	Close() error
}
