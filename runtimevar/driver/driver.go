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

// Package driver defines interfaces to be implemented by runtimevar drivers, which
// will be used by the runtimevar package to interact with the underlying services.
// Application code should use package runtimevar.
package driver // import "gocloud.dev/runtimevar/driver"

import (
	"context"
	"time"

	"gocloud.dev/gcerrors"
)

// DefaultWaitDuration is the default value for WaitDuration.
const DefaultWaitDuration = 30 * time.Second

// WaitDuration returns DefaultWaitDuration if d is <= 0, otherwise it returns d.
func WaitDuration(d time.Duration) time.Duration {
	if d <= 0 {
		return DefaultWaitDuration
	}
	return d
}

// State represents the current state of a variable.
type State interface {
	// Value returns the current variable value.
	Value() (any, error)
	// UpdateTime returns the update time for the variable.
	UpdateTime() time.Time

	// As converts i to driver-specific types.
	// See https://gocloud.dev/concepts/as/ for background information.
	As(any) bool
}

// Watcher watches for updates on a variable and returns an updated Variable object if
// there are changes.  A Watcher object is associated with a variable upon construction.
//
// An application can have more than one Watcher, one for each variable.  It is typical
// to only have one Watcher per variable.
//
// Many services store their configuration data as raw bytes; drivers for such
// services should include a runtimevar.Decoder in their constructor to allow
// users to decode the raw bytes into a particular format (e.g., parsing a
// JSON string).
//
// Drivers for services that don't have raw bytes may dictate the type of the exposed
// Snapshot.Value, or expose custom decoding logic.
type Watcher interface {
	// WatchVariable returns the current State of the variable.
	// If the State has not changed, it returns nil.
	//
	// If WatchVariable returns a wait time > 0, the portable type uses
	// it as a hint to not call WatchVariable again for the wait time.
	//
	// Implementations *may* block, but must return if ctx is Done. If the
	// variable has changed, then implementations *must* eventually return
	// it.
	//
	// A polling implementation should return (State, <poll interval>) for
	// a new State, or (nil, <poll interval>) if State hasn't changed.
	//
	// An implementation that receives notifications from an external source
	// about changes to the underlying variable should:
	// 1. If prev != nil, subscribe to change notifications.
	// 2. Fetch the current State.
	// 3. If prev == nil or if the State has changed, return (State, 0).
	//    A non-zero wait should be returned if State holds an error, to avoid
	//    spinning.
	// 4. Block until it detects a change or ctx is Done, then fetch and return
	//    (State, 0).
	// Note that the subscription in 1 must occur before 2 to avoid race conditions.
	WatchVariable(ctx context.Context, prev State) (state State, wait time.Duration)

	// Close cleans up any resources used by the Watcher object.
	Close() error

	// ErrorAs allows drivers to expose driver-specific types for returned
	// errors; see State.As for more details.
	ErrorAs(error, any) bool

	// ErrorCode should return a code that describes the error, which was returned by
	// one of the other methods in this interface.
	ErrorCode(error) gcerrors.ErrorCode
}
