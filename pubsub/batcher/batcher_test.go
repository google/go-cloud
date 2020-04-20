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

package batcher_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/pubsub/batcher"
)

func TestSplit(t *testing.T) {
	tests := []struct {
		n    int
		opts *batcher.Options
		want []int
	}{
		// Defaults.
		{0, nil, nil},
		{1, nil, []int{1}},
		{10, nil, []int{10}},
		// MinBatchSize.
		{4, &batcher.Options{MinBatchSize: 5}, nil},
		{8, &batcher.Options{MinBatchSize: 5, MaxBatchSize: 7}, []int{7}},
		// <= MaxBatchSize.
		{5, &batcher.Options{MaxBatchSize: 5}, []int{5}},
		{9, &batcher.Options{MaxBatchSize: 10}, []int{9}},
		// > MaxBatchSize with MaxHandlers = 1.
		{5, &batcher.Options{MaxBatchSize: 4}, []int{4}},
		{999, &batcher.Options{MaxBatchSize: 10}, []int{10}},
		// MaxBatchSize with MaxHandlers > 1.
		{10, &batcher.Options{MaxBatchSize: 4, MaxHandlers: 2}, []int{4, 4}},
		{10, &batcher.Options{MaxBatchSize: 5, MaxHandlers: 2}, []int{5, 5}},
		{10, &batcher.Options{MaxBatchSize: 9, MaxHandlers: 2}, []int{9, 1}},
		{9, &batcher.Options{MaxBatchSize: 4, MaxHandlers: 3}, []int{4, 4, 1}},
		{10, &batcher.Options{MaxBatchSize: 4, MaxHandlers: 3}, []int{4, 4, 2}},
		// All 3 options together.
		{8, &batcher.Options{MinBatchSize: 5, MaxBatchSize: 7, MaxHandlers: 2}, []int{7}},
	}

	for _, test := range tests {
		got := batcher.Split(test.n, test.opts)
		if diff := cmp.Diff(got, test.want); diff != "" {
			t.Errorf("%d/%#v: got %v want %v diff %s", test.n, test.opts, got, test.want, diff)
		}
	}
}

func TestSequential(t *testing.T) {
	// Verify that sequential non-concurrent Adds to a batcher produce single-item batches.
	// Since there is no concurrent work, the Batcher will always produce the items one at a time.
	ctx := context.Background()
	var got []int
	e := errors.New("e")
	b := batcher.New(reflect.TypeOf(int(0)), nil, func(items interface{}) error {
		got = items.([]int)
		return e
	})
	for i := 0; i < 10; i++ {
		err := b.Add(ctx, i)
		if err != e {
			t.Errorf("got %v, want %v", err, e)
		}
		want := []int{i}
		if !cmp.Equal(got, want) {
			t.Errorf("got %+v, want %+v", got, want)
		}
	}
}

func TestMinBatchSize(t *testing.T) {
	// Verify the MinBatchSize option works.
	var got [][]int
	b := batcher.New(reflect.TypeOf(int(0)), &batcher.Options{MinBatchSize: 3}, func(items interface{}) error {
		got = append(got, items.([]int))
		return nil
	})
	for i := 0; i < 6; i++ {
		b.AddNoWait(i)
	}
	b.Shutdown()
	want := [][]int{{0, 1, 2}, {3, 4, 5}}
	if !cmp.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestSaturation(t *testing.T) {
	// Verify that under high load the maximum number of handlers are running.
	ctx := context.Background()
	const (
		maxHandlers  = 10
		maxBatchSize = 50
	)
	var (
		mu               sync.Mutex
		outstanding, max int             // number of handlers
		maxBatch         int             // size of largest batch
		count            = map[int]int{} // how many of each item the handlers observe
	)
	b := batcher.New(reflect.TypeOf(int(0)), &batcher.Options{MaxHandlers: maxHandlers, MaxBatchSize: maxBatchSize}, func(x interface{}) error {
		items := x.([]int)
		mu.Lock()
		outstanding++
		if outstanding > max {
			max = outstanding
		}
		for _, x := range items {
			count[x]++
		}
		if len(items) > maxBatch {
			maxBatch = len(items)
		}
		mu.Unlock()
		defer func() { mu.Lock(); outstanding--; mu.Unlock() }()
		// Sleep a little to increase the likelihood of saturation.
		time.Sleep(10 * time.Millisecond)
		return nil
	})
	var wg sync.WaitGroup
	const nItems = 1000
	for i := 0; i < nItems; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Sleep a little to increase the likelihood of saturation.
			time.Sleep(time.Millisecond)
			if err := b.Add(ctx, i); err != nil {
				t.Errorf("b.Add(ctx, %d) error: %v", i, err)
			}
		}()
	}
	wg.Wait()
	// Check that we saturated the batcher.
	if max != maxHandlers {
		t.Errorf("max concurrent handlers = %d, want %d", max, maxHandlers)
	}
	// Check that at least one batch had more than one item.
	if maxBatch <= 1 || maxBatch > maxBatchSize {
		t.Errorf("got max batch size of %d, expected > 1 and <= %d", maxBatch, maxBatchSize)
	}
	// Check that handlers saw every item exactly once.
	want := map[int]int{}
	for i := 0; i < nItems; i++ {
		want[i] = 1
	}
	if diff := cmp.Diff(count, want); diff != "" {
		t.Errorf("items: %s", diff)
	}
}

func TestShutdown(t *testing.T) {
	ctx := context.Background()
	var nHandlers int64 // atomic
	c := make(chan int, 10)
	b := batcher.New(reflect.TypeOf(int(0)), &batcher.Options{MaxHandlers: cap(c)}, func(x interface{}) error {
		for range x.([]int) {
			c <- 0
		}
		atomic.AddInt64(&nHandlers, 1)
		defer atomic.AddInt64(&nHandlers, -1)
		time.Sleep(time.Second) // we want handlers to be active on Shutdown
		return nil
	})
	for i := 0; i < cap(c); i++ {
		go func() {
			err := b.Add(ctx, 0)
			if err != nil {
				t.Errorf("b.Add error: %v", err)
			}
		}()
	}
	// Make sure all goroutines have started.
	for i := 0; i < cap(c); i++ {
		<-c
	}
	b.Shutdown()

	if got := atomic.LoadInt64(&nHandlers); got != 0 {
		t.Fatalf("%d Handlers still active after Shutdown returns", got)
	}
	if err := b.Add(ctx, 1); err == nil {
		t.Error("got nil, want error from Add after Shutdown")
	}
}

func TestItemCanBeInterface(t *testing.T) {
	readerType := reflect.TypeOf([]io.Reader{}).Elem()
	called := false
	b := batcher.New(readerType, nil, func(items interface{}) error {
		called = true
		_, ok := items.([]io.Reader)
		if !ok {
			t.Fatal("items is not a []io.Reader")
		}
		return nil
	})
	b.Add(context.Background(), &bytes.Buffer{})
	if !called {
		t.Fatal("handler not called")
	}
}
