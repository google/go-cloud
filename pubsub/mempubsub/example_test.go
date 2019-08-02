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

package mempubsub_test

import (
	"context"
	"log"
	"time"

	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/mempubsub"
)

func ExampleNewSubscription() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// Construct a *pubsub.Topic.
	topic := mempubsub.NewTopic()
	defer topic.Shutdown(ctx)

	// Construct a *pubsub.Subscription for the topic.
	subscription := mempubsub.NewSubscription(topic, 1*time.Minute /* ack deadline */)
	defer subscription.Shutdown(ctx)
}

func ExampleNewTopic() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	topic := mempubsub.NewTopic()
	defer topic.Shutdown(ctx)
}

func Example_openSubscriptionFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/pubsub/mempubsub"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// Create a topic.
	topic, err := pubsub.OpenTopic(ctx, "mem://topicA")
	if err != nil {
		log.Fatal(err)
	}
	defer topic.Shutdown(ctx)

	// Create a subscription connected to that topic.
	subscription, err := pubsub.OpenSubscription(ctx, "mem://topicA")
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)
}

func Example_openTopicFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/pubsub/mempubsub"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	topic, err := pubsub.OpenTopic(ctx, "mem://topicA")
	if err != nil {
		log.Fatal(err)
	}
	defer topic.Shutdown(ctx)
}
