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
	"github.com/google/go-cloud/internal/pubsub/driver"
)

// Harness descibes the functionality test harnesses must provide to run
// conformance tests.
type Harness interface {
	// MakeTopicDriver creates a driver.Topic to test.
	MakeTopicDriver(ctx context.Context) (driver.Topic, error)

	// MakeSubscriptionDriver creates a driver.Subscription to test. This
	// Subscription should be connected behind the scenes to the Topic
	// returned by MakeTopicDriver.
	MakeSubscriptionDriver(ctx context.Context) (driver.Subscription, error)

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
	t.Run("TestErrors", func(t *testing.T) {
		testErrors(t, newHarness)
	})
	t.Run("TestCanceled", func(t *testing.T) {
		testCanceled(t, newHarness)
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

	top := openTopic(ctx, t, h)
	sub := openSubscription(ctx, t, h)

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

func testErrors(t *testing.T, newHarness HarnessMaker) {
	ctx := context.Background()
	h, err := newHarness(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	wantErr := func(err error) {
		t.Helper()
		if err == nil {
			t.Error("got nil, want error")
		}
	}

	top := openTopic(ctx, t, h)
	top.Close()
	wantErr(top.Send(ctx, nil)) // topic closed

	top = openTopic(ctx, t, h)
	sub := openSubscription(ctx, t, h)
	sub.Close()
	_, err = sub.Receive(ctx)
	wantErr(err) // sub closed
}

func testCanceled(t *testing.T, newHarness HarnessMaker) {
	ctx, cancel := context.WithCancel(context.Background())
	h, err := newHarness(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	cancel()
	wantCanceled := func(err error) {
		t.Helper()
		if err != context.Canceled {
			t.Errorf("got %v, want context.Canceled", err)
		}
	}
	top := openTopic(ctx, t, h)

	wantCanceled(top.Send(ctx, nil))
	sub := openSubscription(ctx, t, h)
	m, err := sub.Receive(ctx)
	wantCanceled(err)
	wantCanceled(m.Ack(ctx))
}

func openTopic(ctx context.Context, t *testing.T, h Harness) *pubsub.Topic {
	t.Helper()
	td, err := h.MakeTopicDriver(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return pubsub.NewTopic(ctx, td)
}

func openSubscription(ctx context.Context, t *testing.T, h Harness) *pubsub.Subscription {
	t.Helper()
	ts, err := h.MakeSubscriptionDriver(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return pubsub.NewSubscription(ctx, ts)
}

func randStr() string {
	return fmt.Sprintf("%d", rand.Int())
}
