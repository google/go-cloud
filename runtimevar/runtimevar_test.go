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
)

// state implements driver.State.
type state struct {
	val        string
	updateTime time.Time
	err        error
}

func (s *state) Value() (interface{}, error) {
	return s.val, s.err
}

func (s *state) UpdateTime() time.Time {
	return s.updateTime
}

func (s *state) As(i interface{}) bool {
	return false
}

// fakeWatcher is a fake implementation of driver.Watcher.
type fakeWatcher struct {
	t     *testing.T
	calls []*state
}

func (w *fakeWatcher) WatchVariable(ctx context.Context, prev driver.State) (driver.State, time.Duration) {
	w.t.Logf("fakewatcher.WatchVariable prev %v", prev)
	if len(w.calls) == 0 {
		w.t.Fatal("  --> unexpected call!")
	}
	c := w.calls[0]
	w.calls = w.calls[1:]
	if c.err != nil {
		if prev != nil && prev.(*state).err != nil && prev.(*state).err.Error() == c.err.Error() {
			w.t.Log("  --> same error")
			return nil, 0
		}
		w.t.Logf("  -> new error %v", c.err)
		return c, 0
	}
	if prev != nil && prev.(*state).val == c.val {
		w.t.Log("  --> same value")
		return nil, 0
	}
	w.t.Logf("  --> returning %v", c)
	return c, 0
}

func (w *fakeWatcher) Close() error {
	if len(w.calls) != 0 {
		return fmt.Errorf("expected %d more calls to WatchVariable", len(w.calls))
	}
	return nil
}

func (*fakeWatcher) ErrorAs(err error, i interface{}) bool { return false }

func (*fakeWatcher) IsNotExist(err error) bool { return false }

func (*fakeWatcher) ErrorCode(error) gcerrors.ErrorCode { return gcerrors.Internal }

// watchResp encapsulates the expected result of a Watch call.
type watchResp struct {
	snap Snapshot
	err  bool
}

func TestVariable(t *testing.T) {

	const (
		v1 = "foo"
		v2 = "bar"
	)
	var (
		upd1         = time.Now()
		upd2         = time.Now().Add(1 * time.Minute)
		fail1, fail2 = errors.New("fail1"), errors.New("fail2")
		snap1        = Snapshot{Value: v1, UpdateTime: upd1}
		snap2        = Snapshot{Value: v2, UpdateTime: upd2}
	)

	tests := []struct {
		name  string
		calls []*state
		// Watch will be called once for each entry in want.
		want []*watchResp
	}{
		{
			name: "Repeated errors don't return until it changes to a different error",
			calls: []*state{
				&state{err: fail1},
				&state{err: fail1},
				&state{err: fail1},
				&state{err: fail1},
				&state{err: fail2},
			},
			want: []*watchResp{
				&watchResp{err: true},
				&watchResp{err: true},
			},
		},
		{
			name: "Repeated errors don't return until it changes to a value",
			calls: []*state{
				&state{err: fail1},
				&state{err: fail1},
				&state{err: fail1},
				&state{err: fail1},
				&state{val: v1, updateTime: upd1},
			},
			want: []*watchResp{
				&watchResp{err: true},
				&watchResp{snap: snap1},
			},
		},
		{
			name: "Repeated values don't return until it changes to an error",
			calls: []*state{
				&state{val: v1, updateTime: upd1},
				&state{val: v1, updateTime: upd1},
				&state{val: v1, updateTime: upd1},
				&state{val: v1, updateTime: upd1},
				&state{err: fail1},
			},
			want: []*watchResp{
				&watchResp{snap: snap1},
				&watchResp{err: true},
			},
		},
		{
			name: "Repeated values don't return until it changes to a different version",
			calls: []*state{
				&state{val: v1, updateTime: upd1},
				&state{val: v1, updateTime: upd1},
				&state{val: v1, updateTime: upd1},
				&state{val: v1, updateTime: upd1},
				&state{val: v2, updateTime: upd2},
			},
			want: []*watchResp{
				&watchResp{snap: snap1},
				&watchResp{snap: snap2},
			},
		},
	}

	ctx := context.Background()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := &fakeWatcher{t: t, calls: tc.calls}
			v := New(w)
			defer func() {
				if err := v.Close(); err != nil {
					t.Error(err)
				}
			}()
			for i, want := range tc.want {
				snap, err := v.Watch(ctx)
				if (err != nil) != want.err {
					t.Errorf("Watch #%d: got error %v wanted error %v", i+1, err, want.err)
				}
				if snap.Value != want.snap.Value {
					t.Fatalf("Watch #%d: got snapshot.Value %v want %v", i+1, snap.Value, want.snap.Value)
				}
				if snap.UpdateTime != want.snap.UpdateTime {
					t.Errorf("Watch #%d: got snapshot.UpdateTime %v want %v", i+1, snap.UpdateTime, want.snap.UpdateTime)
				}
			}
		})
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
