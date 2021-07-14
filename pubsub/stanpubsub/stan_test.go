// Copyright 2021 The Go Cloud Development Kit Authors
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

package stanpubsub

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	nsserver "github.com/nats-io/nats-streaming-server/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/stan.go"
	"gocloud.dev/gcerrors"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/batcher"
	"gocloud.dev/pubsub/driver"
	"gocloud.dev/pubsub/drivertest"
)

const (
	testPort      = 11222
	testClusterID = "gc-test-cluster"
	testClientID  = "gc-test-client"
	benchPort     = 9222
)

var clientCtr int32

func nextClientID() string {
	atomic.AddInt32(&clientCtr, 1)
	c := atomic.LoadInt32(&clientCtr)
	return fmt.Sprintf("%s-%d", testClientID, c)
}

type harness struct {
	ns      *nsserver.StanServer
	sc      stan.Conn
	options *SubscriptionOptions
}

func newHarness(ctx context.Context, t *testing.T, options *SubscriptionOptions) (drivertest.Harness, error) {
	o := nsserver.GetDefaultOptions()
	o.ID = testClusterID
	ns, err := newStanServer(o)
	if err != nil {
		return nil, err
	}

	sc, err := stan.Connect(testClusterID, nextClientID(), stan.NatsURL(fmt.Sprintf("nats://127.0.0.1:%d", testPort)))
	if err != nil {
		return nil, err
	}
	return &harness{ns: ns, sc: sc, options: options}, nil
}

func newStanServer(o *nsserver.Options) (*nsserver.StanServer, error) {
	opts := nsserver.DefaultNatsServerOptions
	opts.Port = testPort

	ns, err := nsserver.RunServerWithOpts(o, &opts)
	if err != nil {
		return nil, err
	}
	return ns, nil
}

func (h *harness) CreateTopic(ctx context.Context, testName string) (driver.Topic, func(), error) {
	cleanup := func() {}

	testName = strings.Replace(testName, "/", ".", -1)
	dt, err := openTopic(h.sc, testName)
	if err != nil {
		return nil, nil, err
	}
	return dt, cleanup, nil
}

func (h *harness) MakeNonexistentTopic(ctx context.Context) (driver.Topic, error) {
	// A nil *topic behaves like a nonexistent topic.
	return (*topic)(nil), nil
}

func (h *harness) CreateSubscription(ctx context.Context, dt driver.Topic, testName string) (driver.Subscription, func(), error) {
	testName = strings.Replace(testName, "/", ".", -1)
	ds, err := openSubscription(h.sc, testName, h.options)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		var sub stan.Subscription
		if ds.As(&sub) {
			sub.Unsubscribe()
		}
	}
	return ds, cleanup, nil
}

func (h *harness) MakeNonexistentSubscription(ctx context.Context) (driver.Subscription, func(), error) {
	return (*subscription)(nil), func() {}, nil
}

func (h *harness) Close() {
	h.sc.Close()
	h.ns.Shutdown()
}

func (h *harness) MaxBatchSizes() (int, int) { return 10, 0 }

func (h *harness) SupportsMultipleSubscriptions() bool {
	if h.options != nil {
		if h.options.DurableName != "" || h.options.Queue != "" {
			return false
		}
	}
	return true
}

type stanAsTest struct{}

func (stanAsTest) Name() string {
	return "stan test"
}

func (stanAsTest) TopicCheck(topic *pubsub.Topic) error {
	var c2 stan.Conn
	if topic.As(c2) {
		return fmt.Errorf("cast succeeded for %T, want failure", c2)
	}
	var c3 stan.Conn
	if !topic.As(&c3) {
		return fmt.Errorf("cast failed for %T", &c3)
	}
	return nil
}

func (stanAsTest) SubscriptionCheck(sub *pubsub.Subscription) error {
	var c2 stan.Subscription
	if sub.As(c2) {
		return fmt.Errorf("cast succeeded for %T, want failure", c2)
	}
	var c3 stan.Subscription
	if !sub.As(&c3) {
		return fmt.Errorf("cast failed for %T", &c3)
	}
	return nil
}

func (stanAsTest) TopicErrorCheck(t *pubsub.Topic, err error) error {
	var dummy string
	if t.ErrorAs(err, &dummy) {
		return fmt.Errorf("cast succeeded for %T, want failure", &dummy)
	}
	return nil
}

func (stanAsTest) SubscriptionErrorCheck(s *pubsub.Subscription, err error) error {
	var dummy string
	if s.ErrorAs(err, &dummy) {
		return fmt.Errorf("cast succeeded for %T, want failure", &dummy)
	}
	return nil
}

func (stanAsTest) MessageCheck(m *pubsub.Message) error {
	var pm stan.Msg
	if m.As(&pm) {
		return fmt.Errorf("cast succeeded for %T, want failure", &pm)
	}
	var ppm *stan.Msg
	if !m.As(&ppm) {
		return fmt.Errorf("cast failed for %T", &ppm)
	}
	return nil
}

func (stanAsTest) BeforeSend(as func(interface{}) bool) error {
	return nil
}

func (stanAsTest) AfterSend(as func(interface{}) bool) error {
	return nil
}

func TestConformance(t *testing.T) {
	asTests := []drivertest.AsTest{stanAsTest{}}

	testCases := []struct {
		testName string
		queue    string
		options  *SubscriptionOptions
	}{
		{
			testName: "Standard",
			queue:    "",
			options:  nil,
		},
		{
			testName: "Queue",
			queue:    "exampleQueue",
			options:  nil,
		},
		{
			testName: "CustomOptions",
			options: &SubscriptionOptions{
				ManualAcks:  true,
				Queue:       "durableQueue",
				DurableName: "DurableName",
				MaxInflight: 100,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.testName, func(t *testing.T) {
			drivertest.RunConformanceTests(t, func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
				return newHarness(ctx, t, tc.options)
			}, asTests)
		})
	}
}

// These are stanpubsub specific to increase coverage.

// If we only send a body we should be able to get that from a direct STAN subscriber.
func TestInteropWithDirectSTAN(t *testing.T) {
	ctx := context.Background()
	dh, err := newHarness(ctx, t, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer dh.Close()
	conn := dh.(*harness).sc

	const topic = "foo"
	body := []byte("hello")

	// Send a message using Go CDK and receive it using STAN directly.
	pt, err := OpenTopic(conn, topic, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer pt.Shutdown(ctx)
	nsub, _ := conn.Subscribe(topic, func(m *stan.Msg) {
		if !bytes.Equal(m.Data, body) {
			t.Fatalf("Data did not match. %q vs %q\n", m.Data, body)
		}
	})
	if err = pt.Send(ctx, &pubsub.Message{Body: body}); err != nil {
		t.Fatal(err)
	}
	nsub.Close()

	// Send a message using STAN directly and receive it using Go CDK.
	ps, err := OpenSubscription(conn, topic, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Shutdown(ctx)
	if err := conn.Publish(topic, body); err != nil {
		t.Fatal(err)
	}
	msg, err := ps.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer msg.Ack()
	if !bytes.Equal(msg.Body, body) {
		t.Fatalf("Data did not match. %q vs %q\n", msg.Body, body)
	}
}

func TestErrorCode(t *testing.T) {
	ctx := context.Background()
	dh, err := newHarness(ctx, t, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer dh.Close()
	h := dh.(*harness)

	// Topics
	dt, err := openTopic(h.sc, "bar")
	if err != nil {
		t.Fatal(err)
	}

	if gce := dt.ErrorCode(nil); gce != gcerrors.OK {
		t.Fatalf("Expected %v, got %v", gcerrors.OK, gce)
	}
	if gce := dt.ErrorCode(context.Canceled); gce != gcerrors.Canceled {
		t.Fatalf("Expected %v, got %v", gcerrors.Canceled, gce)
	}
	if gce := dt.ErrorCode(nats.ErrBadSubject); gce != gcerrors.FailedPrecondition {
		t.Fatalf("Expected %v, got %v", gcerrors.FailedPrecondition, gce)
	}
	if gce := dt.ErrorCode(nats.ErrAuthorization); gce != gcerrors.PermissionDenied {
		t.Fatalf("Expected %v, got %v", gcerrors.PermissionDenied, gce)
	}
	if gce := dt.ErrorCode(nats.ErrMaxPayload); gce != gcerrors.ResourceExhausted {
		t.Fatalf("Expected %v, got %v", gcerrors.ResourceExhausted, gce)
	}
	if gce := dt.ErrorCode(nats.ErrReconnectBufExceeded); gce != gcerrors.ResourceExhausted {
		t.Fatalf("Expected %v, got %v", gcerrors.ResourceExhausted, gce)
	}

	// Subscriptions
	ds, err := openSubscription(h.sc, "bar", nil)
	if err != nil {
		t.Fatal(err)
	}
	if gce := ds.ErrorCode(nil); gce != gcerrors.OK {
		t.Fatalf("Expected %v, got %v", gcerrors.OK, gce)
	}
	if gce := ds.ErrorCode(context.Canceled); gce != gcerrors.Canceled {
		t.Fatalf("Expected %v, got %v", gcerrors.Canceled, gce)
	}
	if gce := ds.ErrorCode(nats.ErrBadSubject); gce != gcerrors.FailedPrecondition {
		t.Fatalf("Expected %v, got %v", gcerrors.FailedPrecondition, gce)
	}
	if gce := ds.ErrorCode(nats.ErrBadSubscription); gce != gcerrors.NotFound {
		t.Fatalf("Expected %v, got %v", gcerrors.NotFound, gce)
	}
	if gce := ds.ErrorCode(nats.ErrTypeSubscription); gce != gcerrors.FailedPrecondition {
		t.Fatalf("Expected %v, got %v", gcerrors.FailedPrecondition, gce)
	}
	if gce := ds.ErrorCode(nats.ErrAuthorization); gce != gcerrors.PermissionDenied {
		t.Fatalf("Expected %v, got %v", gcerrors.PermissionDenied, gce)
	}
	if gce := ds.ErrorCode(nats.ErrMaxMessages); gce != gcerrors.ResourceExhausted {
		t.Fatalf("Expected %v, got %v", gcerrors.ResourceExhausted, gce)
	}
	if gce := ds.ErrorCode(nats.ErrSlowConsumer); gce != gcerrors.ResourceExhausted {
		t.Fatalf("Expected %v, got %v", gcerrors.ResourceExhausted, gce)
	}

	if gce := ds.ErrorCode(nats.ErrTimeout); gce != gcerrors.DeadlineExceeded {
		t.Fatalf("Expected %v, got %v", gcerrors.DeadlineExceeded, gce)
	}

	// Queue Subscription
	qs, err := openSubscription(h.sc, "bar", nil)
	if err != nil {
		t.Fatal(err)
	}
	if gce := qs.ErrorCode(nil); gce != gcerrors.OK {
		t.Fatalf("Expected %v, got %v", gcerrors.OK, gce)
	}
	if gce := qs.ErrorCode(context.Canceled); gce != gcerrors.Canceled {
		t.Fatalf("Expected %v, got %v", gcerrors.Canceled, gce)
	}
	if gce := qs.ErrorCode(nats.ErrBadSubject); gce != gcerrors.FailedPrecondition {
		t.Fatalf("Expected %v, got %v", gcerrors.FailedPrecondition, gce)
	}
	if gce := qs.ErrorCode(nats.ErrBadSubscription); gce != gcerrors.NotFound {
		t.Fatalf("Expected %v, got %v", gcerrors.NotFound, gce)
	}
	if gce := qs.ErrorCode(nats.ErrTypeSubscription); gce != gcerrors.FailedPrecondition {
		t.Fatalf("Expected %v, got %v", gcerrors.FailedPrecondition, gce)
	}
	if gce := qs.ErrorCode(nats.ErrAuthorization); gce != gcerrors.PermissionDenied {
		t.Fatalf("Expected %v, got %v", gcerrors.PermissionDenied, gce)
	}
	if gce := qs.ErrorCode(nats.ErrMaxMessages); gce != gcerrors.ResourceExhausted {
		t.Fatalf("Expected %v, got %v", gcerrors.ResourceExhausted, gce)
	}
	if gce := qs.ErrorCode(nats.ErrSlowConsumer); gce != gcerrors.ResourceExhausted {
		t.Fatalf("Expected %v, got %v", gcerrors.ResourceExhausted, gce)
	}
	if gce := qs.ErrorCode(nats.ErrTimeout); gce != gcerrors.DeadlineExceeded {
		t.Fatalf("Expected %v, got %v", gcerrors.DeadlineExceeded, gce)
	}
}

func BenchmarkNatsStreamingQueuePubSub(b *testing.B) {
	ctx := context.Background()

	o := nsserver.GetDefaultOptions()
	opts := nsserver.DefaultNatsServerOptions
	opts.Port = benchPort
	o.ID = testClusterID
	s, err := nsserver.Run(o, &opts)
	if err != nil {
		b.Fatalf("nats-streaming: %v", err)
	}
	defer s.Shutdown()

	nc, err := stan.Connect(testClusterID, testClientID, stan.NatsURL(fmt.Sprintf("nats://127.0.0.1:%d", benchPort)))
	if err != nil {
		b.Fatal(err)
	}
	defer nc.Close()

	op := SubscriptionOptions{Queue: b.Name()}
	h := &harness{ns: s, sc: nc, options: &op}
	dt, cleanup, err := h.CreateTopic(ctx, b.Name())
	if err != nil {
		b.Fatal(err)
	}
	defer cleanup()

	qs, cleanup, err := h.CreateSubscription(ctx, dt, b.Name())
	if err != nil {
		b.Fatal(err)
	}
	defer cleanup()

	recvBatcherOpts := &batcher.Options{MaxBatchSize: stan.DefaultMaxInflight}

	topic := pubsub.NewTopic(dt, nil)
	defer topic.Shutdown(ctx)
	queueSub := pubsub.NewSubscription(qs, recvBatcherOpts, nil)
	defer queueSub.Shutdown(ctx)

	drivertest.RunBenchmarks(b, topic, queueSub)
}

func BenchmarkNatsStreamingPubSub(b *testing.B) {
	ctx := context.Background()

	o := nsserver.GetDefaultOptions()
	opts := nsserver.NewNATSOptions()
	opts.Port = benchPort
	o.ID = testClusterID
	s, err := nsserver.Run(o, opts)
	if err != nil {
		b.Fatalf("nats-streaming: %v", err)
	}
	defer s.Shutdown()

	nc, err := stan.Connect(testClusterID, testClientID, stan.NatsURL(fmt.Sprintf("nats://127.0.0.1:%d", benchPort)))
	if err != nil {
		b.Fatal(err)
	}
	defer nc.Close()

	h := &harness{ns: s, sc: nc}
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

	topic := pubsub.NewTopic(dt, nil)
	defer topic.Shutdown(ctx)

	recvBatcherOpts := &batcher.Options{MaxBatchSize: stan.DefaultMaxInflight}
	sub := pubsub.NewSubscription(ds, recvBatcherOpts, nil)
	defer sub.Shutdown(ctx)

	drivertest.RunBenchmarks(b, topic, sub)
}

func testURLOpener() (*URLOpener, error) {
	oldEnvVal := os.Getenv("STAN_SERVER_URL")
	os.Setenv("STAN_SERVER_URL", fmt.Sprintf("nats://localhost:%d", testPort))
	defer os.Setenv("STAN_SERVER_URL", oldEnvVal)
	oldCLVal := os.Getenv("STAN_CLUSTER_ID")
	os.Setenv("STAN_CLUSTER_ID", testClusterID)
	defer os.Setenv("STAN_CLUSTER_ID", oldCLVal)
	oldCVal := os.Getenv("STAN_CLIENT_ID")
	os.Setenv("STAN_CLIENT_ID", nextClientID())
	defer os.Setenv("STAN_CLIENT_ID", oldCVal)
	return connect()
}

func TestOpenTopicFromURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	o := nsserver.GetDefaultOptions()
	o.ID = testClusterID
	ns, err := newStanServer(o)
	if err != nil {
		t.Fatal(err)
	}
	defer ns.Shutdown()

	u, err := testURLOpener()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"stan://mytopic", false},
		// Invalid parameter.
		{"stan://mytopic?param=value", true},
	}

	for _, test := range tests {
		tu, err := url.Parse(test.URL)
		if err != nil {
			t.Errorf("parsing: %s failed: %v", test.URL, err)
			continue
		}
		topic, err := u.OpenTopicURL(ctx, tu)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
		if topic != nil {
			if err = topic.Shutdown(ctx); err != nil {
				t.Errorf("closing topic: %s failed: %v", test.URL, err)
			}
		}

	}
}

func TestOpenSubscriptionFromURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	o := nsserver.GetDefaultOptions()
	o.ID = testClusterID
	ns, err := newStanServer(o)
	if err != nil {
		t.Fatal(err)
	}
	defer ns.Shutdown()

	uo, err := testURLOpener()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		TestName string
		URL      string
		WantErr  bool
	}{
		// OK.
		{"OK", "stan://mytopic", false},
		// Invalid parameter.
		{"InvalidParameter", "stan://mytopic?param=value", true},
		// Queue URL Parameter for QueueSubscription.
		{"Queue", "stan://mytopic?queue=queue1", false},
		// DurableName URL Parameter.
		{"DurableName", "stan://mytopic?durable_name=durableName", false},
		// MaxInflight URL Parameter.
		{"MaxInflight", "stan://mytopic?max_inflight=200", false},
		// StartSequence URL Parameter.
		{"StartSequence", "stan://mytopic?start_sequence=1", false},
		// AckWait URL Parameter.
		{"AckWait", "stan://mytopic?ack_wait=5s", false},
		// ManualAcks Parameter.
		{"ManualAcks", "stan://mytopic?manual_acks", false},
	}

	for _, test := range tests {
		t.Run(test.TestName, func(t *testing.T) {
			u, err := url.Parse(test.URL)
			if err != nil {
				t.Errorf("parsing URL: '%s' failed: %v", test.URL, err)
				return
			}

			sub, err := uo.OpenSubscriptionURL(ctx, u)
			if (err != nil) != test.WantErr {
				t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
			}
			if sub != nil {
				sub.Shutdown(ctx)
			}
		})
	}
}

func TestCodec(t *testing.T) {
	var buf bytes.Buffer
	for _, dm := range []*driver.Message{
		{Metadata: nil, Body: nil},
		{Metadata: map[string]string{"a": "1"}, Body: nil},
		{Metadata: nil, Body: []byte("hello")},
		{Metadata: map[string]string{"a": "1"}, Body: []byte("hello")},
		{Metadata: map[string]string{"a": "1"}, Body: []byte("hello"),
			AckID: "foo", AsFunc: func(interface{}) bool { return true }},
	} {
		bytes, err := encodeMessage(&buf, dm)
		if err != nil {
			t.Fatal(err)
		}
		buf.Reset()
		var got driver.Message
		if err := decodeMessage(bytes, &got); err != nil {
			t.Fatal(err)
		}
		want := *dm
		want.AckID = nil
		want.AsFunc = nil
		if diff := cmp.Diff(got, want); diff != "" {
			t.Errorf("%+v:\n%s", want, diff)
		}
	}
}
