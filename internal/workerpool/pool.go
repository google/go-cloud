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

// Package workerpool provides a func called Run that allows doing work
// concurrently, only spawning goroutines as needed and not spawning more of
// them than a set limit. The idea for this package comes from Bryan C. Mills's
// talk: https://www.youtube.com/watch?v=5zXAHh5tJqQ&t=33m13s
package workerpool

import (
	"context"
	"sync"
)

type Task func(context.Context) error

// Run runs a worker pool with no more than limit goroutines. It repeatedly
// calls nextTask to get tasks to perform. These tasks are in the form of funcs
// that do some work via side effects. If nextTask returns nil, Run will no
// longer ask for more tasks, and will return after waiting for the running
// goroutines to finish. The provided context can be used to cancel everything
// and exit the loop.
func Run(ctx context.Context, limit int, nextTask func(context.Context) (Task, error)) error {
	ctx, cancel := context.WithCancel(ctx)
	type token struct{}
	sem := make(chan token, limit)
	var mu sync.Mutex
	var retErr error
	setErr := func(err error) {
		mu.Lock()
		if retErr == nil {
			retErr = err
		}
		mu.Unlock()
		cancel()
	}
Loop:
	for {
		doTask, err := nextTask(ctx)
		if err != nil {
			setErr(err)
			break
		}
		select {
		case <-ctx.Done():
			setErr(ctx.Err())
			break Loop
		case sem <- token{}:
		}
		go func() {
			if err := doTask(ctx); err != nil {
				setErr(err)
			}
			<-sem
		}()
	}

	// Wait for the worker goroutines to finish. This means that if doWork
	// hangs then Run will hang. The benefit is that if doWork is
	// well-behaved then Run does not leak any goroutines.
	for n := limit; n > 0; n-- {
		sem <- token{}
	}

	return retErr
}
