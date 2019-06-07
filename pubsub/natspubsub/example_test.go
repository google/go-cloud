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
	"log"

	"github.com/nats-io/nats.go"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/natspubsub"
)

func ExampleOpenTopic() {
	// This example is used in https://gocloud.dev/howto/pubsub/publish/#nats-ctor

	// Variables set up elsewhere:
	ctx := context.Background()

	natsConn, err := nats.Connect("nats://nats.example.com")
	if err != nil {
		log.Fatal(err)
	}
	defer natsConn.Close()

	topic, err := natspubsub.OpenTopic(natsConn, "example.mysubject", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer topic.Shutdown(ctx)
}

func ExampleOpenSubscription() {
	// This example is used in https://gocloud.dev/howto/pubsub/subscribe/#nats-ctor

	// Variables set up elsewhere:
	ctx := context.Background()

	natsConn, err := nats.Connect("nats://nats.example.com")
	if err != nil {
		log.Fatal(err)
	}
	defer natsConn.Close()

	subscription, err := natspubsub.OpenSubscription(
		natsConn,
		"example.mysubject",
		nil)
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)
}

func Example_openTopicFromURL() {
	// This example is used in https://gocloud.dev/howto/pubsub/publish/#nats

	// import _ "gocloud.dev/pubsub/natspubsub"

	// Variables set up elsewhere:
	ctx := context.Background()

	// pubsub.OpenTopic creates a *pubsub.Topic from a URL.
	// This URL will Dial the NATS server at the URL in the environment variable
	// NATS_SERVER_URL and send messages with subject "example.mysubject".
	topic, err := pubsub.OpenTopic(ctx, "nats://example.mysubject")
	if err != nil {
		log.Fatal(err)
	}
	defer topic.Shutdown(ctx)
}

func Example_openSubscriptionFromURL() {
	// This example is used in https://gocloud.dev/howto/pubsub/subscribe/#nats

	// import _ "gocloud.dev/pubsub/natspubsub"

	// Variables set up elsewhere:
	ctx := context.Background()

	// pubsub.OpenSubscription creates a *pubsub.Subscription from a URL.
	// This URL will Dial the NATS server at the URL in the environment variable
	// NATS_SERVER_URL and receive messages with subject "example.mysubject".
	subscription, err := pubsub.OpenSubscription(ctx, "nats://example.mysubject")
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)
}
