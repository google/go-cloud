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

package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	gax "github.com/googleapis/gax-go"
)

// Used to indicate a duration returned from the isRetryable function.
// gax.Backoff.Pause never returns negative durations.
const retryDur = time.Duration(-1)

// Errors to distinguish retryable and non-retryable cases.
var (
	errRetry   = errors.New("retry")
	errNoRetry = errors.New("no retry")
)

func retryable(err error) (bool, time.Duration) {
	if err == errRetry {
		return true, retryDur
	}
	return false, 0
}

func TestCall(t *testing.T) {
	for _, test := range []struct {
		desc             string
		isRetryable      func(error) (bool, time.Duration)
		f                func(int) error // passed the number of calls so far
		wantErr          error           // the return value of call
		wantCount        int             // number of times f is called
		wantRetryableDur bool            // whether the duration comes from isRetryable or not
	}{
		{
			desc:        "f returns nil",
			isRetryable: retryable,
			f:           func(int) error { return nil },
			wantCount:   1,
			wantErr:     nil,
		},
		{
			desc:        "f returns non-retryable error",
			isRetryable: retryable,
			f:           func(int) error { return errNoRetry },
			wantCount:   1,
			wantErr:     errNoRetry,
		},
		{
			desc:        "f returns retryable error",
			isRetryable: retryable,
			f: func(n int) error {
				if n < 2 {
					return errRetry
				}
				return errNoRetry
			},
			wantCount:        3,
			wantErr:          errNoRetry,
			wantRetryableDur: true,
		},
		{
			desc:        "f returns context error", // same as non-retryable
			isRetryable: retryable,
			f:           func(int) error { return context.Canceled },
			wantCount:   1,
			wantErr:     context.Canceled,
		},
		{
			desc: "backoff from gax",
			isRetryable: func(err error) (bool, time.Duration) {
				if err == errRetry {
					return true, 0
				}
				return false, 0
			},
			f: func(n int) error {
				if n == 0 {
					return errRetry
				}
				return errNoRetry
			},
			wantCount:        2,
			wantErr:          errNoRetry,
			wantRetryableDur: false,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			var gotDur time.Duration
			sleep := func(_ context.Context, dur time.Duration) error { gotDur = dur; return nil }
			gotCount := 0
			f := func() error { gotCount++; return test.f(gotCount - 1) }
			gotErr := call(context.Background(), gax.Backoff{}, test.isRetryable, f, sleep)
			if gotErr != test.wantErr {
				t.Errorf("error: got %v, want %v", gotErr, test.wantErr)
			}
			if gotCount != test.wantCount {
				t.Errorf("retry count: got %d, want %d", gotCount, test.wantCount)
			}
			if (gotDur < 0) != test.wantRetryableDur {
				t.Errorf("sleep duration: got %s, want retryable dur: %t", gotDur, test.wantRetryableDur)
			}
		})
	}
}

func TestCallCancel(t *testing.T) {
	t.Run("done on entry", func(t *testing.T) {
		// If the context is done on entry, f is never called.
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		gotCount := 0
		f := func() error { gotCount++; return nil }
		gotErr := call(ctx, gax.Backoff{}, retryable, f, nil)
		if gotCount != 0 {
			t.Errorf("retry count: got %d, want 0", gotCount)
		}
		if gotErr != context.Canceled {
			t.Errorf("error: got %v, want context.Canceled", gotErr)
		}
	})
	t.Run("done in sleep", func(t *testing.T) {
		// If the context is done during sleep, we get a ContextError.
		gotCount := 0
		f := func() error { gotCount++; return errRetry }
		sleep := func(context.Context, time.Duration) error { return context.Canceled }
		gotErr := call(context.Background(), gax.Backoff{}, retryable, f, sleep)
		if gotCount != 1 {
			t.Errorf("retry count: got %d, want 1", gotCount)
		}
		cerr, ok := gotErr.(*ContextError)
		if !ok {
			t.Fatalf("got error %v, not a *ContextError", gotErr)
		}
		if cerr.CtxErr != context.Canceled {
			t.Errorf("CtxErr: got %v, want context.Canceled", cerr.CtxErr)
		}
		if cerr.FuncErr != errRetry {
			t.Errorf("FuncErr: got %v, want errRetry", cerr.FuncErr)
		}
	})
}
