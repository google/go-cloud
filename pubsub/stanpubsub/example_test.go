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

package stanpubsub_test

import (
	"context"
	"log"

	"github.com/nats-io/stan.go"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/stanpubsub"
)

func ExampleOpenTopic() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	stanConn, err := stan.Connect("cluster-id", "client-id", stan.NatsURL("nats://nats.example.com"))
	if err != nil {
		log.Fatal(err)
	}
	defer stanConn.Close()

	topic, err := stanpubsub.OpenTopic(stanConn, "example.mysubject", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer topic.Shutdown(ctx)
}

func ExampleOpenSubscription() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	stanConn, err := stan.Connect("cluster-id", "client-id", stan.NatsURL("nats://nats.example.com"))
	if err != nil {
		log.Fatal(err)
	}
	defer stanConn.Close()

	subscription, err := stanpubsub.OpenSubscription(
		stanConn,
		"example.mysubject",
		"",
	)
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)
}

func ExampleOpenQueueSubscription() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	stanConn, err := stan.Connect("cluster-id", "client-id", stan.NatsURL("nats://nats.example.com"))
	if err != nil {
		log.Fatal(err)
	}
	defer stanConn.Close()

	subscription, err := stanpubsub.OpenSubscription(
		stanConn,
		"example.mysubject",
		"exampleQueueGroup",
	)
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)
}

func ExampleOpenManualAckSubscription() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	stanConn, err := stan.Connect("cluster-id", "client-id", stan.NatsURL("nats://nats.example.com"))
	if err != nil {
		log.Fatal(err)
	}
	defer stanConn.Close()

	subscription, err := stanpubsub.OpenSubscription(
		stanConn,
		"example.mysubject",
		"",
		stan.SetManualAckMode(),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)
}

func Example_openTopicFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/pubsub/natspubsub"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// pubsub.OpenTopic creates a *pubsub.Topic from a URL.
	// This URL will Dial the NATS Streaming server with the connection details defined in environment variables:
	// STAN_SERVER_URL 	- url address of the NATS server.
	// STAN_CLUSTER_ID 	- NATS Streaming cluster id.
	// STAN_CLIENT_ID 	- NAST Streaming client id.
	// The message would be sent on the topic: `example.mysubject`
	topic, err := pubsub.OpenTopic(ctx, "stan://example.mysubject")
	if err != nil {
		log.Fatal(err)
	}
	defer topic.Shutdown(ctx)
}

func Example_openSubscriptionFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/pubsub/natspubsub"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// pubsub.OpenSubscription creates a *pubsub.Subscription from a URL.
	// This URL will Dial the NATS Streaming server with the connection details defined in environment variables:
	// STAN_SERVER_URL 	- url address of the NATS server.
	// STAN_CLUSTER_ID 	- NATS Streaming cluster id.
	// STAN_CLIENT_ID 	- NAST Streaming client id.
	// The message would be received from the subject: `example.mysubject`
	subscription, err := pubsub.OpenSubscription(ctx, "stan://example.mysubject")
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)
}

func Example_openQueueSubscriptionFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/pubsub/natspubsub"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// pubsub.OpenSubscription creates a *pubsub.Subscription from a URL.
	// This URL will Dial the NATS Streaming server with the connection details defined in environment variables:
	// STAN_SERVER_URL 	- url address of the NATS server.
	// STAN_CLUSTER_ID 	- NATS Streaming cluster id.
	// STAN_CLIENT_ID 	- NAST Streaming client id.
	// The message would be received from the subject: `example.mysubject`
	// This URL will be parsed and the queue attribute will be used as the QueueGroup parameter when
	// creating the NATS Streaming Subscription.
	subscription, err := pubsub.OpenSubscription(ctx, "stan://example.mysubject?queue=myqueue")
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)
}

func Example_openQueueDurableNameSubscriptionFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/pubsub/natspubsub"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// pubsub.OpenSubscription creates a *pubsub.Subscription from a URL.
	// This URL will Dial the NATS Streaming server with the connection details defined in environment variables:
	// STAN_SERVER_URL 	- url address of the NATS server.
	// STAN_CLUSTER_ID 	- NATS Streaming cluster id.
	// STAN_CLIENT_ID 	- NAST Streaming client id.
	// The message would be received from the subject: `example.mysubject`
	// This URL will be parsed and the queue attribute will be used as the Queue parameter with DurableName
	// when creating the NATS Streaming Subscription.
	subscription, err := pubsub.OpenSubscription(ctx, "stan://example.mysubject?queue=myqueue&durableName=mydurablename")
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)
}

func Example_openManualAckSubscriptionFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/pubsub/natspubsub"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// pubsub.OpenSubscription creates a *pubsub.Subscription from a URL.
	// This URL will Dial the NATS Streaming server with the connection details defined in environment variables:
	// STAN_SERVER_URL 	- url address of the NATS server.
	// STAN_CLUSTER_ID 	- NATS Streaming cluster id.
	// STAN_CLIENT_ID 	- NAST Streaming client id.
	// The message would be received from the subject: `example.mysubject`
	// This URL will be parsed and the subscription would be using manual message acks.
	subscription, err := pubsub.OpenSubscription(ctx, "stan://example.mysubject?manualAck")
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)
}
