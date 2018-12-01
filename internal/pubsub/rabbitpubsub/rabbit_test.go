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

// To run these tests, first run:
//     docker run -d --hostname my-rabbit --name rabbit -p 5672:5672 rabbitmq:3
// Then wait a few seconds for the server to be ready.

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/google/go-cloud/internal/pubsub/driver"
	"github.com/google/go-cloud/internal/pubsub/drivertest"
	"github.com/streadway/amqp"
)

const rabbitURL = "amqp://guest:guest@localhost:5672/"

func mustDialRabbit(t *testing.T) *amqp.Connection {
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		t.Skipf("skipping because the RabbitMQ server is not up (dial error: %v)", err)
	}
	return conn
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, func(_ context.Context, t *testing.T) (drivertest.Harness, error) {
		return &harness{conn: mustDialRabbit(t)}, nil
	})
}

type harness struct {
	conn *amqp.Connection
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
	return newTopic(h.conn, exchange)
}

func (h *harness) MakeNonexistentTopic(context.Context) (driver.Topic, error) {
	return newTopic(h.conn, "nonexistent-topic")
}

func (h *harness) MakeSubscription(_ context.Context, dt driver.Topic) (driver.Subscription, error) {
	queue := h.newName("s")
	if err := bindQueue(h.conn, queue, dt.(*topic).exchange); err != nil {
		return nil, err
	}
	return newSubscription(h.conn, queue)
}

func (h *harness) MakeNonexistentSubscription(_ context.Context) (driver.Subscription, error) {
	return nil, errors.New("unimplemented")
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

	top, err := newTopic(conn, "t")
	if err != nil {
		t.Fatal(err)
	}

	errc := make(chan error, 100)
	for g := 0; g < cap(errc); g++ {
		go func() {
			var msgs []*driver.Message
			for i := 0; i < 10; i++ {
				msgs = append(msgs, &driver.Message{
					Metadata: map[string]string{"a": strconv.Itoa(i)},
					Body:     []byte(fmt.Sprintf("msg-%d", i)),
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

func declareExchange(conn *amqp.Connection, name string) error {
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	return ch.ExchangeDeclare(
		name,
		"fanout", // kind
		false,    // durable
		false,    // delete when unused
		false,    // internal
		false,    // no-wait
		nil)      // args
}

func bindQueue(conn *amqp.Connection, queueName, exchangeName string) error {
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	q, err := ch.QueueDeclare(
		queueName,
		false, // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil)   // arguments
	if err != nil {
		return err
	}
	return ch.QueueBind(q.Name, q.Name, exchangeName,
		false, // no-wait
		nil)   // args
}
