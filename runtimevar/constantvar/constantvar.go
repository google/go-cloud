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

// Package constantvar provides a runtimevar.Driver implementation for variables
// that never change.
package constantvar // import "gocloud.dev/runtimevar/constantvar"

import (
	"context"
	"time"

	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
)

// New constructs a runtimevar.Variable that returns a Snapshot with value from
// Watch. Subsequent calls to Watch will block.
func New(value interface{}) *runtimevar.Variable {
	return runtimevar.New(&watcher{value: value, t: time.Now()})
}

// NewBytes uses decoder to decode b. If the decode succeeds, it returns
// New with the decoded value, otherwise it returns NewError with the error.
func NewBytes(b []byte, decoder *runtimevar.Decoder) *runtimevar.Variable {
	value, err := decoder.Decode(b)
	if err != nil {
		return NewError(err)
	}
	return New(value)
}

// NewError constructs a runtimevar.Variable that returns err from Watch.
// Subsequent calls to Watch will block.
func NewError(err error) *runtimevar.Variable {
	return runtimevar.New(&watcher{err: err})
}

// watcher implements driver.Watcher and driver.State.
type watcher struct {
	value interface{}
	err   error
	t     time.Time
}

func (w *watcher) Value() (interface{}, error) {
	return w.value, w.err
}

func (w *watcher) UpdateTime() time.Time {
	return w.t
}

// WatchVariable implements driver.WatchVariable.
func (w *watcher) WatchVariable(ctx context.Context, prev driver.State) (driver.State, time.Duration) {

	// The first time this is called, return the constant value.
	if prev == nil {
		return w, 0
	}
	// On subsequent calls, block forever as the value will never change.
	<-ctx.Done()
	w.err = ctx.Err()
	return w, 0
}

// Close implements driver.Close.
func (*watcher) Close() error { return nil }
