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

// Package constantvar provides a runtimevar implementation with Variables
// that never change. Use New, NewBytes, or NewError to construct a
// *runtimevar.Variable.
//
// As
//
// constantvar does not support any types for As.
package constantvar // import "gocloud.dev/runtimevar/constantvar"

import (
	"context"
	"errors"
	"time"

	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
)

// ErrNotExist is a sentinel error that can be used with NewError to return an
// error for which IsNotExist returns true.
var ErrNotExist = errors.New("variable does not exist")

// New constructs a *runtimevar.Variable holding value.
func New(value interface{}) *runtimevar.Variable {
	return runtimevar.New(&watcher{value: value, t: time.Now()})
}

// NewBytes uses decoder to decode b. If the decode succeeds, it constructs
// a *runtimevar.Variable holding the decoded value. If the decode fails, it
// constructs a runtimevar.Variable that always fails with the error.
func NewBytes(b []byte, decoder *runtimevar.Decoder) *runtimevar.Variable {
	value, err := decoder.Decode(b)
	if err != nil {
		return NewError(err)
	}
	return New(value)
}

// NewError constructs a *runtimevar.Variable that always fails. Runtimevar
// wraps errors returned by provider implementations, so the error returned
// by runtimevar will not equal err.
func NewError(err error) *runtimevar.Variable {
	return runtimevar.New(&watcher{err: err})
}

// watcher implements driver.Watcher and driver.State.
type watcher struct {
	value interface{}
	err   error
	t     time.Time
}

// Value implements driver.State.Value.
func (w *watcher) Value() (interface{}, error) {
	return w.value, w.err
}

// UpdateTime implements driver.State.UpdateTime.
func (w *watcher) UpdateTime() time.Time {
	return w.t
}

// As implements driver.State.As.
func (w *watcher) As(i interface{}) bool {
	return false
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

// ErrorAs implements driver.ErrorAs.
func (*watcher) ErrorAs(err error, i interface{}) bool { return false }

// IsNotExist implements driver.IsNotExist.
func (*watcher) IsNotExist(err error) bool { return err == ErrNotExist }
