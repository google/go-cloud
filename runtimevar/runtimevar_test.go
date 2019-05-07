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
	"net/url"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/runtimevar/driver"
	"gocloud.dev/secrets/localsecrets"
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
		snap, err := v.Latest(ctx)
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

	// Ensure a blocking Watch returns when a new value arrives.
	// Wait blockingCheckDelay to give some time to be blocking in Watch.
	// There's no guarantee it will get there, but if Watch doesn't handle
	// cancelation properly, then the test will fail whenever it does get there,
	// so at least we'll observe a flaky test.
	time.AfterFunc(blockingCheckDelay, func() { fake.Set(&state{val: "hello6"}) })
	if snap, err := v.Watch(ctx); err != nil {
		t.Errorf("Watch interrupted by new value returned %v, want nil", err)
	} else if snap.Value != "hello6" {
		t.Errorf("Watch got %v, want hello6", snap.Value)
	}

	// Similarly, ensure a blocking Watch is interrupted by Close.
	time.AfterFunc(blockingCheckDelay, func() {
		if err := v.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := v.Watch(ctx); err != ErrClosed {
		t.Errorf("Watch interrupted by Close returned %v, want ErrClosed", err)
	}

	// Watch should now return ErrClosed.
	if _, err := v.Watch(ctx); err != ErrClosed {
		t.Errorf("Watch after Close returned %v, want ErrClosed", err)
	}
}

func TestVariable_Latest(t *testing.T) {
	const content1, content2 = "foo", "bar"
	const numGoroutines = 10
	ctx := context.Background()

	fake := &fakeWatcher{}
	v := New(fake)

	// Not healthy at startup.
	if v.CheckHealth() == nil {
		t.Error("got nil from CheckHealth, want error")
	}

	// Latest should block until the context is done, as there's no value.
	ctx2, cancel := context.WithTimeout(ctx, blockingCheckDelay)
	defer cancel()
	if _, err := v.Latest(ctx2); err == nil {
		t.Errorf("Latest with no value yet should block: got nil err, want err")
	}
	if ctx2.Err() == nil {
		t.Error("Latest with no value yet should block")
	}
	// And we're not healthy.
	if v.CheckHealth() == nil {
		t.Error("got nil from CheckHealth, want error")
	}

	// Call Latest concurrently. There's still no value.
	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			ctx2, cancel := context.WithTimeout(ctx, blockingCheckDelay)
			cancel()
			if _, err := v.Latest(ctx2); err == nil {
				t.Errorf("Latest with no value yet: got nil err, want err")
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
		t.Errorf("Latest with error value should block: got nil err, want err")
	}
	if ctx2.Err() == nil {
		t.Error("Latest with error value should block")
	}
	// And we're still not healthy.
	if v.CheckHealth() == nil {
		t.Error("got nil from CheckHealth, want error")
	}

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
	// Set a good value, after a small delay to give the goroutines a chance
	// to get into Latest.
	time.Sleep(blockingCheckDelay)
	fake.Set(&state{val: content1})
	wg.Wait()
	// And now we're healthy.
	if err := v.CheckHealth(); err != nil {
		t.Errorf("got %v from CheckHealth, want nil", err)
	}

	// Set a different value. At some point after this, Latest should start
	// returning a Snapshot with Value set to content2.
	fake.Set(&state{val: content2})

	// Call Latest concurrently, only exiting each goroutine when they
	// see the content2 value.
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			for {
				val, err := v.Latest(ctx)
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
				val, err := v.Latest(ctx)
				if err != nil {
					// Errors are unexpected at this point.
					t.Error(err)
				} else if val.Value != content2 {
					t.Errorf("got %v want %s", val.Value, content2)
				}
				wg.Done()
				return
			}
		}()
	}
	wg.Wait()

	// Still healthy.
	if err := v.CheckHealth(); err != nil {
		t.Errorf("got %v from CheckHealth, want nil", err)
	}

	// Close the variable.
	if err := v.Close(); err != nil {
		t.Error(err)
	}

	// Latest should now return ErrClosed.
	if _, err := v.Latest(ctx); err != ErrClosed {
		t.Errorf("Latest after close returned %v, want ErrClosed", err)
	}
	// Unhealthy now.
	if err := v.CheckHealth(); err != ErrClosed {
		t.Errorf("got %v from CheckHealth, want ErrClosed", err)
	}
}

// Tests that Latest is interrupted by Close.
func TestVariable_LatestBlockedDuringClose(t *testing.T) {
	fake := &fakeWatcher{}
	v := New(fake)

	ctx := context.Background()

	// Wait blockingCheckDelay to give some time to be blocking in Latest.
	// There's no guarantee it will get there, but if Latest doesn't handle
	// cancelation properly, then the test will fail whenever it does get there,
	// so at least we'll observe a flaky test.
	time.AfterFunc(blockingCheckDelay, func() {
		if err := v.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := v.Latest(ctx); err != ErrClosed {
		t.Errorf("Latest interrupted by Close got %v, want ErrClosed", err)
	}

	// Calling Close again should return ErrClosed.
	if err := v.Close(); err != ErrClosed {
		t.Errorf("calling Close 2x returned %v, want ErrClosed", err)
	}
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
// wrapped exactly once by the portable type.
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

	mux := new(URLMux)
	fake := &fakeOpener{}
	mux.RegisterVariable("foo", fake)
	mux.RegisterVariable("err", fake)

	if diff := cmp.Diff(mux.VariableSchemes(), []string{"err", "foo"}); diff != "" {
		t.Errorf("Schemes: %s", diff)
	}
	if !mux.ValidVariableScheme("foo") || !mux.ValidVariableScheme("err") {
		t.Errorf("ValidVariableScheme didn't return true for valid scheme")
	}
	if mux.ValidVariableScheme("foo2") || mux.ValidVariableScheme("http") {
		t.Errorf("ValidVariableScheme didn't return false for invalid scheme")
	}

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
		{
			name: "using api scheme prefix",
			url:  "runtimevar+foo:///foo/bar/baz",
		},
		{
			name: "using api+type scheme prefix",
			url:  "runtimevar+variable+foo:///foo/bar/baz",
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
			if got := fake.u.String(); got != tc.url {
				t.Errorf("got %q want %q", got, tc.url)
			}
			// Repeat with OpenVariableURL.
			parsed, err := url.Parse(tc.url)
			if err != nil {
				t.Fatal(err)
			}
			_, gotErr = mux.OpenVariableURL(ctx, parsed)
			if gotErr != nil {
				t.Fatalf("got err %v, want nil", gotErr)
			}
			if got := fake.u.String(); got != tc.url {
				t.Errorf("got %q want %q", got, tc.url)
			}
		})
	}
}

type fakeOpener struct {
	u *url.URL // last url passed to OpenVariableURL
}

func (o *fakeOpener) OpenVariableURL(ctx context.Context, u *url.URL) (*Variable, error) {
	if u.Scheme == "err" {
		return nil, errors.New("fail")
	}
	o.u = u
	return nil, nil
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
				got, err := decoder.Decode(context.Background(), b)
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
	got, err := StringDecoder.Decode(context.Background(), []byte(input))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if input != got.(string) {
		t.Errorf("output got %v, want %q", got, input)
	}
}

func TestBytesDecoder(t *testing.T) {
	input := []byte("hello world")
	got, err := BytesDecoder.Decode(context.Background(), input)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if diff := cmp.Diff(got, input); diff != "" {
		t.Errorf("output got %v, want %q", got, input)
	}
}

func TestDecryptDecoder(t *testing.T) {
	ctx := context.Background()
	secretKey, err := localsecrets.NewRandomKey()
	if err != nil {
		t.Fatal(err)
	}
	keeper := localsecrets.NewKeeper(secretKey)

	tests := []struct {
		desc      string
		in        interface{}
		encodeFn  func(interface{}) ([]byte, error)
		postDecFn Decode
	}{
		{
			desc:     "Bytes",
			in:       []byte("hello world"),
			encodeFn: func(obj interface{}) ([]byte, error) { return obj.([]byte), nil },
		},
		{
			desc:      "String",
			in:        "hello world",
			encodeFn:  func(obj interface{}) ([]byte, error) { return []byte(obj.(string)), nil },
			postDecFn: StringDecode,
		},
		{
			desc: "JSON",
			in: map[string]string{
				"slice": "pizza",
			},
			encodeFn:  json.Marshal,
			postDecFn: JSONDecode,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			decoder := NewDecoder(tc.in, DecryptDecode(keeper, tc.postDecFn))

			b, err := tc.encodeFn(tc.in)
			if err != nil {
				t.Fatalf("encode error %v", err)
			}
			encrypted, err := keeper.Encrypt(ctx, b)
			if err != nil {
				t.Fatalf("encrypt error: %v", err)
			}

			got, err := decoder.Decode(ctx, encrypted)
			if err != nil {
				t.Fatalf("parse input\n%s\nerror: %v", string(b), err)
			}
			if reflect.TypeOf(got) != reflect.TypeOf(tc.in) {
				t.Errorf("type mismatch got %T, want %T", got, tc.in)
			}
			if diff := cmp.Diff(got, tc.in); diff != "" {
				t.Errorf("value diff:\n%v", diff)
			}
		})
	}
}
