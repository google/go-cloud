// Copyright 2018 Google LLC
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
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/driver"
	"github.com/google/go-cmp/cmp"
)

// fakeWatcher is a fake implementation of driver.Watcher meant only for exercising the
// generic APIs via tests.
type fakeWatcher struct {
	dc driver.Variable
}

func (fw *fakeWatcher) Close() error {
	return nil
}

func (fw *fakeWatcher) WatchVariable(context.Context) (driver.Variable, error) {
	return fw.dc, nil
}

// Ensure that fakeWatcher implements driver.Watcher
var _ driver.Watcher = &fakeWatcher{}

func TestVariable(t *testing.T) {
	ctx := context.Background()

	dc1 := driver.Variable{
		Value:      42,
		UpdateTime: time.Now(),
	}
	fw := &fakeWatcher{dc1}
	cfg := runtimevar.New(fw)

	// Watch should return the initial variable.
	snap1, err := cfg.Watch(ctx)
	if err != nil {
		t.Fatalf("Variable.Watch returns error %v", err)
	}
	if got := snap1.Value.(int); got != dc1.Value {
		t.Errorf("Snapshot.Value got %v, want %v", got, dc1.Value)
	}
	if got := snap1.UpdateTime; got != dc1.UpdateTime {
		t.Errorf("Snapshot.UpdateTime got %v, want %v", got, dc1.UpdateTime)
	}

	// Update the watcher's return value for Watch.
	dc2 := driver.Variable{
		Value:      8080,
		UpdateTime: time.Now(),
	}
	fw.dc = dc2

	// Retrieve next value.
	snap2, err := cfg.Watch(ctx)
	if err != nil {
		t.Fatalf("Variable.Watch returns error %v", err)
	}
	if got := snap2.Value.(int); got != dc2.Value {
		t.Errorf("Snapshot.Value got %v, want %v", got, dc2.Value)
	}
	if got := snap2.UpdateTime; got != dc2.UpdateTime {
		t.Errorf("Snapshot.UpdateTime got %v, want %v", got, dc2.UpdateTime)
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
