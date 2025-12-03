// Copyright 2025 The Go Cloud Development Kit Authors
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

package gcppubsubv2_test

import (
	"context"
	"log"

	"gocloud.dev/gcp"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/gcppubsubv2"
)

func ExampleOpenTopic() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// Your GCP credentials.
	// See https://cloud.google.com/docs/authentication/production
	// for more info on alternatives.
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		log.Fatal(err)
	}
	// Open a gRPC connection to the GCP Pub/Sub API.
	// Alternatively, skip this and pass nil to Client below to use the default.
	conn, cleanup, err := gcppubsubv2.Dial(ctx, creds.TokenSource)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	// Construct a Client using the connection.
	client, err := gcppubsubv2.Client(ctx, "myprojectid", conn)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Construct a *pubsub.Topic.
	topic := gcppubsubv2.OpenTopic(client, "example-topic", nil)
	defer topic.Shutdown(ctx)
}

func Example_openTopicFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/pubsub/gcppubsubv2"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	topic, err := pubsub.OpenTopic(ctx, "gcppubsubv2://projects/myproject/topics/mytopic")
	if err != nil {
		log.Fatal(err)
	}
	defer topic.Shutdown(ctx)
}

func ExampleOpenSubscription() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// Your GCP credentials.
	// See https://cloud.google.com/docs/authentication/production
	// for more info on alternatives.
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Open a gRPC connection to the GCP Pub/Sub API.
	// Alternatively, skip this and pass nil to Client below to use the default.
	conn, cleanup, err := gcppubsubv2.Dial(ctx, creds.TokenSource)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	// Construct a Client using the connection.
	client, err := gcppubsubv2.Client(ctx, "myprojectID", conn)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Construct a *pubsub.Subscription.
	subscription := gcppubsubv2.OpenSubscription(client, "example-subscription", nil)
	defer subscription.Shutdown(ctx)
}

func Example_openSubscriptionFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/pubsub/gcppubsubv2"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	subscription, err := pubsub.OpenSubscription(ctx,
		"gcppubsubv2://projects/my-project/subscriptions/my-subscription")
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)
}
