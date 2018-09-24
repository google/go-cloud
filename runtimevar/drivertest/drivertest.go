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

// Package drivertest provides a conformance test for implementations of
// runtimevar.
package drivertest

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cmp/cmp"
)

const (
	// shortWait is a very short delay.
	shortWait = 1 * time.Microsecond
	// defaultWait is a zero time.Duration that causes drivers to use
	// their default wait.
	defaultWait = time.Duration(0)
)

// Harness descibes the functionality test harnesses must provide to run conformance tests.
type Harness interface {
	// MakeVar creates a *runtimevar.Variable to watch the given variable.
	// If possible, wait should be used to configure what the driver returns
	// as the wait time.
	MakeVar(ctx context.Context, name string, decoder *runtimevar.Decoder, wait time.Duration) (*runtimevar.Variable, error)
	// CreateVariable creates the variable with the given contents in the provider.
	CreateVariable(ctx context.Context, name string, val []byte) error
	// UpdateVariable updates an existing variable to have the given contents in the provider.
	UpdateVariable(ctx context.Context, name string, val []byte) error
	// DeleteVariable deletes an existing variable in the provider.
	DeleteVariable(ctx context.Context, name string) error
	// Close is called when the test is complete.
	Close()
	// Mutable returns true iff the driver supports UpdateVariable/DeleteVariable.
	// If false, those functions should return errors, and the conformance tests
	// will skip and/or ignore errors for tests that require them.
	Mutable() bool
}

// HarnessMaker describes functions that construct a harness for running tests.
// It is called exactly once per test; Harness.Close() will be called when the test is complete.
type HarnessMaker func(t *testing.T) (Harness, error)

// RunConformanceTests runs conformance tests for provider implementations
// of runtimevar.
func RunConformanceTests(t *testing.T, newHarness HarnessMaker) {
	t.Run("TestNonExistentVariable", func(t *testing.T) {
		testNonExistentVariable(t, newHarness)
	})
	t.Run("TestString", func(t *testing.T) {
		testString(t, newHarness)
	})
	t.Run("TestJSON", func(t *testing.T) {
		testJSON(t, newHarness)
	})
	t.Run("TestInvalidJSON", func(t *testing.T) {
		testInvalidJSON(t, newHarness)
	})
	t.Run("TestUpdate", func(t *testing.T) {
		testUpdate(t, newHarness)
	})
	t.Run("TestDelete", func(t *testing.T) {
		testDelete(t, newHarness)
	})
}

func testNonExistentVariable(t *testing.T, newHarness HarnessMaker) {
	h, err := newHarness(t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	ctx := context.Background()

	v, err := h.MakeVar(ctx, "does-not-exist", runtimevar.StringDecoder, shortWait)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := v.Close(); err != nil {
			t.Error(err)
		}
	}()
	got, err := v.Watch(ctx)
	if err == nil {
		t.Errorf("got %v expected not-found error", got.Value)
	}
}

func testString(t *testing.T, newHarness HarnessMaker) {
	const (
		name    = "test-config-variable"
		content = "hello world"
	)

	h, err := newHarness(t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	ctx := context.Background()

	if err := h.CreateVariable(ctx, name, []byte(content)); err != nil {
		t.Fatal(err)
	}
	if h.Mutable() {
		defer func() {
			if err := h.DeleteVariable(ctx, name); err != nil {
				t.Fatal(err)
			}
		}()
	}

	// Use defaultWait here because we're going to test Watch not returning
	// below. Using shortWait here results in polling implementations spinning
	// and replay-based tests being flaky based on how many polls happen
	// before the ctx times out.
	v, err := h.MakeVar(ctx, name, runtimevar.StringDecoder, defaultWait)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := v.Close(); err != nil {
			t.Error(err)
		}
	}()
	got, err := v.Watch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// The variable is decoded to a string and matches the expected content.
	if gotS, ok := got.Value.(string); !ok {
		t.Fatalf("got value of type %v expected string", reflect.TypeOf(got.Value))
	} else if gotS != content {
		t.Errorf("got %q want %q", got.Value, content)
	}

	// A second watch should block forever since the value hasn't changed.
	// A short wait here doesn't guarantee that this is working, but will catch
	// most problems.
	tCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	got, err = v.Watch(tCtx)
	if err == nil {
		t.Errorf("got %v want error", got)
	}
	if tCtx.Err() == nil {
		t.Errorf("got err %v; want Watch to have blocked until context was Done", err)
	}
}

// Message is used as a target for JSON decoding.
type Message struct {
	Name, Text string
}

func testJSON(t *testing.T, newHarness HarnessMaker) {
	const (
		name        = "test-config-variable"
		jsonContent = `[
{"Name": "Ed", "Text": "Knock knock."},
{"Name": "Sam", "Text": "Who's there?"}
]`
	)
	want := []*Message{{Name: "Ed", Text: "Knock knock."}, {Name: "Sam", Text: "Who's there?"}}

	h, err := newHarness(t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	ctx := context.Background()

	if err := h.CreateVariable(ctx, name, []byte(jsonContent)); err != nil {
		t.Fatal(err)
	}
	if h.Mutable() {
		defer func() {
			if err := h.DeleteVariable(ctx, name); err != nil {
				t.Fatal(err)
			}
		}()
	}

	var jsonData []*Message
	v, err := h.MakeVar(ctx, name, runtimevar.NewDecoder(jsonData, runtimevar.JSONDecode), shortWait)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := v.Close(); err != nil {
			t.Error(err)
		}
	}()
	got, err := v.Watch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// The variable is decoded to a []*Message and matches the expected content.
	if gotSlice, ok := got.Value.([]*Message); !ok {
		t.Fatalf("got value of type %v expected []*Message", reflect.TypeOf(got.Value))
	} else if !cmp.Equal(gotSlice, want) {
		t.Errorf("got %v want %v", gotSlice, want)
	}
}

func testInvalidJSON(t *testing.T, newHarness HarnessMaker) {
	const (
		name    = "test-config-variable"
		content = "not-json"
	)

	h, err := newHarness(t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	ctx := context.Background()

	if err := h.CreateVariable(ctx, name, []byte(content)); err != nil {
		t.Fatal(err)
	}
	if h.Mutable() {
		defer func() {
			if err := h.DeleteVariable(ctx, name); err != nil {
				t.Fatal(err)
			}
		}()
	}

	var jsonData []*Message
	v, err := h.MakeVar(ctx, name, runtimevar.NewDecoder(jsonData, runtimevar.JSONDecode), shortWait)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := v.Close(); err != nil {
			t.Error(err)
		}
	}()
	got, err := v.Watch(ctx)
	if err == nil {
		t.Errorf("got %v wanted invalid-json error", got.Value)
	}
}

func testUpdate(t *testing.T, newHarness HarnessMaker) {
	const (
		name     = "test-config-variable"
		content1 = "hello world"
		content2 = "goodbye world"
	)

	h, err := newHarness(t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if !h.Mutable() {
		return
	}
	ctx := context.Background()

	// Create the variable and verify Watch sees the value.
	if err := h.CreateVariable(ctx, name, []byte(content1)); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = h.DeleteVariable(ctx, name) }()

	v, err := h.MakeVar(ctx, name, runtimevar.StringDecoder, shortWait)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := v.Close(); err != nil {
			t.Error(err)
		}
	}()
	got, err := v.Watch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Value.(string) != content1 {
		t.Errorf("got %q want %q", got.Value, content1)
	}

	// Update the variable and verify Watch sees the updated value.
	if err := h.UpdateVariable(ctx, name, []byte(content2)); err != nil {
		t.Fatal(err)
	}
	got, err = v.Watch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Value.(string) != content2 {
		t.Errorf("got %q want %q", got.Value, content2)
	}
}

func testDelete(t *testing.T, newHarness HarnessMaker) {
	const (
		name     = "test-config-variable"
		content1 = "hello world"
		content2 = "goodbye world"
	)

	h, err := newHarness(t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if !h.Mutable() {
		return
	}
	ctx := context.Background()

	// Create the variable and verify Watch sees the value.
	if err := h.CreateVariable(ctx, name, []byte(content1)); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = h.DeleteVariable(ctx, name) }()

	v, err := h.MakeVar(ctx, name, runtimevar.StringDecoder, shortWait)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := v.Close(); err != nil {
			t.Error(err)
		}
	}()
	got, err := v.Watch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Value.(string) != content1 {
		t.Errorf("got %q want %q", got.Value, content1)
	}
	prev := got

	// Delete the variable.
	if err := h.DeleteVariable(ctx, name); err != nil {
		t.Fatal(err)
	}

	// Watch should return an error now.
	if got, err = v.Watch(ctx); err == nil {
		t.Fatalf("got %v, want error because variable is deleted", got.Value)
	}

	// Reset the variable with new content and verify via Watch.
	if err := h.CreateVariable(ctx, name, []byte(content2)); err != nil {
		t.Fatal(err)
	}
	got, err = v.Watch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Value.(string) != content2 {
		t.Errorf("got %q want %q", got.Value, content2)
	}
	if got.UpdateTime.Before(prev.UpdateTime) {
		t.Errorf("got UpdateTime %v < previous %v, want >=", got.UpdateTime, prev.UpdateTime)
	}
}
