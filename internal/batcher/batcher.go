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
	"errors"
	"reflect"
	"sync"
)

// A Batcher batches items.
type Batcher struct {
	maxHandlers   int
	handler       func(interface{}) error
	itemSliceZero reflect.Value  // nil (zero value) for slice of items
	wg            sync.WaitGroup // tracks active Add calls

	mu        sync.Mutex
	pending   []waiter // items waiting to be handled
	nHandlers int      // number of currently running handler goroutines
	shutdown  bool
}

type waiter struct {
	item interface{}
	errc chan error
}

// New creates a new Batcher.
//
// itemType is type that will be batched. For example, if you
// want to create batches of *Entry, pass reflect.TypeOf(&Entry{}) for itemType.
//
// maxHandlers is the maximum number of handlers that will run concurrently.
//
// handler is a function that will be called on each bundle. If itemExample is
// of type T, the argument to handler is of type []T.
func New(itemType reflect.Type, maxHandlers int, handler func(interface{}) error) *Batcher {
	return &Batcher{
		maxHandlers:   maxHandlers,
		handler:       handler,
		itemSliceZero: reflect.Zero(reflect.SliceOf(itemType)),
	}
}

// Add adds an item to the batcher. It blocks until the handler has
// processed the item and reports the error that the handler returned.
// If Shutdown has been called, Add immediately returns an error.
func (b *Batcher) Add(ctx context.Context, item interface{}) error {
	c := b.AddNoWait(item)
	// Wait until either our result is ready or the context is done.
	select {
	case err := <-c:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// AddNoWait adds an item to the batcher and returns immediately. When the handler is
// called on the item, the handler's error return value will be sent to the channel
// returned from AddNoWait.
func (b *Batcher) AddNoWait(item interface{}) <-chan error {
	b.mu.Lock()
	defer b.mu.Unlock()
	// Create a channel to receive the error from the handler.
	c := make(chan error, 1)
	if b.shutdown {
		c <- errors.New("batcher: shut down")
		return c
	}
	// Add the item to the pending list.
	b.pending = append(b.pending, waiter{item, c})
	if b.nHandlers < b.maxHandlers {
		// If we can start a handler, do so with the item just added and any others that are pending.
		batch := b.pending
		b.pending = nil
		b.wg.Add(1)
		go func() {
			b.callHandler(batch)
			b.wg.Done()
		}()
		b.nHandlers++
	}
	// If we can't start a handler, then one of the currently running handlers will
	// take our item.
	return c
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

// Shutdown waits for all active calls to Add to finish, then
// returns. After Shutdown is called, all subsequent calls to Add fail.
// Shutdown should be called only once.
func (b *Batcher) Shutdown() {
	b.mu.Lock()
	b.shutdown = true
	b.mu.Unlock()
	b.wg.Wait()
}
