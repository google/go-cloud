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

package rabbitpubsub_test

import (
	"context"
	"log"

	"github.com/streadway/amqp"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/rabbitpubsub"
)

func ExampleOpenTopic() {
	// Connect to RabbitMQ.
	conn, err := amqp.Dial("your-rabbit-url")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// Construct a *pubsub.Topic.
	ctx := context.Background()
	t := rabbitpubsub.OpenTopic(conn, "exchange-name", nil)
	defer t.Shutdown(ctx)

	// Now we can use t to send messages.
	err = t.Send(ctx, &pubsub.Message{Body: []byte("hello")})
}

func ExampleOpenSubscription() {
	// Connect to RabbitMQ.
	conn, err := amqp.Dial("your-rabbit-url")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// Construct a *pubsub.Subscription..
	ctx := context.Background()
	s := rabbitpubsub.OpenSubscription(conn, "queue-name", nil)
	defer s.Shutdown(ctx)

	// Now we can use s to receive messages.
	msg, err := s.Receive(ctx)
	if err != nil {
		log.Fatalf("opening subscription: %v", err)
	}
	msg.Ack()
}

func Example_openFromURL() {
	ctx := context.Background()

	// OpenTopic creates a *pubsub.Topic from a URL.
	// This URL will Dial the RabbitMQ server at the URL in the environment
	// variable RABBIT_SERVER_URL and open the exchange "myexchange".
	t, err := pubsub.OpenTopic(ctx, "rabbit://myexchange")

	// Similarly, OpenSubscription creates a *pubsub.Subscription from a URL.
	// This URL will use the same connection and open the queue "myqueue".
	s, err := pubsub.OpenSubscription(ctx, "rabbit://myqueue")
	_, _, _ = t, s, err
}
