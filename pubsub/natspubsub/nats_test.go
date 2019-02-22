// Copyright 2019 The Go Cloud Development Kit Authors
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

package natspubsub

import (
	"context"
	"fmt"
	"testing"

	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
	"gocloud.dev/pubsub/drivertest"

	"github.com/nats-io/gnatsd/server"
	gnatsd "github.com/nats-io/gnatsd/test"
	"github.com/nats-io/go-nats"
)

const (
	TEST_PORT  = 11222
	BENCH_PORT = 9222
)

type harness struct {
	s  *server.Server
	nc *nats.Conn
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	opts := gnatsd.DefaultTestOptions
	opts.Port = TEST_PORT
	s := gnatsd.RunServer(&opts)
	nc, err := nats.Connect(fmt.Sprintf("nats://127.0.0.1:%d", TEST_PORT))
	if err != nil {
		return nil, err
	}
	return &harness{s, nc}, nil
}

func (h *harness) CreateTopic(ctx context.Context, testName string) (dt driver.Topic, cleanup func(), err error) {
	cleanup = func() {}
	dt = openTopic(h.nc, testName)
	return dt, cleanup, nil
}

func (h *harness) MakeNonexistentTopic(ctx context.Context) (driver.Topic, error) {
	// A nil *topic behaves like a nonexistent topic.
	return (*topic)(nil), nil
}

func (h *harness) CreateSubscription(ctx context.Context, dt driver.Topic, testName string) (ds driver.Subscription, cleanup func(), err error) {
	ds = openSubscription(h.nc, testName)
	// FIXME(dlc) - Check for error?
	cleanup = func() {
		var sub *nats.Subscription
		if ds.As(&sub) {
			sub.Unsubscribe()
		}
	}
	return ds, cleanup, nil
}

func (h *harness) MakeNonexistentSubscription(ctx context.Context) (driver.Subscription, error) {
	return (*subscription)(nil), nil
}

func (h *harness) Close() {
	if h == nil {
		return
	}
	if h.nc != nil {
		h.nc.Close()
	}
	if h.s != nil {
		h.s.Shutdown()
	}
}

type natsAsTest struct{}

func (natsAsTest) Name() string {
	return "nats test"
}

func (natsAsTest) TopicCheck(top *pubsub.Topic) error {
	var c2 nats.Conn
	if top.As(&c2) {
		return fmt.Errorf("cast succeeded for %T, want failure", &c2)
	}
	var c3 *nats.Conn
	if !top.As(&c3) {
		return fmt.Errorf("cast failed for %T", &c3)
	}
	return nil
}

func (natsAsTest) SubscriptionCheck(sub *pubsub.Subscription) error {
	var c2 nats.Subscription
	if sub.As(&c2) {
		return fmt.Errorf("cast succeeded for %T, want failure", &c2)
	}
	var c3 *nats.Subscription
	if !sub.As(&c3) {
		return fmt.Errorf("cast failed for %T", &c3)
	}
	return nil
}

func (natsAsTest) TopicErrorCheck(t *pubsub.Topic, err error) error {
	return nil
}

func (natsAsTest) SubscriptionErrorCheck(s *pubsub.Subscription, err error) error {
	return nil
}

func (r natsAsTest) MessageCheck(m *pubsub.Message) error {
	var pm nats.Msg
	if m.As(&pm) {
		return fmt.Errorf("cast succeeded for %T, want failure", &pm)
	}
	var ppm *nats.Msg
	if !m.As(&ppm) {
		return fmt.Errorf("cast failed for %T", &ppm)
	}
	return nil
}

func TestConformance(t *testing.T) {
	asTests := []drivertest.AsTest{natsAsTest{}}
	drivertest.RunConformanceTests(t, newHarness, asTests, nil)
}

func BenchmarkNatsPubSub(b *testing.B) {
	ctx := context.Background()

	opts := gnatsd.DefaultTestOptions
	opts.Port = BENCH_PORT
	s := gnatsd.RunServer(&opts)
	defer s.Shutdown()

	nc, err := nats.Connect(fmt.Sprintf("nats://127.0.0.1:%d", BENCH_PORT))
	if err != nil {
		b.Fatal(err)
	}
	defer nc.Close()

	h := &harness{s, nc}
	dt, cleanup, err := h.CreateTopic(ctx, b.Name())
	if err != nil {
		b.Fatal(err)
	}
	defer cleanup()
	ds, cleanup, err := h.CreateSubscription(ctx, dt, b.Name())
	if err != nil {
		b.Fatal(err)
	}
	defer cleanup()
	drivertest.RunBenchmarks(b, pubsub.NewTopic(dt), pubsub.NewSubscription(ds, nil))
}
