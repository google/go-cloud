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

// Package batcher supports batching of items. Create a Batcher with a handler and
// add items to it. Items are accumulated while handler calls are in progress; when
// the handler returns, it will be called again with items accumulated since the last
// call. Multiple concurrent calls to the handler are supported.
package batcher

import (
	"context"
	"reflect"
	"sync"
)

// A Batcher batches items.
type Batcher struct {
	maxHandlers   int
	handler       func(interface{}) error
	itemSliceZero reflect.Value // nil (zero value) for slice of items

	mu        sync.Mutex
	pending   []waiter // items waiting to be handled
	nHandlers int      // number of currently running handler goroutines
}

type waiter struct {
	item interface{}
	errc chan error
}

// New creates a new Batcher.
//
// itemExample is a value of the type that will be batched. For example, if you
// want to create batches of *Entry, pass &Entry{} for itemExample.
//
// maxHandlers is the maximum number of handlers that will run concurrently.
//
// handler is a function that will be called on each bundle. If itemExample is
// of type T, the argument to handler is of type []T.
func New(itemExample interface{}, maxHandlers int, handler func(interface{}) error) *Batcher {
	return &Batcher{
		maxHandlers:   maxHandlers,
		handler:       handler,
		itemSliceZero: reflect.Zero(reflect.SliceOf(reflect.TypeOf(itemExample))),
	}
}

// Add adds an item to the batcher. It blocks until the handler has
// processed the item and reports the error that the handler returned.
func (b *Batcher) Add(ctx context.Context, item interface{}) error {
	// Create a channel to receive the error from the handler.
	c := make(chan error, 1)
	b.mu.Lock()
	// Add the item to the pending list.
	b.pending = append(b.pending, waiter{item, c})
	if b.nHandlers < b.maxHandlers {
		// If we can start a handler, do so with the item just added and any others that are pending.
		go b.callHandler(b.pending)
		b.pending = nil
		b.nHandlers++
	}
	// If we can't start a handler, then one of the currently running handlers will
	// take our item.
	b.mu.Unlock()
	// Wait until either our result is ready or the context is done.
	select {
	case err := <-c:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *Batcher) callHandler(batch []waiter) {
	for batch != nil {
		// Collect the items into a slice of the example type.
		items := b.itemSliceZero
		for _, m := range batch {
			items = reflect.Append(items, reflect.ValueOf(m.item))
		}
		// Call the handler and report the result to all waiting
		// callers of Add.
		err := b.handler(items.Interface())
		for _, m := range batch {
			m.errc <- err
		}
		b.mu.Lock()
		// If there is more work, keep running; otherwise exit. Take the new batch
		// and decrement the handler count atomically, so that newly added items will
		// always get to run.
		batch = b.pending
		b.pending = nil
		if batch == nil {
			b.nHandlers--
		}
		b.mu.Unlock()
	}
}
