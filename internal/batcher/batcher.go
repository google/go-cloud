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
// the handler returns, it will be called again with the accumulated items.
package batcher

import (
	"context"
	"reflect"
	"sync"
)

// A Batcher batches items.
type Batcher struct {
	handler       func(interface{}) error
	itemSliceZero reflect.Value // nil (zero value) for slice of items
	sem           chan struct{} // semaphore for outstanding handlers

	mu      sync.Mutex
	pending []waiter // items waiting to be handled
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
// maxOutstandingHandlers is the maximum number of handlers that will run concurrently.
//
// handler is a function that will be called on each bundle. If itemExample is
// of type T, the argument to handler is of type []T.
func New(itemExample interface{}, maxOutstandingHandlers int, handler func(interface{}) error) *Batcher {
	b := &Batcher{
		handler:       handler,
		sem:           make(chan struct{}, maxOutstandingHandlers),
		itemSliceZero: reflect.Zero(reflect.SliceOf(reflect.TypeOf(itemExample))),
	}
	for i := 0; i < maxOutstandingHandlers; i++ {
		b.sem <- struct{}{}
	}
	return b
}

// Add adds an item to the batcher. It blocks until the handler has
// processed the item and reports the error that the handler returned.
func (b *Batcher) Add(ctx context.Context, item interface{}) error {
	// Create a channel to receive the error from the handler. Give it a buffer so
	// that if a goroutine is in the middle of calling a handler when another
	// finishes, the latter won't block sending to the channel.
	c := make(chan error, 1)
	b.mu.Lock()
	b.pending = append(b.pending, waiter{item, c})
	b.mu.Unlock()
	// Keep processing batches until this item has been handled.
	for {
		// If there's a choice between doing work and getting the result for this
		// item, prefer the result.
		select {
		case err := <-c:
			return err
		default:
		}
		// Wait until either we get a result or we can do some work.
		select {
		case err := <-c:
			return err
		case <-b.sem:
			b.callHandler()
			b.sem <- struct{}{}
		}
	}
}

// Invoke the handler on the slice of pending items.
func (b *Batcher) callHandler() {
	b.mu.Lock()
	if len(b.pending) == 0 {
		b.mu.Unlock()
		return
	}
	batch := b.pending
	b.pending = nil
	b.mu.Unlock()
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
}
