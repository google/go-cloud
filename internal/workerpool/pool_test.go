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
	"errors"
	"fmt"
	"math/rand"
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
		nextTask := func(context.Context) (workerpool.Task, error) {
			task := func(context.Context) error {
				<-ctx.Done()
				return nil
			}
			return task, nil
		}
		errc := make(chan error)
		go func() {
			errc <- workerpool.Run(ctx, 1, nextTask)
		}()
		cancel()
		err := <-errc
		if err != context.Canceled {
			t.Errorf("workerpool.Run returned %v, want context.Canceled", err)
		}
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
		wantErr := errors.New("too many tasks created")
		nextTask := func(context.Context) (workerpool.Task, error) {
			i++
			if i > n {
				return nil, wantErr
			}
			task := func(i int) workerpool.Task {
				return func(context.Context) error {
					mu.Lock()
					m[i] = true
					mu.Unlock()
					return nil
				}
			}(i)
			return task, nil
		}

		err := workerpool.Run(context.Background(), 10, nextTask)
		if err != wantErr {
			t.Errorf("returned error is %v, want %v", err, wantErr)
		}
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

	t.Run("returns error if a task returns error", func(t *testing.T) {
		ng0 := runtime.NumGoroutine()
		ctx := context.Background()
		wantErr := fmt.Errorf("error-%d", rand.Int())
		nextTask := func(context.Context) (workerpool.Task, error) {
			task := func(context.Context) error {
				return wantErr
			}
			return task, nil
		}
		err := workerpool.Run(ctx, 1, nextTask)
		if err != wantErr {
			t.Errorf(`workerpool.Run returned "%v" for error, want "%v"`, err, wantErr)
		}
		t.Log("waiting for goroutines to return")
		for runtime.NumGoroutine() != ng0 {
			time.Sleep(time.Millisecond)
		}
	})

	t.Run("returns error if task getter returns an error", func(t *testing.T) {
		ng0 := runtime.NumGoroutine()
		ctx := context.Background()
		wantErr := fmt.Errorf("error-%d", rand.Int())
		nextTask := func(context.Context) (workerpool.Task, error) {
			return nil, wantErr
		}
		err := workerpool.Run(ctx, 1, nextTask)
		if err != wantErr {
			t.Errorf(`workerpool.Run returned "%v" for error, want "%v"`, err, wantErr)
		}
		t.Log("waiting for goroutines to return")
		for runtime.NumGoroutine() != ng0 {
			time.Sleep(time.Millisecond)
		}
	})

	t.Run("cancels if nextTask returns an error", func(t *testing.T) {
		ctx := context.Background()
		wantErr := errors.New("no more tasks")
		tasks := []workerpool.Task{
			func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
		}
		nextTask := func(context.Context) (workerpool.Task, error) {
			if len(tasks) == 0 {
				return nil, wantErr
			}
			task := tasks[0]
			tasks = tasks[1:]
			return task, nil
		}
		err := workerpool.Run(ctx, 2, nextTask)
		if err != wantErr {
			t.Errorf(`workerpool.Run returned "%v" for error, want "%v"`, err, wantErr)
		}
	})
}
