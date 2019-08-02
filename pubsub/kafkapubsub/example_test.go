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

package kafkapubsub_test

import (
	"context"
	"log"

	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/kafkapubsub"
)

func ExampleOpenTopic() {
	// PRAGMA(gocloud.dev): This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA(gocloud.dev): On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// The set of brokers in the Kafka cluster.
	addrs := []string{"1.2.3.4:9092"}
	// The Kafka client configuration to use.
	config := kafkapubsub.MinimalConfig()

	// Construct a *pubsub.Topic.
	topic, err := kafkapubsub.OpenTopic(addrs, config, "my-topic", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer topic.Shutdown(ctx)
}

func ExampleOpenSubscription() {
	// PRAGMA(gocloud.dev): This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA(gocloud.dev): On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// The set of brokers in the Kafka cluster.
	addrs := []string{"1.2.3.4:9092"}
	// The Kafka client configuration to use.
	config := kafkapubsub.MinimalConfig()

	// Construct a *pubsub.Subscription, joining the consumer group "my-group"
	// and receiving messages from "my-topic".
	subscription, err := kafkapubsub.OpenSubscription(
		addrs, config, "my-group", []string{"my-topic"}, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)
}

func Example_openTopicFromURL() {
	// PRAGMA(gocloud.dev): This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA(gocloud.dev): Add a blank import: _ "gocloud.dev/pubsub/kafkapubsub"
	// PRAGMA(gocloud.dev): On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// pubsub.OpenTopic creates a *pubsub.Topic from a URL.
	// The host + path are the topic name to send to.
	// The set of brokers must be in an environment variable KAFKA_BROKERS.
	topic, err := pubsub.OpenTopic(ctx, "kafka://my-topic")
	if err != nil {
		log.Fatal(err)
	}
	defer topic.Shutdown(ctx)
}

func Example_openSubscriptionFromURL() {
	// PRAGMA(gocloud.dev): This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA(gocloud.dev): Add a blank import: _ "gocloud.dev/pubsub/kafkapubsub"
	// PRAGMA(gocloud.dev): On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// pubsub.OpenSubscription creates a *pubsub.Subscription from a URL.
	// The host + path are used as the consumer group name.
	// The "topic" query parameter sets one or more topics to subscribe to.
	// The set of brokers must be in an environment variable KAFKA_BROKERS.
	subscription, err := pubsub.OpenSubscription(ctx,
		"kafka://my-group?topic=my-topic")
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)
}
