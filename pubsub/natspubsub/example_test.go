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

package natspubsub_test

import (
	"context"
	"github.com/nats-io/nats.go/jetstream"
	"gocloud.dev/pubsub/natspubsub"
	"gocloud.dev/pubsub/natspubsub/connections"
	"log"

	"github.com/nats-io/nats.go"

	"gocloud.dev/pubsub"
)

func ExampleOpenSubscription() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/pubsub/natspubsub"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()
	natsConn, err := nats.Connect("nats://nats.example.com")
	if err != nil {
		log.Fatal(err)
	}
	defer natsConn.Close()

	conn, err := connections.NewPlain(natsConn)
	if err != nil {
		log.Fatal(err)
	}

	subscription, err := natspubsub.OpenSubscription(
		ctx, conn, &connections.SubscriptionOptions{Subjects: []string{"example.mysubject"}})
	if err != nil {
		log.Fatal(err)
	}
	defer func(subscription *pubsub.Subscription, ctx context.Context) {
		_ = subscription.Shutdown(ctx)
	}(subscription, ctx)
}

func ExampleOpenTopic() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/pubsub/natspubsub"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	natsConn, err := nats.Connect("nats://nats.example.com")
	if err != nil {
		log.Fatal(err)
	}
	defer natsConn.Close()

	js, err := jetstream.New(natsConn)
	if err != nil {
		log.Fatal(err)
	}

	conn := connections.NewJetstream(js)

	topic, err := natspubsub.OpenTopic(ctx, conn, &connections.TopicOptions{Subject: "example.mysubject"})
	if err != nil {
		log.Fatal(err)
	}
	defer func(topic *pubsub.Topic, ctx context.Context) {
		_ = topic.Shutdown(ctx)
	}(topic, ctx)
}

func Example_openTopicFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/pubsub/natspubsub"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// pubsub.OpenTopic creates a *pubsub.Connection from a URL.
	// This URL will Dial the NATS server at the URL in the environment variable
	// NATS_SERVER_URL and send messages with subject "example.mysubject".
	// This URL will be parsed and the natsv2 attribute will be used to
	// use NATS v2.2.0+ native message headers as the message metadata.
	topic, err := pubsub.OpenTopic(ctx, "nats://nats.example.com/example.mysubject")
	if err != nil {
		log.Fatal(err)
	}
	defer func(topic *pubsub.Topic, ctx context.Context) {
		_ = topic.Shutdown(ctx)
	}(topic, ctx)
}

func Example_openSubscriptionFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/pubsub/natspubsub"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// pubsub.OpenSubscription creates a *pubsub.Subscription from a URL.
	// This URL will Dial the NATS server at the URL in the environment variable
	// NATS_SERVER_URL and receive messages with subject "example.mysubject".
	// This URL will be parsed and the natsv2 attribute will be used to
	// use NATS v2.2.0+ native message headers as the message metadata.
	subscription, err := pubsub.OpenSubscription(ctx, "nats://nats.example.com/example.mysubject?jetstream=true")
	if err != nil {
		log.Fatal(err)
	}
	defer func(subscription *pubsub.Subscription, ctx context.Context) {
		_ = subscription.Shutdown(ctx)
	}(subscription, ctx)
}
