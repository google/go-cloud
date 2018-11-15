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
// driver.
package drivertest

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"github.com/google/go-cloud/internal/pubsub"
)

// Harness descibes the functionality test harnesses must provide to run
// conformance tests.
type Harness interface {
	// MakeTopicDriver creates a Topic and associated Subscription to test.
	MakePair() (*pubsub.Topic, *pubsub.Subscription, error)

	// Close closes resources used by the harness.
	Close()
}

// HarnessMaker describes functions that construct a harness for running tests.
// It is called exactly once per test; Harness.Close() will be called when the test is complete.
type HarnessMaker func(ctx context.Context, t *testing.T) (Harness, error)

// RunConformanceTests runs conformance tests for provider implementations of pubsub.
func RunConformanceTests(t *testing.T, newHarness HarnessMaker) {
	t.Run("TestSendReceive", func(t *testing.T) {
		testSendReceive(t, newHarness)
	})
	t.Run("TestErrors1", func(t *testing.T) {
		testErrors1(t, newHarness)
	})
	t.Run("TestErrors2", func(t *testing.T) {
		testErrors2(t, newHarness)
	})
	t.Run("TestCancelSendReceive", func(t *testing.T) {
		testCancelSendReceive(t, newHarness)
	})
	t.Run("TestCancelAck", func(t *testing.T) {
		testCancelAck(t, newHarness)
	})
}

// testSendReceive tests that a single message sent to a Topic gets received
// from a corresponding Subscription.
func testSendReceive(t *testing.T, newHarness HarnessMaker) {
	ctx := context.Background()
	h, err := newHarness(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	top, sub, err := h.MakePair()
	if err != nil {
		t.Fatal(err)
	}

	// Send to the topic.
	m := &pubsub.Message{
		Body:     []byte(randStr()),
		Metadata: map[string]string{randStr(): randStr()},
	}
	if err := top.Send(ctx, m); err != nil {
		t.Fatal(err)
	}

	// Receive from the subscription.
	m2, err := sub.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the received message matches the sent one.
	if string(m2.Body) != string(m.Body) {
		t.Errorf("received message body = %q, want %q", m2.Body, m.Body)
	}
	if len(m2.Metadata) != len(m.Metadata) {
		t.Errorf("got %d metadata keys, want %d", len(m2.Metadata), len(m.Metadata))
	}
	for k, v := range m.Metadata {
		if m2.Metadata[k] != v {
			t.Errorf("got %q for %q, want %q", m2.Metadata[k], k, v)
		}
	}
}

func testErrors1(t *testing.T, newHarness HarnessMaker) {
	ctx := context.Background()
	h, err := newHarness(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	top, _, err := h.MakePair()
	if err != nil {
		t.Fatal(err)
	}
	top.Close()
	m := &pubsub.Message{}
	if err := top.Send(ctx, m); err == nil {
		t.Error("top.Send returned nil, want error")
	}
}

func testErrors2(t *testing.T, newHarness HarnessMaker) {
	ctx := context.Background()
	h, err := newHarness(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	_, sub, err := h.MakePair()
	if err != nil {
		t.Fatal(err)
	}
	sub.Close()
	if _, err = sub.Receive(ctx); err == nil {
		t.Error("sub.Receive returned nil, want error")
	}
}

func testCancelSendReceive(t *testing.T, newHarness HarnessMaker) {
	ctx, cancel := context.WithCancel(context.Background())
	h, err := newHarness(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	top, sub, err := h.MakePair()
	if err != nil {
		t.Fatal(err)
	}

	cancel()

	m := &pubsub.Message{}
	if err := top.Send(ctx, m); err != context.Canceled {
		t.Errorf("top.Send returned %v, want context.Canceled", err)
	}
	if _, err := sub.Receive(ctx); err != context.Canceled {
		t.Errorf("sub.Receive returned %v, want context.Canceled", err)
	}
}

func testCancelAck(t *testing.T, newHarness HarnessMaker) {
	ctx, cancel := context.WithCancel(context.Background())
	h, err := newHarness(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	top, sub, err := h.MakePair()
	if err != nil {
		t.Fatal(err)
	}

	m := &pubsub.Message{}
	if err := top.Send(ctx, m); err != nil {
		t.Fatal(err)
	}
	mr, err := sub.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}

	cancel()

	if err := mr.Ack(ctx); err != context.Canceled {
		t.Errorf("mr.Ack returned %v, want context.Canceled", err)
	}
}

func randStr() string {
	return fmt.Sprintf("%d", rand.Int())
}
