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

package batcher

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestBatcherSequential(t *testing.T) {
	// Verify that sequential non-concurrent Adds to a batcher produce single-item batches.
	// Since there is no concurrent work, the Batcher will always produce the items one at a time.
	ctx := context.Background()
	var got []int
	e := errors.New("e")
	b := New(int(0), 1, func(items interface{}) error {
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

func TestBatcherSaturation(t *testing.T) {
	// Verify that under high load the maximum number of handlers are running.
	ctx := context.Background()
	const maxHandlers = 10
	var (
		mu               sync.Mutex
		outstanding, max int             // number of handlers
		maxBatch         int             // size of largest batch
		count            = map[int]int{} // how many of each item the handlers observe
	)
	b := New(int(0), maxHandlers, func(x interface{}) error {
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
	if maxBatch <= 1 {
		t.Errorf("got max batch size of %d, expected > 1", maxBatch)
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
