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

package rabbitpubsub

// To run these tests against a real RabbitMQ server, first run localrabbit.sh.
// Then wait a few seconds for the server to be ready.
// If no server is running, the tests will use a fake (see fake_test.go).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/testing/setup"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
	"gocloud.dev/pubsub/drivertest"
)

const rabbitURL = "amqp://guest:guest@localhost:5672/"

var logOnce sync.Once

func mustDialRabbit(t testing.TB) amqpConnection {
	t.Helper()

	if !setup.HasDockerTestEnvironment() {
		logOnce.Do(func() {
			t.Log("using the fake because the RabbitMQ server is not available")
		})
		return newFakeConnection()
	}
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		t.Fatal(err)
	}
	logOnce.Do(func() {
		t.Logf("using the RabbitMQ server at %s", rabbitURL)
	})
	return &connection{conn}
}

func TestConformance(t *testing.T) {
	harnessMaker := func(_ context.Context, t *testing.T) (drivertest.Harness, error) {
		t.Helper()

		return &harness{conn: mustDialRabbit(t)}, nil
	}
	_, isFake := mustDialRabbit(t).(*fakeConnection)
	asTests := []drivertest.AsTest{rabbitAsTest{isFake}}
	drivertest.RunConformanceTests(t, harnessMaker, asTests)

	// Run the conformance tests with the fake if we haven't.
	if isFake {
		return
	}
	t.Logf("now running tests with the fake")
	harnessMaker = func(_ context.Context, t *testing.T) (drivertest.Harness, error) {
		t.Helper()

		return &harness{conn: newFakeConnection()}, nil
	}
	asTests = []drivertest.AsTest{rabbitAsTest{true}}
	drivertest.RunConformanceTests(t, harnessMaker, asTests)
}

func BenchmarkRabbit(b *testing.B) {
	ctx := context.Background()
	h := &harness{conn: mustDialRabbit(b)}
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
	sub := pubsub.NewSubscription(ds, nil, nil)
	defer sub.Shutdown(ctx)

	drivertest.RunBenchmarks(b, topic, sub)
}

type harness struct {
	conn      amqpConnection
	numTopics uint32
	numSubs   uint32
}

func (h *harness) CreateTopic(_ context.Context, testName string) (dt driver.Topic, cleanup func(), err error) {
	exchange := fmt.Sprintf("%s-topic-%d", testName, atomic.AddUint32(&h.numTopics, 1))
	if err := declareExchange(h.conn, exchange); err != nil {
		return nil, nil, err
	}
	cleanup = func() {
		ch, err := h.conn.Channel()
		if err != nil {
			panic(err)
		}
		ch.ExchangeDelete(exchange)
	}
	return newTopic(h.conn, exchange, nil), cleanup, nil
}

func (h *harness) MakeNonexistentTopic(context.Context) (driver.Topic, error) {
	return newTopic(h.conn, "nonexistent-topic", nil), nil
}

func (h *harness) CreateSubscription(_ context.Context, dt driver.Topic, testName string) (ds driver.Subscription, cleanup func(), err error) {
	queue := fmt.Sprintf("%s-subscription-%d", testName, atomic.AddUint32(&h.numSubs, 1))
	if err := bindQueue(h.conn, queue, dt.(*topic).exchange); err != nil {
		return nil, nil, err
	}
	cleanup = func() {
		ch, err := h.conn.Channel()
		if err != nil {
			panic(err)
		}
		ch.QueueDelete(queue)
	}
	ds = newSubscription(h.conn, queue, nil)
	return ds, cleanup, nil
}

func (h *harness) MakeNonexistentSubscription(_ context.Context) (driver.Subscription, func(), error) {
	return newSubscription(h.conn, "nonexistent-subscription", nil), func() {}, nil
}

func (h *harness) Close() {
	h.conn.Close()
}

func (h *harness) MaxBatchSizes() (int, int) { return 0, 0 }

func (*harness) SupportsMultipleSubscriptions() bool { return true }

func TestUnroutable(t *testing.T) {
	// Expect that we get an error on publish if the exchange has no queue bound to it.
	// The error should be a MultiError containing one error per message.
	ctx := context.Background()
	conn := mustDialRabbit(t)
	defer conn.Close()

	if err := declareExchange(conn, "u"); err != nil {
		t.Fatal(err)
	}
	topic := newTopic(conn, "u", nil)
	msgs := []*driver.Message{
		{Body: []byte("")},
		{Body: []byte("")},
	}
	err := topic.SendBatch(ctx, msgs)
	var merr MultiError
	if !topic.ErrorAs(err, &merr) {
		t.Fatalf("got error of type %T, want MultiError", err)
	}
	if got, want := len(merr), len(msgs); got != want {
		t.Fatalf("got %d errors, want %d", got, want)
	}
	// Test MultiError.Error.
	if got, want := strings.Count(merr.Error(), ";")+1, len(merr); got != want {
		t.Errorf("got %d semicolon-separated messages, want %d", got, want)
	}
	// Test each individual error.
	for i, err := range merr {
		if !strings.Contains(err.Error(), "NO_ROUTE") {
			t.Errorf("%d: got %v, want an error with 'NO_ROUTE'", i, err)
		}
	}
}

func TestErrorCode(t *testing.T) {
	for _, test := range []struct {
		in   error
		want gcerrors.ErrorCode
	}{
		{nil, gcerrors.Unknown},
		{&os.PathError{}, gcerrors.Unknown},
		{&amqp.Error{Code: amqp.SyntaxError}, gcerrors.Internal},
		{&amqp.Error{Code: amqp.NotImplemented}, gcerrors.Unimplemented},
		{&amqp.Error{Code: amqp.ContentTooLarge}, gcerrors.Unknown},
	} {
		if got := errorCode(test.in); got != test.want {
			t.Errorf("%v: got %s, want %s", test.in, got, test.want)
		}
	}
}

func TestOpens(t *testing.T) {
	ctx := context.Background()
	if got := OpenTopic(nil, "t", nil); got == nil {
		t.Error("got nil, want non-nil")
	} else {
		got.Shutdown(ctx)
	}
	if got := OpenSubscription(nil, "s", nil); got == nil {
		t.Error("got nil, want non-nil")
	} else {
		got.Shutdown(ctx)
	}
}

func TestIsRetryable(t *testing.T) {
	for _, test := range []struct {
		err  error
		want bool
	}{
		{errors.New("xyz"), false},
		{io.ErrUnexpectedEOF, false},
		{&amqp.Error{Code: amqp.AccessRefused}, false},
		{&amqp.Error{Code: amqp.ContentTooLarge}, true},
		{&amqp.Error{Code: amqp.ConnectionForced}, true},
	} {
		got := isRetryable(test.err)
		if got != test.want {
			t.Errorf("%+v: got %t, want %t", test.err, got, test.want)
		}
	}
}

func TestRunWithContext(t *testing.T) {
	// runWithContext will run its argument to completion if the context isn't done.
	e := errors.New("")
	// f sleeps for a bit just to give the scheduler a chance to run.
	f := func() error { time.Sleep(100 * time.Millisecond); return e }
	got := runWithContext(context.Background(), f)
	if want := e; got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	// runWithContext will return ctx.Err if context is done.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got = runWithContext(ctx, f)
	if want := context.Canceled; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func declareExchange(conn amqpConnection, name string) error {
	ch, err := conn.Channel()
	if err != nil {
		panic(err)
	}
	defer ch.Close()
	return ch.ExchangeDeclare(name)
}

func bindQueue(conn amqpConnection, queueName, exchangeName string) error {
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	return ch.QueueDeclareAndBind(queueName, exchangeName)
}

type rabbitAsTest struct {
	usingFake bool
}

func (rabbitAsTest) Name() string {
	return "rabbit test"
}

func (r rabbitAsTest) TopicCheck(topic *pubsub.Topic) error {
	var conn2 amqp.Connection
	if topic.As(&conn2) {
		return fmt.Errorf("cast succeeded for %T, want failure", &conn2)
	}
	if !r.usingFake {
		var conn3 *amqp.Connection
		if !topic.As(&conn3) {
			return fmt.Errorf("cast failed for %T", &conn3)
		}
	}
	return nil
}

func (r rabbitAsTest) SubscriptionCheck(sub *pubsub.Subscription) error {
	var conn2 amqp.Connection
	if sub.As(&conn2) {
		return fmt.Errorf("cast succeeded for %T, want failure", &conn2)
	}
	if !r.usingFake {
		var conn3 *amqp.Connection
		if !sub.As(&conn3) {
			return fmt.Errorf("cast failed for %T", &conn3)
		}
	}
	return nil
}

func (rabbitAsTest) TopicErrorCheck(t *pubsub.Topic, err error) error {
	var aerr *amqp.Error
	if !t.ErrorAs(err, &aerr) {
		return fmt.Errorf("failed to convert %v (%T) to an amqp.Error", err, err)
	}
	if aerr.Code != amqp.NotFound {
		return fmt.Errorf("got code %v, want NotFound", aerr.Code)
	}

	err = MultiError{err}
	var merr MultiError
	if !t.ErrorAs(err, &merr) {
		return fmt.Errorf("failed to convert %v (%T) to a MultiError", err, err)
	}
	var perr *os.PathError
	if t.ErrorAs(err, &perr) {
		return errors.New("got true for PathError, want false")
	}
	return nil
}

func (rabbitAsTest) SubscriptionErrorCheck(s *pubsub.Subscription, err error) error {
	var aerr *amqp.Error
	if !s.ErrorAs(err, &aerr) {
		return fmt.Errorf("failed to convert %v (%T) to an amqp.Error", err, err)
	}
	if aerr.Code != amqp.NotFound {
		return fmt.Errorf("got code %v, want NotFound", aerr.Code)
	}

	err = MultiError{err}
	var merr MultiError
	if !s.ErrorAs(err, &merr) {
		return fmt.Errorf("failed to convert %v (%T) to a MultiError", err, err)
	}
	var perr *os.PathError
	if s.ErrorAs(err, &perr) {
		return errors.New("got true for PathError, want false")
	}
	return nil
}

func (r rabbitAsTest) MessageCheck(m *pubsub.Message) error {
	var pd *amqp.Delivery
	if m.As(&pd) {
		return fmt.Errorf("cast succeeded for %T, want failure", &pd)
	}
	if !r.usingFake {
		var d amqp.Delivery
		if !m.As(&d) {
			return fmt.Errorf("cast failed for %T", &d)
		}
	}
	return nil
}

func (rabbitAsTest) BeforeSend(as func(any) bool) error {
	var pub *amqp.Publishing
	if !as(&pub) {
		return fmt.Errorf("cast failed for %T", &pub)
	}
	return nil
}

func (rabbitAsTest) AfterSend(as func(any) bool) error {
	return nil
}

func TestOpenTopicFromURL(t *testing.T) {
	t.Setenv("RABBIT_SERVER_URL", rabbitURL)

	tests := []struct {
		label       string
		URLTemplate string
		WantErr     bool
	}{
		{"valid url", "rabbit://%s", false},
		{"valid url with key name parameter", "rabbit://%s?key_name=foo", false},
		{"invalid url with parameters", "rabbit://%s?param=value", true},
		{"invalid url with key name parameter", "rabbit://%s?key_name=", true},
	}

	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			conn := mustDialRabbit(t)
			_, isFake := conn.(*fakeConnection)
			if isFake {
				t.Skip("test requires real rabbitmq")
			}

			h := &harness{conn: conn}

			ctx := context.Background()

			dt, cleanupTopic, err := h.CreateTopic(ctx, t.Name())
			if err != nil {
				t.Fatalf("unable to create topic: %v", err)
			}

			t.Cleanup(cleanupTopic)

			exchange := dt.(*topic).exchange
			url := fmt.Sprintf(test.URLTemplate, exchange)

			topic, err := pubsub.OpenTopic(ctx, url)
			if (err != nil) != test.WantErr {
				t.Errorf("%s: got error %v, want error %v", test.URLTemplate, err, test.WantErr)
			}
			if topic != nil {
				topic.Shutdown(ctx)
			}
		})
	}
}

func TestOpenSubscriptionFromURL(t *testing.T) {
	t.Setenv("RABBIT_SERVER_URL", rabbitURL)

	tests := []struct {
		label       string
		URLTemplate string
		WantErr     bool
	}{
		{"url with no QoS prefetch count", "rabbit://%s", false},
		{"invalid parameters", "rabbit://%s?param=value", true},
		{"valid url with QoS prefetch count", "rabbit://%s?prefetch_count=1024", false},
		{"invalid url with QoS prefetch count", "rabbit://%s?prefetch_count=value", true},
	}

	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			conn := mustDialRabbit(t)
			_, isFake := conn.(*fakeConnection)
			if isFake {
				t.Skip("test requires real rabbitmq")
			}

			h := &harness{conn: conn}

			ctx := context.Background()

			dt, cleanupTopic, err := h.CreateTopic(ctx, t.Name())
			if err != nil {
				t.Fatalf("unable to create topic: %v", err)
			}

			t.Cleanup(cleanupTopic)

			ds, cleanupSubscription, err := h.CreateSubscription(ctx, dt, t.Name())
			if err != nil {
				t.Fatalf("unable to create subscription: %v", err)
			}

			t.Cleanup(cleanupSubscription)

			queue := ds.(*subscription).queue
			url := fmt.Sprintf(test.URLTemplate, queue)

			sub, err := pubsub.OpenSubscription(ctx, url)
			if (err != nil) != test.WantErr {
				t.Errorf("%s: got error %v, want error %v", test.URLTemplate, err, test.WantErr)
			}

			if sub != nil {
				sub.Shutdown(ctx)
			}
		})
	}
}
