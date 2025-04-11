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

// Package batcher supports batching of items. Create a Batcher with a handler and
// add items to it. Items are accumulated while handler calls are in progress; when
// the handler returns, it will be called again with items accumulated since the last
// call. Multiple concurrent calls to the handler are supported.
package batcher // import "gocloud.dev/pubsub/batcher"

import (
	"context"
	"errors"
	"reflect"
	"sync"
)

// Split determines how to split n (representing n items) into batches based on
// opts. It returns a slice of batch sizes.
//
// For example, Split(10) might return [10], [5, 5], or [2, 2, 2, 2, 2]
// depending on opts. opts may be nil to accept defaults.
//
// Split will return nil if n is less than o.MinBatchSize.
//
// The sum of returned batches may be less than n (e.g., if n is 10x larger
// than o.MaxBatchSize, but o.MaxHandlers is less than 10).
func Split(n int, opts *Options) []int {
	o := newOptionsWithDefaults(opts)
	if n < o.MinBatchSize {
		// No batch yet.
		return nil
	}
	if o.MaxBatchSize == 0 {
		// One batch is fine.
		return []int{n}
	}

	// TODO(rvangent): Consider trying to even out the batch sizes.
	// For example, n=10 with MaxBatchSize 9 and MaxHandlers 2 will Split
	// to [9, 1]; it could be [5, 5].
	var batches []int
	for n >= o.MinBatchSize && len(batches) < o.MaxHandlers {
		b := o.MaxBatchSize
		if b > n {
			b = n
		}
		batches = append(batches, b)
		n -= b
	}
	return batches
}

// A Batcher batches items.
type Batcher struct {
	opts          Options
	handler       func(any) error
	itemSliceZero reflect.Value  // nil (zero value) for slice of items
	wg            sync.WaitGroup // tracks active Add calls

	mu        sync.Mutex
	pending   []waiter // items waiting to be handled
	nHandlers int      // number of currently running handler goroutines
	shutdown  bool
}

// Message is larger than the maximum batch byte size
var ErrMessageTooLarge = errors.New("batcher: message too large")

type sizableItem interface {
	ByteSize() int
}

type waiter struct {
	item any
	errc chan error
}

// Options sets options for Batcher.
type Options struct {
	// Maximum number of concurrent handlers. Defaults to 1.
	MaxHandlers int
	// Minimum size of a batch. Defaults to 1.
	MinBatchSize int
	// Maximum size of a batch. 0 means no limit.
	MaxBatchSize int
	// Maximum bytesize of a batch. 0 means no limit.
	MaxBatchByteSize int
}

// newOptionsWithDefaults returns Options with defaults applied to opts.
// opts may be nil to accept all defaults.
func newOptionsWithDefaults(opts *Options) Options {
	var o Options
	if opts != nil {
		o = *opts
	}
	if o.MaxHandlers == 0 {
		o.MaxHandlers = 1
	}
	if o.MinBatchSize == 0 {
		o.MinBatchSize = 1
	}
	return o
}

// newMergedOptions returns o merged with opts.
func (o *Options) NewMergedOptions(opts *Options) *Options {
	maxH := o.MaxHandlers
	if opts.MaxHandlers != 0 && (maxH == 0 || opts.MaxHandlers < maxH) {
		maxH = opts.MaxHandlers
	}
	minB := o.MinBatchSize
	if opts.MinBatchSize != 0 && (minB == 0 || opts.MinBatchSize > minB) {
		minB = opts.MinBatchSize
	}
	maxB := o.MaxBatchSize
	if opts.MaxBatchSize != 0 && (maxB == 0 || opts.MaxBatchSize < maxB) {
		maxB = opts.MaxBatchSize
	}
	maxBB := o.MaxBatchByteSize
	if opts.MaxBatchByteSize != 0 && (maxBB == 0 || opts.MaxBatchByteSize < maxBB) {
		maxBB = opts.MaxBatchByteSize
	}
	c := &Options{
		MaxHandlers:      maxH,
		MinBatchSize:     minB,
		MaxBatchSize:     maxB,
		MaxBatchByteSize: maxBB,
	}
	return c
}

// New creates a new Batcher.
//
// itemType is type that will be batched. For example, if you
// want to create batches of *Entry, pass reflect.TypeOf(&Entry{}) for itemType.
//
// opts can be nil to accept defaults.
//
// handler is a function that will be called on each bundle. If itemExample is
// of type T, the argument to handler is of type []T.
func New(itemType reflect.Type, opts *Options, handler func(any) error) *Batcher {
	return &Batcher{
		opts:          newOptionsWithDefaults(opts),
		handler:       handler,
		itemSliceZero: reflect.Zero(reflect.SliceOf(itemType)),
	}
}

// Add adds an item to the batcher. It blocks until the handler has
// processed the item and reports the error that the handler returned.
// If Shutdown has been called, Add immediately returns an error.
func (b *Batcher) Add(ctx context.Context, item any) error {
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
func (b *Batcher) AddNoWait(item any) <-chan error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Create a channel to receive the error from the handler.
	c := make(chan error, 1)
	if b.shutdown {
		c <- errors.New("batcher: shut down")
		return c
	}

	if b.opts.MaxBatchByteSize > 0 {
		if sizable, ok := item.(sizableItem); ok {
			if sizable.ByteSize() > b.opts.MaxBatchByteSize {
				c <- ErrMessageTooLarge
				return c
			}
		}
	}

	// Add the item to the pending list.
	b.pending = append(b.pending, waiter{item, c})
	if b.nHandlers < b.opts.MaxHandlers {
		// If we can start a handler, do so with the item just added and any others that are pending.
		batch := b.nextBatch()
		if batch != nil {
			b.wg.Add(1)
			go func() {
				b.callHandler(batch)
				b.wg.Done()
			}()
			b.nHandlers++
		}
	}
	// If we can't start a handler, then one of the currently running handlers will
	// take our item.
	return c
}

// nextBatch returns the batch to process, and updates b.pending.
// It returns nil if there's no batch ready for processing.
// b.mu must be held.
func (b *Batcher) nextBatch() []waiter {
	if len(b.pending) < b.opts.MinBatchSize {
		return nil
	}

	if b.opts.MaxBatchByteSize == 0 && (b.opts.MaxBatchSize == 0 || len(b.pending) <= b.opts.MaxBatchSize) {
		// Send it all!
		batch := b.pending
		b.pending = nil
		return batch
	}

	batch := make([]waiter, 0, len(b.pending))
	batchByteSize := 0
	for _, msg := range b.pending {
		itemByteSize := 0
		if sizable, ok := msg.item.(sizableItem); ok {
			itemByteSize = sizable.ByteSize()
		}
		reachedMaxSize := b.opts.MaxBatchSize > 0 && len(batch)+1 > b.opts.MaxBatchSize
		reachedMaxByteSize := b.opts.MaxBatchByteSize > 0 && batchByteSize+itemByteSize > b.opts.MaxBatchByteSize

		if reachedMaxSize || reachedMaxByteSize {
			break
		}
		batch = append(batch, msg)
		batchByteSize = batchByteSize + itemByteSize
	}

	b.pending = b.pending[len(batch):]
	return batch
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
		batch = b.nextBatch()
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
