// Copyright 2019 The Go Cloud Development Kit Authors
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

package workerpool_test

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"gocloud.dev/internal/workerpool"
)

func TestRun(t *testing.T) {
	t.Run("can be canceled", func(t *testing.T) {
		ng0 := runtime.NumGoroutine()
		ctx, cancel := context.WithCancel(context.Background())
		nextTask := func(context.Context) func(context.Context) {
			return func(context.Context) {
				<-ctx.Done()
			}
		}
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			workerpool.Run(ctx, 1, nextTask)
			wg.Done()
		}()
		cancel()
		wg.Wait()
		t.Log("waiting for goroutines to return")
		for runtime.NumGoroutine() != ng0 {
			time.Sleep(time.Millisecond)
		}
	})

	t.Run("can run all tasks", func(t *testing.T) {
		ng0 := runtime.NumGoroutine()
		i := 0
		n := 100
		// m keeps track of which tasks have been completed.
		var mu sync.Mutex
		m := make(map[int]bool)
		nextTask := func(context.Context) func(context.Context) {
			i++
			if i > n {
				return nil
			}
			return func(i int) func(context.Context) {
				return func(context.Context) {
					mu.Lock()
					m[i] = true
					mu.Unlock()
				}
			}(i)
		}

		workerpool.Run(context.Background(), 10, nextTask)
		t.Log("waiting for tasks to finish")
		for {
			mu.Lock()
			if len(m) == n {
				break
			}
			mu.Unlock()
			time.Sleep(time.Millisecond)
		}
		t.Log("waiting for goroutines to return")
		for runtime.NumGoroutine() != ng0 {
			time.Sleep(time.Millisecond)
		}
	})
}
