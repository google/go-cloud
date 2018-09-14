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

// Package runtimevar_test contains tests that exercises the runtimevar APIs. It does not test
// driver implementations.
package runtimevar_test

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/driver"
	"github.com/google/go-cmp/cmp"
)

// watchVariableCall encapsulates a fake response to a driver.Watcher.WatchVariable call.
type watchVariableCall struct {
	// version and err will be compared to prevVersion and prevErr
	// respectively to determine whether to return all nil values.
	v       *driver.Variable
	version interface{}
	err     error
}

// fakeWatcher is a fake implementation of driver.Watcher.
type fakeWatcher struct {
	t     *testing.T
	calls []*watchVariableCall
}

func (w *fakeWatcher) Close() error {
	if len(w.calls) != 0 {
		return fmt.Errorf("expected %d more calls to WatchVariable", len(w.calls))
	}
	return nil
}

func (w *fakeWatcher) WatchVariable(ctx context.Context, prevVersion interface{}, prevErr error) (*driver.Variable, interface{}, time.Duration, error) {
	w.t.Logf("fakewatcher.WatchVariable prevVersion %v prevErr %v", prevVersion, prevErr)
	if len(w.calls) == 0 {
		w.t.Fatal("  --> unexpected call!")
	}
	c := w.calls[0]
	w.calls = w.calls[1:]
	if prevErr != nil && c.err != nil && c.err.Error() == prevErr.Error() {
		w.t.Log("  --> same error")
		return nil, nil, 0, nil
	}
	if prevVersion != nil && prevVersion == c.version {
		w.t.Log("  --> same version")
		return nil, nil, 0, nil
	}
	w.t.Logf("  --> returning val %v, version %v, err %v", c.v, c.version, c.err)
	return c.v, c.version, 0, c.err
}

// watchResp encapsulates the expected result of a Watch call.
type watchResp struct {
	snap runtimevar.Snapshot
	err  bool
}

func TestVariable(t *testing.T) {

	fail1, fail2 := errors.New("fail1"), errors.New("fail2")
	var1 := &driver.Variable{Value: 42, UpdateTime: time.Now()}
	var2 := &driver.Variable{Value: 43, UpdateTime: time.Now().Add(1 * time.Minute)}
	snap1 := runtimevar.Snapshot{Value: var1.Value, UpdateTime: var1.UpdateTime}
	snap2 := runtimevar.Snapshot{Value: var2.Value, UpdateTime: var2.UpdateTime}

	tests := []struct {
		name    string
		calls   []*watchVariableCall
		// Watch will be called once for each entry in want.
		want []*watchResp
	}{
		{
			name: "Repeated errors don't return until it changes to a different error",
			calls: []*watchVariableCall{
				&watchVariableCall{err: fail1},
				&watchVariableCall{err: fail1},
				&watchVariableCall{err: fail1},
				&watchVariableCall{err: fail1},
				&watchVariableCall{err: fail2},
			},
			want: []*watchResp{
				&watchResp{err: true},
				&watchResp{err: true},
			},
		},
		{
			name: "Repeated errors don't return until it changes to a value",
			calls: []*watchVariableCall{
				&watchVariableCall{err: fail1},
				&watchVariableCall{err: fail1},
				&watchVariableCall{err: fail1},
				&watchVariableCall{err: fail1},
				&watchVariableCall{v: var1},
			},
			want: []*watchResp{
				&watchResp{err: true},
				&watchResp{snap: snap1},
			},
		},
		{
			name: "Repeated values don't return until it changes to an error",
			calls: []*watchVariableCall{
				&watchVariableCall{v: var1, version: 1},
				&watchVariableCall{v: var1, version: 1},
				&watchVariableCall{v: var1, version: 1},
				&watchVariableCall{v: var1, version: 1},
				&watchVariableCall{err: fail1},
			},
			want: []*watchResp{
				&watchResp{snap: snap1},
				&watchResp{err: true},
			},
		},
		{
			name: "Repeated values don't return until it changes to a different version",
			calls: []*watchVariableCall{
				&watchVariableCall{v: var1, version: 1},
				&watchVariableCall{v: var1, version: 1},
				&watchVariableCall{v: var1, version: 1},
				&watchVariableCall{v: var1, version: 1},
				&watchVariableCall{v: var2, version: 2},
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
			v := runtimevar.New(w)
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
		decodeFn runtimevar.Decode
	}{
		{
			desc:     "JSON",
			encodeFn: json.Marshal,
			decodeFn: runtimevar.JSONDecode,
		},
		{
			desc:     "Gob",
			encodeFn: gobMarshal,
			decodeFn: runtimevar.GobDecode,
		},
	} {
		for i, input := range inputs {
			t.Run(fmt.Sprintf("%s_%d", tc.desc, i), func(t *testing.T) {
				decoder := runtimevar.NewDecoder(input, tc.decodeFn)
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
	got, err := runtimevar.StringDecoder.Decode([]byte(input))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if input != got.(string) {
		t.Errorf("output got %v, want %q", got, input)
	}
}

func TestBytesDecoder(t *testing.T) {
	input := []byte("hello world")
	got, err := runtimevar.BytesDecoder.Decode(input)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if diff := cmp.Diff(got, input); diff != "" {
		t.Errorf("output got %v, want %q", got, input)
	}
}
