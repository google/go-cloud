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

package rabbitpubsub

// To run these tests against a real RabbitMQ server, first run:
//     docker run -d --hostname my-rabbit --name rabbit -p 5672:5672 rabbitmq:3
// Then wait a few seconds for the server to be ready.
// If no server is running, the tests will use a fake (see fake_tset.go).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/streadway/amqp"
	"gocloud.dev/internal/pubsub"
	"gocloud.dev/internal/pubsub/driver"
	"gocloud.dev/internal/pubsub/drivertest"
)

const rabbitURL = "amqp://guest:guest@localhost:5672/"

var logOnce sync.Once

func mustDialRabbit(t *testing.T) amqpConnection {
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		logOnce.Do(func() {
			t.Logf("using the fake because the RabbitMQ server is not up (dial error: %v)", err)
		})
		return newFakeConnection()
	}
	logOnce.Do(func() {
		t.Logf("using the RabbitMQ server at %s", rabbitURL)
	})
	return &connection{conn}
}

func TestConformance(t *testing.T) {
	harnessMaker := func(_ context.Context, t *testing.T) (drivertest.Harness, error) {
		return &harness{conn: mustDialRabbit(t)}, nil
	}
	_, isFake := mustDialRabbit(t).(*fakeConnection)
	asTests := []drivertest.AsTest{
		rabbitAsTest{isFake},
	}
	drivertest.RunConformanceTests(t, harnessMaker, asTests)
}

type harness struct {
	conn amqpConnection
	uid  int32 // atomic. Unique ID, so tests don't interact with each other.
}

func (h *harness) newName(prefix string) string {
	return fmt.Sprintf("%s%d", prefix, atomic.AddInt32(&h.uid, 1))
}

func (h *harness) MakeTopic(context.Context) (driver.Topic, error) {
	exchange := h.newName("t")
	if err := declareExchange(h.conn, exchange); err != nil {
		return nil, err
	}
	return newTopic(h.conn, exchange), nil
}

func (h *harness) MakeNonexistentTopic(context.Context) (driver.Topic, error) {
	return newTopic(h.conn, "nonexistent-topic"), nil
}

func (h *harness) MakeSubscription(_ context.Context, dt driver.Topic) (driver.Subscription, error) {
	queue := h.newName("s")
	if err := bindQueue(h.conn, queue, dt.(*topic).exchange); err != nil {
		return nil, err
	}
	return newSubscription(h.conn, queue), nil
}

func (h *harness) MakeNonexistentSubscription(_ context.Context) (driver.Subscription, error) {
	return newSubscription(h.conn, "nonexistent-subscription"), nil
}

func (h *harness) Close() {
	h.conn.Close()
}

// This test is important for the RabbitMQ driver because the underlying client is
// poorly designed with respect to concurrency, so we must make sure to exercise the
// driver with concurrent calls.
//
// We can't make this a conformance test at this time because there is no way
// to set the batcher's maxHandlers parameter to anything other than 1.
func TestPublishConcurrently(t *testing.T) {
	// See if we can call SendBatch concurrently without deadlock or races.
	ctx := context.Background()
	conn := mustDialRabbit(t)
	defer conn.Close()

	if err := declareExchange(conn, "t"); err != nil {
		t.Fatal(err)
	}
	// The queue is needed, or RabbitMQ says the message is unroutable.
	if err := bindQueue(conn, "s", "t"); err != nil {
		t.Fatal(err)
	}

	top := newTopic(conn, "t")
	errc := make(chan error, 100)
	for g := 0; g < cap(errc); g++ {
		g := g
		go func() {
			var msgs []*driver.Message
			for i := 0; i < 10; i++ {
				msgs = append(msgs, &driver.Message{
					Metadata: map[string]string{"a": strconv.Itoa(i)},
					Body:     []byte(fmt.Sprintf("msg-%d-%d", g, i)),
				})
			}
			errc <- top.SendBatch(ctx, msgs)
		}()
	}
	for i := 0; i < cap(errc); i++ {
		if err := <-errc; err != nil {
			t.Fatal(err)
		}
	}
}

func TestUnroutable(t *testing.T) {
	// Expect that we get an error on publish if the exchange has no queue bound to it.
	// The error should be a MultiError containing one error per message.
	ctx := context.Background()
	conn := mustDialRabbit(t)
	defer conn.Close()

	if err := declareExchange(conn, "u"); err != nil {
		t.Fatal(err)
	}
	top := newTopic(conn, "u")
	msgs := []*driver.Message{
		{Body: []byte("")},
		{Body: []byte("")},
	}
	err := top.SendBatch(ctx, msgs)
	merr, ok := err.(MultiError)
	if !ok {
		t.Fatalf("got error of type %T, want MultiError", err)
	}
	if got, want := len(merr), len(msgs); got != want {
		t.Fatalf("got %d errors, want %d", got, want)
	}
	for i, err := range merr {
		if !strings.Contains(err.Error(), "NO_ROUTE") {
			t.Errorf("%d: got %v, want an error with 'NO_ROUTE'", i, err)
		}
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
		return err
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

func (r rabbitAsTest) TopicCheck(top *pubsub.Topic) error {
	var conn2 amqp.Connection
	if top.As(&conn2) {
		return fmt.Errorf("cast succeeded for %T, want failure", &conn2)
	}
	if !r.usingFake {
		var conn3 *amqp.Connection
		if !top.As(&conn3) {
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
