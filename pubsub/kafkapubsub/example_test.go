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
	// This example is used in https://gocloud.dev/howto/pubsub/publish/#kafka-ctor

	// Variables set up elsewhere:
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
	// This example is used in https://gocloud.dev/howto/pubsub/subscribe/#kafka-ctor

	// Variables set up elsewhere:
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

func Example_openTopic() {
	// This example is used in https://gocloud.dev/howto/pubsub/publish/#kafka

	// import _ "gocloud.dev/pubsub/kafkapubsub"

	// Variables set up elsewhere:
	ctx := context.Background()

	// OpenTopic creates a *pubsub.Topic from a URL.
	// The host + path are the topic name to send to.
	// The set of brokers must be in an environment variable KAFKA_BROKERS.
	topic, err := pubsub.OpenTopic(ctx, "kafka://my-topic")
	if err != nil {
		log.Fatal(err)
	}
	defer topic.Shutdown(ctx)
}

func Example_openSubscription() {
	// This example is used in https://gocloud.dev/howto/pubsub/subscribe/#kafka

	// import _ "gocloud.dev/pubsub/kafkapubsub"

	// Variables set up elsewhere:
	ctx := context.Background()

	// OpenSubscription creates a *pubsub.Subscription from a URL.
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
