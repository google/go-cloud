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

// Package drivertest provides a conformance test for implementations of
// runtimevar.
package drivertest // import "gocloud.dev/runtimevar/drivertest"

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/testing/setup"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
)

// Harness descibes the functionality test harnesses must provide to run conformance tests.
type Harness interface {
	// MakeWatcher creates a driver.Watcher to watch the given variable.
	MakeWatcher(ctx context.Context, name string, decoder *runtimevar.Decoder) (driver.Watcher, error)
	// CreateVariable creates the variable with the given contents.
	CreateVariable(ctx context.Context, name string, val []byte) error
	// UpdateVariable updates an existing variable to have the given contents.
	UpdateVariable(ctx context.Context, name string, val []byte) error
	// DeleteVariable deletes an existing variable.
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

// AsTest represents a test of As functionality.
// The conformance test:
// 1. Reads a Snapshot of the variable before it exists.
// 2. Calls ErrorCheck.
// 3. Creates the variable and reads a Snapshot of it.
// 4. Calls SnapshotCheck.
type AsTest interface {
	// Name should return a descriptive name for the test.
	Name() string
	// SnapshotCheck will be called to allow verification of Snapshot.As.
	SnapshotCheck(s *runtimevar.Snapshot) error
	// ErrorCheck will be called to allow verification of Variable.ErrorAs.
	// driver is provided so that errors other than err can be checked;
	// Variable.ErrorAs won't work since it expects driver errors to be wrapped.
	ErrorCheck(v *runtimevar.Variable, err error) error
}

type verifyAsFailsOnNil struct{}

func (verifyAsFailsOnNil) Name() string {
	return "verify As returns false when passed nil"
}

func (verifyAsFailsOnNil) SnapshotCheck(v *runtimevar.Snapshot) error {
	if v.As(nil) {
		return errors.New("want Snapshot.As to return false when passed nil")
	}
	return nil
}

func (verifyAsFailsOnNil) ErrorCheck(v *runtimevar.Variable, err error) (ret error) {
	defer func() {
		if recover() == nil {
			ret = errors.New("want ErrorAs to panic when passed nil")
		}
	}()
	v.ErrorAs(err, nil)
	return nil
}

// RunConformanceTests runs conformance tests for driver implementations
// of runtimevar.
func RunConformanceTests(t *testing.T, newHarness HarnessMaker, asTests []AsTest) {
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
	t.Run("TestUpdateWithErrors", func(t *testing.T) {
		testUpdateWithErrors(t, newHarness)
	})
	asTests = append(asTests, verifyAsFailsOnNil{})
	t.Run("TestAs", func(t *testing.T) {
		for _, st := range asTests {
			if st.Name() == "" {
				t.Fatalf("AsTest.Name is required")
			}
			t.Run(st.Name(), func(t *testing.T) {
				testAs(t, newHarness, st)
			})
		}
	})
}

// deadlineExceeded returns true if err represents a context exceeded error.
// It can either be a true context.DeadlineExceeded, or an RPC aborted due to
// ctx cancellation; we don't have a good way of checking for the latter
// explicitly so we check the Error() string.
func deadlineExceeded(err error) bool {
	return err == context.DeadlineExceeded || strings.Contains(err.Error(), "context deadline exceeded")
}

// waitTimeForBlockingCheck returns a duration to wait when verifying that a
// call blocks. When in replay mode, it can be quite short to make tests run
// quickly. When in record mode, it has to be long enough that RPCs can
// consistently finish.
func waitTimeForBlockingCheck() time.Duration {
	if *setup.Record {
		return 5 * time.Second
	}
	return 10 * time.Millisecond
}

func testNonExistentVariable(t *testing.T, newHarness HarnessMaker) {
	h, err := newHarness(t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	ctx := context.Background()

	drv, err := h.MakeWatcher(ctx, "does-not-exist", runtimevar.StringDecoder)
	if err != nil {
		t.Fatal(err)
	}
	v := runtimevar.New(drv)
	defer func() {
		if err := v.Close(); err != nil {
			t.Error(err)
		}
	}()
	got, err := v.Watch(ctx)
	if err == nil {
		t.Errorf("got %v expected not-found error", got.Value)
	} else if gcerrors.Code(err) != gcerrors.NotFound {
		t.Error("got IsNotExist false, expected true")
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

	drv, err := h.MakeWatcher(ctx, name, runtimevar.StringDecoder)
	if err != nil {
		t.Fatal(err)
	}
	v := runtimevar.New(drv)
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
		t.Fatalf("got value of type %T expected string", got.Value)
	} else if gotS != content {
		t.Errorf("got %q want %q", got.Value, content)
	}

	// A second watch should block forever since the value hasn't changed.
	// A short wait here doesn't guarantee that this is working, but will catch
	// most problems.
	tCtx, cancel := context.WithTimeout(ctx, waitTimeForBlockingCheck())
	defer cancel()
	got, err = v.Watch(tCtx)
	if err == nil {
		t.Errorf("got %v want error", got)
	}
	// tCtx should be cancelled. However, tests using record/replay mode can
	// be in the middle of an RPC when that happens, and save the resulting
	// RPC error during record. During replay, that error can be returned
	// immediately (before tCtx is cancelled). So, we accept deadline exceeded
	// errors as well.
	if tCtx.Err() == nil && !deadlineExceeded(err) {
		t.Errorf("got err %v; want Watch to have blocked until context was Done, or for the error to be deadline exceeded", err)
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
	drv, err := h.MakeWatcher(ctx, name, runtimevar.NewDecoder(jsonData, runtimevar.JSONDecode))
	if err != nil {
		t.Fatal(err)
	}
	v := runtimevar.New(drv)
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
		t.Fatalf("got value of type %T expected []*Message", got.Value)
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
	drv, err := h.MakeWatcher(ctx, name, runtimevar.NewDecoder(jsonData, runtimevar.JSONDecode))
	if err != nil {
		t.Fatal(err)
	}
	v := runtimevar.New(drv)
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

	// Create the variable and verify WatchVariable sees the value.
	if err := h.CreateVariable(ctx, name, []byte(content1)); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = h.DeleteVariable(ctx, name) }()

	drv, err := h.MakeWatcher(ctx, name, runtimevar.StringDecoder)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := drv.Close(); err != nil {
			t.Error(err)
		}
	}()
	state, _ := drv.WatchVariable(ctx, nil)
	if state == nil {
		t.Fatalf("got nil state, want a non-nil state with a value")
	}
	got, err := state.Value()
	if err != nil {
		t.Fatal(err)
	}
	if gotS, ok := got.(string); !ok {
		t.Fatalf("got value of type %T expected string", got)
	} else if gotS != content1 {
		t.Errorf("got %q want %q", got, content1)
	}

	// The variable hasn't changed, so drv.WatchVariable should either
	// return nil or block.
	cancelCtx, cancel := context.WithTimeout(ctx, waitTimeForBlockingCheck())
	defer cancel()
	unchangedState, _ := drv.WatchVariable(cancelCtx, state)
	if unchangedState == nil {
		// OK
	} else {
		got, err = unchangedState.Value()
		if err != context.DeadlineExceeded {
			t.Fatalf("got state %v/%v, wanted nil or nil/DeadlineExceeded after no change", got, err)
		}
	}

	// Update the variable and verify WatchVariable sees the updated value.
	if err := h.UpdateVariable(ctx, name, []byte(content2)); err != nil {
		t.Fatal(err)
	}
	state, _ = drv.WatchVariable(ctx, state)
	if state == nil {
		t.Fatalf("got nil state, want a non-nil state with a value")
	}
	got, err = state.Value()
	if err != nil {
		t.Fatal(err)
	}
	if gotS, ok := got.(string); !ok {
		t.Fatalf("got value of type %T expected string", got)
	} else if gotS != content2 {
		t.Errorf("got %q want %q", got, content2)
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

	// Create the variable and verify WatchVariable sees the value.
	if err := h.CreateVariable(ctx, name, []byte(content1)); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = h.DeleteVariable(ctx, name) }()

	drv, err := h.MakeWatcher(ctx, name, runtimevar.StringDecoder)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := drv.Close(); err != nil {
			t.Error(err)
		}
	}()
	state, _ := drv.WatchVariable(ctx, nil)
	if state == nil {
		t.Fatalf("got nil state, want a non-nil state with a value")
	}
	got, err := state.Value()
	if err != nil {
		t.Fatal(err)
	}
	if gotS, ok := got.(string); !ok {
		t.Fatalf("got value of type %T expected string", got)
	} else if gotS != content1 {
		t.Errorf("got %q want %q", got, content1)
	}
	prev := state

	// Delete the variable.
	if err := h.DeleteVariable(ctx, name); err != nil {
		t.Fatal(err)
	}

	// WatchVariable should return a state with an error now.
	state, _ = drv.WatchVariable(ctx, state)
	if state == nil {
		t.Fatalf("got nil state, want a non-nil state with an error")
	}
	got, err = state.Value()
	if err == nil {
		t.Fatalf("got %v want error because variable is deleted", got)
	}

	// Reset the variable with new content and verify via WatchVariable.
	if err := h.CreateVariable(ctx, name, []byte(content2)); err != nil {
		t.Fatal(err)
	}
	state, _ = drv.WatchVariable(ctx, state)
	if state == nil {
		t.Fatalf("got nil state, want a non-nil state with a value")
	}
	got, err = state.Value()
	if err != nil {
		t.Fatal(err)
	}
	if gotS, ok := got.(string); !ok {
		t.Fatalf("got value of type %T expected string", got)
	} else if gotS != content2 {
		t.Errorf("got %q want %q", got, content2)
	}
	if state.UpdateTime().Before(prev.UpdateTime()) {
		t.Errorf("got UpdateTime %v < previous %v, want >=", state.UpdateTime(), prev.UpdateTime())
	}
}

func testUpdateWithErrors(t *testing.T, newHarness HarnessMaker) {
	const (
		name     = "test-updating-variable-to-error"
		content1 = `[{"Name": "Foo", "Text": "Bar"}]`
		content2 = "invalid-json"
		content3 = "invalid-json2"
	)
	want := []*Message{{Name: "Foo", Text: "Bar"}}

	h, err := newHarness(t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if !h.Mutable() {
		return
	}
	ctx := context.Background()

	// Create the variable and verify WatchVariable sees the value.
	if err := h.CreateVariable(ctx, name, []byte(content1)); err != nil {
		t.Fatal(err)
	}

	var jsonData []*Message
	drv, err := h.MakeWatcher(ctx, name, runtimevar.NewDecoder(jsonData, runtimevar.JSONDecode))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := drv.Close(); err != nil {
			t.Error(err)
		}
	}()
	state, _ := drv.WatchVariable(ctx, nil)
	if state == nil {
		t.Fatal("got nil state, want a non-nil state with a value")
	}
	got, err := state.Value()
	if err != nil {
		t.Fatal(err)
	}
	if gotSlice, ok := got.([]*Message); !ok {
		t.Fatalf("got value of type %T expected []*Message", got)
	} else if !cmp.Equal(gotSlice, want) {
		t.Errorf("got %v want %v", gotSlice, want)
	}

	// Update the variable to invalid JSON and verify WatchVariable returns an error.
	if err := h.UpdateVariable(ctx, name, []byte(content2)); err != nil {
		t.Fatal(err)
	}
	state, _ = drv.WatchVariable(ctx, state)
	if state == nil {
		t.Fatal("got nil state, want a non-nil state with an error")
	}
	_, err = state.Value()
	if err == nil {
		t.Fatal("got nil err want invalid JSON error")
	}

	// Update the variable again, with different invalid JSON.
	// WatchVariable should block or return nil since it's the same error as before.
	if err := h.UpdateVariable(ctx, name, []byte(content3)); err != nil {
		t.Fatal(err)
	}
	tCtx, cancel := context.WithTimeout(ctx, waitTimeForBlockingCheck())
	defer cancel()
	state, _ = drv.WatchVariable(tCtx, state)
	if state == nil {
		// OK: nil indicates no change.
	} else {
		// WatchVariable should have blocked until tCtx was cancelled, and we
		// should have gotten that error back.
		got, err := state.Value()
		if err == nil {
			t.Fatalf("got %v and nil error, want non-nil error", got)
		}
		// tCtx should be cancelled. However, tests using record/replay mode can
		// be in the middle of an RPC when that happens, and save the resulting
		// RPC error during record. During replay, that error can be returned
		// immediately (before tCtx is cancelled). So, we accept deadline exceeded
		// errors as well.
		if tCtx.Err() == nil && !deadlineExceeded(err) {
			t.Errorf("got err %v; want Watch to have blocked until context was Done, or for the error to be deadline exceeded", err)
		}
	}
}

// testAs tests the various As functions, using AsTest.
func testAs(t *testing.T, newHarness HarnessMaker, st AsTest) {
	const (
		name    = "variable-for-as"
		content = "hello world"
	)

	h, err := newHarness(t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	ctx := context.Background()

	// Try to read the variable before it exists.
	drv, err := h.MakeWatcher(ctx, name, runtimevar.StringDecoder)
	if err != nil {
		t.Fatal(err)
	}
	v := runtimevar.New(drv)
	s, gotErr := v.Watch(ctx)
	if gotErr == nil {
		t.Fatalf("got nil error and %v, expected non-nil error", v)
	}
	if err := st.ErrorCheck(v, gotErr); err != nil {
		t.Error(err)
	}
	var dummy string
	if s.As(&dummy) {
		t.Error(errors.New("want Snapshot.As to return false when Snapshot is zero value"))
	}
	if err := v.Close(); err != nil {
		t.Error(err)
	}

	// Create the variable and verify WatchVariable sees the value.
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

	drv, err = h.MakeWatcher(ctx, name, runtimevar.StringDecoder)
	if err != nil {
		t.Fatal(err)
	}
	v = runtimevar.New(drv)
	defer func() {
		if err := v.Close(); err != nil {
			t.Error(err)
		}
	}()
	s, err = v.Watch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SnapshotCheck(&s); err != nil {
		t.Error(err)
	}
}
