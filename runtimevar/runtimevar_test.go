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

// Package runtimevar contains tests that exercises the runtimevar APIs. It does not test
// driver implementations.
package runtimevar

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/runtimevar/driver"
)

// How long we wait on a call that is expected to block forever before cancelling it.
const blockingCheckDelay = 25 * time.Millisecond

// state implements driver.State.
type state struct {
	val        string
	updateTime time.Time
	err        error
}

func (s *state) Value() (interface{}, error) { return s.val, s.err }
func (s *state) UpdateTime() time.Time       { return s.updateTime }
func (s *state) As(i interface{}) bool       { return false }

// fakeWatcher is a fake implementation of driver.Watcher that returns a set *state.
type fakeWatcher struct {
	driver.Watcher

	mu     sync.Mutex
	state  *state
	newval bool // true iff WatchVariable should return state
}

func (w *fakeWatcher) Set(s *state) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.state = s
	w.newval = true
}

func (w *fakeWatcher) WatchVariable(ctx context.Context, prev driver.State) (driver.State, time.Duration) {
	if err := ctx.Err(); err != nil {
		w.Set(&state{err: err})
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.newval {
		return nil, 1 * time.Millisecond // to avoid spinning
	}
	w.newval = false
	return w.state, 0
}

func (*fakeWatcher) Close() error                       { return nil }
func (*fakeWatcher) ErrorCode(error) gcerrors.ErrorCode { return gcerrors.Internal }

func TestVariable_Watch(t *testing.T) {
	fake := &fakeWatcher{}
	v := New(fake)

	ctx := context.Background()

	// Watch should block when there's no value yet.
	ctx2, cancel := context.WithTimeout(ctx, blockingCheckDelay)
	defer cancel()
	if _, err := v.Watch(ctx2); err == nil {
		t.Errorf("Watch with no value yet should block: got nil err, want err")
	}
	if ctx2.Err() == nil {
		t.Error("Watch with no value yet should block")
	}

	// Setting an error value makes Watch return an error.
	fake.Set(&state{err: errFake})
	if _, err := v.Watch(ctx); err == nil {
		t.Fatal("Watch returned non-nil error, want error")
	}
	// But calling Watch again blocks.
	ctx2, cancel = context.WithTimeout(ctx, blockingCheckDelay)
	defer cancel()
	if _, err := v.Watch(ctx2); err == nil {
		t.Errorf("Watch called again with error value should block: got nil err, want err")
	}
	if ctx2.Err() == nil {
		t.Error("Watch called again with error value should block")
	}

	// Setting a different error makes Watch return again.
	fake.Set(&state{err: errors.New("another fake error")})
	if _, err := v.Watch(ctx); err == nil {
		t.Fatal("Watch returned non-nil error, want error")
	}

	// Setting a value makes Watch return again.
	fake.Set(&state{val: "hello"})
	if _, err := v.Watch(ctx); err != nil {
		t.Fatalf("Watch returned error %v, want nil", err)
	}

	// Make a few updates. Each of these will try to write to the nextWatchCh,
	// but we should only keep the latest one.
	fake.Set(&state{val: "hello1"})
	fake.Set(&state{val: "hello2"})
	fake.Set(&state{val: "hello3"})
	fake.Set(&state{val: "hello4"})
	fake.Set(&state{val: "hello5"})
	// Wait until we're sure the last one has been received.
	for {
		snap, err := v.Latest(nil)
		if err != nil {
			t.Errorf("got unexpected error from Latest: %v", err)
		}
		if snap.Value == "hello5" {
			break
		}
	}

	// Watch should get the last one, hello5.
	if snap, err := v.Watch(ctx); err != nil {
		t.Fatalf("Watch returned error %v, want nil", err)
	} else if snap.Value != "hello5" {
		t.Errorf("Watch got %v, want hello5", snap.Value)
	}

	// And the next call should block.
	ctx2, cancel = context.WithTimeout(ctx, blockingCheckDelay)
	defer cancel()
	if _, err := v.Watch(ctx2); err == nil {
		t.Errorf("Watch after no change in good value should block: got nil err, want err")
	}
	if ctx2.Err() == nil {
		t.Error("Watch after no change in good value should block")
	}

	// Close the variable.
	if err := v.Close(); err != nil {
		t.Error(err)
	}
	// Watch should now return io.EOF.
	if _, err := v.Watch(ctx); err != io.EOF {
		t.Errorf("Watch after close returned %v, want io.EOF", err)
	}
}

func TestVariable_Latest(t *testing.T) {
	const content1, content2 = "foo", "bar"
	const numGoroutines = 10

	fake := &fakeWatcher{}
	v := New(fake)
	defer v.Close()

	ctx := context.Background()

	// Latest with nil ctx should return right away with an error, as there's
	// no value yet.
	if _, err := v.Latest(nil); err == nil {
		t.Errorf("Latest with nil ctx but no value yet: got nil err, want err")
	}

	// Latest with non-nil ctx should block until the context is done, as there's
	// still no value.
	ctx2, cancel := context.WithTimeout(ctx, blockingCheckDelay)
	defer cancel()
	if _, err := v.Latest(ctx2); err == nil {
		t.Errorf("Latest with non-nil ctx but no value yet should block: got nil err, want err")
	}
	if ctx2.Err() == nil {
		t.Error("Latest with non-nil ctx but no value yet should block")
	}

	// Call Latest concurrently. There's still no value.
	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			if _, err := v.Latest(nil); err == nil {
				t.Errorf("Latest with nil ctx but no value yet: got nil err, want err")
			}
			wg.Done()
		}()
	}
	wg.Wait()

	// Set an error value. Latest should still block.
	fake.Set(&state{err: errFake})
	ctx2, cancel = context.WithTimeout(ctx, blockingCheckDelay)
	defer cancel()
	if _, err := v.Latest(ctx2); err == nil {
		t.Errorf("Latest with non-nil ctx but error value should block: got nil err, want err")
	}
	if ctx2.Err() == nil {
		t.Error("Latest with non-nil ctx but error value should block")
	}

	// Set a good value.
	fake.Set(&state{val: content1})

	// Call Latest concurrently, only exiting each goroutine when they
	// see the content1 value.
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			for {
				val, err := v.Latest(ctx)
				if err != nil {
					continue
				}
				if val.Value != content1 {
					t.Errorf("got %v want %s", val, content1)
				}
				wg.Done()
				return
			}
		}()
	}
	wg.Wait()

	// Set a different value. At some point after this, Latest should start
	// returning a Snapshot with Value set to content2.
	fake.Set(&state{val: content2})

	// Call Latest concurrently, only exiting each goroutine when they
	// see the content2 value.
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			for {
				val, err := v.Latest(nil)
				if err != nil {
					// Errors are unexpected at this point.
					t.Error(err)
				}
				if val.Value == content1 {
					// Still seeing the old value.
					continue
				}
				if val.Value != content2 {
					t.Errorf("got %v want %s", val, content2)
				}
				wg.Done()
				return
			}
		}()
	}
	wg.Wait()

	// Set an error value. Latest should still return content2.
	fake.Set(&state{err: errFake})

	// Call Latest concurrently. The test will be flaky if some of them
	// start getting errors.
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			for {
				val, err := v.Latest(nil)
				if err != nil {
					// Errors are unexpected at this point.
					t.Error(err)
				}
				if val.Value != content2 {
					t.Errorf("got %v want %s", val, content2)
				}
				wg.Done()
				return
			}
		}()
	}
	wg.Wait()
}

var errFake = errors.New("fake")

// erroringWatcher implements driver.Watcher.
// WatchVariable always returns a state with errFake, and Close
// always returns errFake.
type erroringWatcher struct {
	driver.Watcher
}

func (b *erroringWatcher) WatchVariable(ctx context.Context, prev driver.State) (driver.State, time.Duration) {
	return &state{err: errFake}, 0
}

func (b *erroringWatcher) Close() error {
	return errFake
}

func (b *erroringWatcher) ErrorCode(err error) gcerrors.ErrorCode {
	return gcerrors.Internal
}

// TestErrorsAreWrapped tests that all errors returned from the driver are
// wrapped exactly once by the concrete type.
func TestErrorsAreWrapped(t *testing.T) {
	ctx := context.Background()
	v := New(&erroringWatcher{})

	// verifyWrap ensures that err is wrapped exactly once.
	verifyWrap := func(description string, err error) {
		if unwrapped, ok := err.(*gcerr.Error); !ok {
			t.Errorf("%s: not wrapped: %v", description, err)
		} else if du, ok := unwrapped.Unwrap().(*gcerr.Error); ok {
			t.Errorf("%s: double wrapped: %v", description, du)
		}
	}

	_, err := v.Watch(ctx)
	verifyWrap("Watch", err)

	err = v.Close()
	verifyWrap("Close", err)
}

var (
	testOpenOnce sync.Once
	testOpenGot  *url.URL
)

func TestURLMux(t *testing.T) {
	ctx := context.Background()
	var got *url.URL

	mux := new(URLMux)
	// Register scheme foo to always return nil. Sets got as a side effect
	mux.RegisterVariable("foo", variableURLOpenFunc(func(_ context.Context, u *url.URL) (*Variable, error) {
		got = u
		return nil, nil
	}))
	// Register scheme err to always return an error.
	mux.RegisterVariable("err", variableURLOpenFunc(func(_ context.Context, u *url.URL) (*Variable, error) {
		return nil, errors.New("fail")
	}))

	for _, tc := range []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "empty URL",
			wantErr: true,
		},
		{
			name:    "invalid URL",
			url:     ":foo",
			wantErr: true,
		},
		{
			name:    "invalid URL no scheme",
			url:     "foo",
			wantErr: true,
		},
		{
			name:    "unregistered scheme",
			url:     "bar://myvar",
			wantErr: true,
		},
		{
			name:    "func returns error",
			url:     "err://myvar",
			wantErr: true,
		},
		{
			name: "no query options",
			url:  "foo://myvar",
		},
		{
			name: "empty query options",
			url:  "foo://myvar?",
		},
		{
			name: "query options",
			url:  "foo://myvar?aAa=bBb&cCc=dDd",
		},
		{
			name: "multiple query options",
			url:  "foo://myvar?x=a&x=b&x=c",
		},
		{
			name: "fancy var name",
			url:  "foo:///foo/bar/baz",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, gotErr := mux.OpenVariable(ctx, tc.url)
			if (gotErr != nil) != tc.wantErr {
				t.Fatalf("got err %v, want error %v", gotErr, tc.wantErr)
			}
			if gotErr != nil {
				return
			}
			want, err := url.Parse(tc.url)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(got, want); diff != "" {
				t.Errorf("got\n%v\nwant\n%v\ndiff\n%s", got, want, diff)
			}
		})
	}
}

type variableURLOpenFunc func(context.Context, *url.URL) (*Variable, error)

func (f variableURLOpenFunc) OpenVariableURL(ctx context.Context, u *url.URL) (*Variable, error) {
	return f(ctx, u)
}

func TestDecoder(t *testing.T) {
	type Struct struct {
		FieldA string
		FieldB map[string]interface{}
	}

	num := 4321
	numptr := &num
	str := "boring string"
	strptr := &str

	inputs := []interface{}{
		str,
		strptr,
		num,
		numptr,
		100.1,
		Struct{
			FieldA: "hello",
			FieldB: map[string]interface{}{
				"hello": "world",
			},
		},
		&Struct{
			FieldA: "world",
		},
		map[string]string{
			"slice": "pizza",
		},
		&map[string]interface{}{},
		[]string{"hello", "world"},
		&[]int{1, 0, 1},
		[...]float64{3.1415},
		&[...]int64{4, 5, 6},
	}

	for _, tc := range []struct {
		desc     string
		encodeFn func(interface{}) ([]byte, error)
		decodeFn Decode
	}{
		{
			desc:     "JSON",
			encodeFn: json.Marshal,
			decodeFn: JSONDecode,
		},
		{
			desc:     "Gob",
			encodeFn: gobMarshal,
			decodeFn: GobDecode,
		},
	} {
		for i, input := range inputs {
			t.Run(fmt.Sprintf("%s_%d", tc.desc, i), func(t *testing.T) {
				decoder := NewDecoder(input, tc.decodeFn)
				b, err := tc.encodeFn(input)
				if err != nil {
					t.Fatalf("marshal error %v", err)
				}
				got, err := decoder.Decode(b)
				if err != nil {
					t.Fatalf("parse input\n%s\nerror: %v", string(b), err)
				}
				if reflect.TypeOf(got) != reflect.TypeOf(input) {
					t.Errorf("type mismatch got %T, want %T", got, input)
				}
				if diff := cmp.Diff(got, input); diff != "" {
					t.Errorf("value diff:\n%v", diff)
				}
			})
		}
	}
}

func gobMarshal(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func TestStringDecoder(t *testing.T) {
	input := "hello world"
	got, err := StringDecoder.Decode([]byte(input))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if input != got.(string) {
		t.Errorf("output got %v, want %q", got, input)
	}
}

func TestBytesDecoder(t *testing.T) {
	input := []byte("hello world")
	got, err := BytesDecoder.Decode(input)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if diff := cmp.Diff(got, input); diff != "" {
		t.Errorf("output got %v, want %q", got, input)
	}
}
