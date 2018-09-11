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

// Package constantvar provides a runtimevar driver implementation for variables
// that never change.
package constantvar

import (
	"context"
	"math"
	"time"

	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/driver"
)

// New constructs a runtimevar.Variable that returns value from Watch.
// Subsequent calls to Watch will block.
func New(value interface{}) *runtimevar.Variable {
	return runtimevar.New(&watcher{value: value, t: time.Now()})
}

// NewError constructs a runtimevar.Variable that returns err from Watch.
// Subsequent calls to Watch will block.
func NewError(err error) *runtimevar.Variable {
	return runtimevar.New(&watcher{err: err})
}

// impl implements driver.Watcher.
type watcher struct {
	value interface{}
	err   error
	t     time.Time
}

func (w *watcher) WatchVariable(ctx context.Context, prevVersion interface{}, prevErr error) (*driver.Variable, interface{}, time.Duration, error) {

	// The first time this is called, return the constant value.
	if prevVersion == nil && prevErr == nil {
		return &driver.Variable{Value: w.value, UpdateTime: w.t}, true, 0, w.err
	}
	// On subsequent calls, block ~forever as the value will never change.
	return nil, nil, time.Duration(math.Inf(+1)), nil
}

func (_ *watcher) Close() error { return nil }
