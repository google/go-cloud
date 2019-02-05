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
		type Task struct{}
		nextTask := func(context.Context) interface{} {
			return Task{}
		}
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			workerpool.Run(ctx, 1, nextTask, func(ctx context.Context, _ interface{}) {
				<-ctx.Done()
			})
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
		type Task struct{ i int }
		nextTask := func(context.Context) interface{} {
			i++
			if i > n {
				return nil
			}
			return Task{i}
		}

		// m keeps track of which tasks have yet to be completed.
		var mu sync.Mutex
		m := make(map[int]bool)
		for i := 1; i <= n; i++ {
			m[i] = true
		}

		workerpool.Run(context.Background(), 10, nextTask, func(ctx context.Context, ti interface{}) {
			t := ti.(Task)
			mu.Lock()
			defer mu.Unlock()
			delete(m, t.i)
		})
		t.Log("waiting for tasks to finish")
		for {
			mu.Lock()
			if len(m) == 0 {
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
