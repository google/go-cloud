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

package retry

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	gax "github.com/googleapis/gax-go"
	"golang.org/x/xerrors"
)

// Errors to distinguish retryable and non-retryable cases.
var (
	errRetry   = errors.New("retry")
	errNoRetry = errors.New("no retry")
)

func retryable(err error) bool {
	return err == errRetry
}

func TestCall(t *testing.T) {
	for _, test := range []struct {
		desc        string
		isRetryable func(error) bool
		f           func(int) error // passed the number of calls so far
		wantErr     error           // the return value of call
		wantCount   int             // number of times f is called
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
			wantCount: 3,
			wantErr:   errNoRetry,
		},
		{
			desc:        "f returns context error", // same as non-retryable
			isRetryable: retryable,
			f:           func(int) error { return context.Canceled },
			wantCount:   1,
			wantErr:     context.Canceled,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			sleep := func(context.Context, time.Duration) error { return nil }
			gotCount := 0
			f := func() error { gotCount++; return test.f(gotCount - 1) }
			gotErr := call(context.Background(), gax.Backoff{}, test.isRetryable, f, sleep)
			if gotErr != test.wantErr {
				t.Errorf("error: got %v, want %v", gotErr, test.wantErr)
			}
			if gotCount != test.wantCount {
				t.Errorf("retry count: got %d, want %d", gotCount, test.wantCount)
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
		wantErr := &ContextError{CtxErr: context.Canceled}
		if !equalContextError(gotErr, wantErr) {
			t.Errorf("error: got %v, want %v", gotErr, wantErr)
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
		wantErr := &ContextError{CtxErr: context.Canceled, FuncErr: errRetry}
		if !equalContextError(gotErr, wantErr) {
			t.Errorf("error: got %v, want %v", gotErr, wantErr)
		}
	})
}

func equalContextError(got error, want *ContextError) bool {
	cerr, ok := got.(*ContextError)
	if !ok {
		return false
	}
	return cerr.CtxErr == want.CtxErr && cerr.FuncErr == want.FuncErr
}

func TestErrorsIs(t *testing.T) {
	err := &ContextError{
		CtxErr:  context.Canceled,
		FuncErr: os.ErrExist,
	}
	for _, target := range []error{err, context.Canceled, os.ErrExist} {
		if !xerrors.Is(err, target) {
			t.Errorf("xerrors.Is(%v) == false, want true", target)
		}
	}
}
