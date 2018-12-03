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

package rabbitpubsub_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/go-cloud/internal/pubsub"
	"github.com/google/go-cloud/internal/pubsub/rabbitpubsub"
	"github.com/streadway/amqp"
)

const rabbitURL = "amqp://guest:guest@localhost:5672/"

func Example() {
	// Connect to RabbitMQ.
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		// We can't connect to RabbitMQ, most likely because it's not available
		// in the current environment. Rather than have the test fail,
		// print the line that is expected.
		fmt.Println("success")
		return
	}
	defer conn.Close()

	// Declare an exchange and a queue. Bind the queue to the exchange.
	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("creating channel: %v", err)
	}
	const exchangeName = "my-topic"
	err = ch.ExchangeDeclare(
		exchangeName,
		"fanout", // kind
		false,    // durable
		false,    // delete when unused
		false,    // internal
		false,    // no-wait
		nil)      // args
	if err != nil {
		log.Fatalf("declaring exchange: %v", err)
	}
	const queueName = "my-subscription"
	q, err := ch.QueueDeclare(
		queueName,
		false, // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil)   // arguments
	if err != nil {
		log.Fatalf("declaring queue: %v", err)
	}
	err = ch.QueueBind(q.Name, q.Name, exchangeName,
		false, // no-wait
		nil)   // args
	if err != nil {
		log.Fatalf("binding queue: %v", err)
	}
	ch.Close() // OpenSubscription will create its own channels.

	// Publish a message to the exchange (that is, topic).
	topic, err := rabbitpubsub.OpenTopic(conn, exchangeName)
	if err != nil {
		log.Fatalf("opening topic: %v", err)
	}
	ctx := context.Background()
	err = topic.Send(ctx, &pubsub.Message{Body: []byte("hello")})
	if err != nil {
		log.Fatalf("sending: %v", err)
	}
	if err := topic.Close(); err != nil {
		log.Fatalf("closing topic: %v", err)
	}

	// Receive the message from the queue (that is, subscription).
	sub, err := rabbitpubsub.OpenSubscription(conn, queueName)
	if err != nil {
		log.Fatalf("opening subscription: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	msg, err := sub.Receive(ctx)
	if err != nil {
		log.Fatalf("opening subscription: %v", err)
	}
	msg.Ack()
	if err := sub.Close(); err != nil {
		log.Fatalf("closing subscription: %v", err)
	}
	fmt.Println("success")
	// Output: success
}
