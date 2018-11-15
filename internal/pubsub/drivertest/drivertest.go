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
func RunConformanceTests(t *testing.T, newHarness HarnessMaker, asTests []AsTest) {
	t.Run("TestSendReceive", func(t *testing.T) {
		testSendReceive(t, newHarness)
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

	// Open the topic.
	td, err := h.MakeTopicDriver(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t := pubsub.NewTopic(ctx, td)

	// Open the subscription.
	ts, err := h.MakeSubscriptionDriver(ctx)
	if err != nil {
		t.Fatal(err)
	}
	s := pubsub.NewSubscription(ctx, ts)

	// Send to the topic.
	if err := t.Send(ctx, m); err != nil {
		t.Fatal(err)
	}

	// Receive from the subscription.
	m2, err := s.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the received message matches the sent one.
	if string(m2.Body) != string(m.Body) {
		t.Errorf("received message body = %q, want %q", m2.body, m.Body)
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
