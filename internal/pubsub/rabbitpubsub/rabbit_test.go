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
	"testing"

	"github.com/google/go-cloud/internal/pubsub/driver"
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

func TestPublishConcurrently(t *testing.T) {
	// See if we can call SendBatch concurrently without deadlock or races.
	ctx := context.Background()
	conn := mustDialRabbit(t)
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	const exchangeName = "t"
	err = ch.ExchangeDeclare(
		exchangeName,
		"fanout", // kind
		false,    // durable
		false,    // delete when unused
		false,    // internal
		false,    // no-wait
		nil)      // args
	if err != nil {
		t.Fatal(err)
	}
	ch.Close()

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

func TestPublishNoExchange(t *testing.T) {
	// Verify that we get an error if publishing to a topic for which there is
	// no exchange.
	ctx := context.Background()
	conn := mustDialRabbit(t)
	defer conn.Close()

	top, err := newTopic(conn, "nonexistent-exchange")
	if err != nil {
		t.Fatal(err)
	}
	err = top.SendBatch(ctx, []*driver.Message{{
		Metadata: map[string]string{"a": "1"},
		Body:     []byte("body"),
	}})
	if err == nil {
		t.Error("got nil, wanted error")
	}
}
