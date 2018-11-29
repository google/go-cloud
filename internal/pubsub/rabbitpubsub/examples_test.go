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

	"github.com/google/go-cloud/internal/pubsub"
	"github.com/google/go-cloud/internal/pubsub/rabbitpubsub"
	"github.com/streadway/amqp"
)

const rabbitURL = "amqp://guest:guest@localhost:5672/"

func Example_publish() {
	// Connect to RabbitMQ.
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		log.Printf("cannot connect to RabbitMQ: %v\n", err)
		return
	}
	defer conn.Close()

	// Declare a fanout exchange to be used as a Pub/Sub topic.
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
	ch.Close()

	// Open a topic, using the exchange name.
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
	fmt.Println("message sent")
}
