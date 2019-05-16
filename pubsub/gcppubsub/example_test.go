// Copyright 2018 The Go Cloud Development Kit Authors
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

package gcppubsub_test

import (
	"context"
	"log"

	"gocloud.dev/gcp"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/gcppubsub"
)

func ExampleOpenTopic() {
	// This example is used in https://gocloud.dev/howto/pubsub/publish/#gcp-ctor

	// Variables set up elsewhere:
	ctx := context.Background()

	// Your GCP credentials.
	// See https://cloud.google.com/docs/authentication/production
	// for more info on alternatives.
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		log.Fatal(err)
	}
	// Get the project ID from the credentials (required by OpenTopic).
	projectID, err := gcp.DefaultProjectID(creds)
	if err != nil {
		log.Fatal(err)
	}
	// Open a gRPC connection to the GCP Pub/Sub API.
	conn, cleanup, err := gcppubsub.Dial(ctx, creds.TokenSource)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	// Construct a PublisherClient using the connection.
	pubClient, err := gcppubsub.PublisherClient(ctx, conn)
	if err != nil {
		log.Fatal(err)
	}
	defer pubClient.Close()

	// Construct a *pubsub.Topic.
	topic := gcppubsub.OpenTopic(pubClient, projectID, "example-topic", nil)
	defer topic.Shutdown(ctx)
}

func Example_openTopicFromURL() {
	// This example is used in https://gocloud.dev/howto/pubsub/publish/#gcp

	// import _ "gocloud.dev/pubsub/gcppubsub"

	// Variables set up elsewhere:
	ctx := context.Background()

	topic, err := pubsub.OpenTopic(ctx, "gcppubsub://myproject/mytopic")
	if err != nil {
		log.Fatal(err)
	}
	defer topic.Shutdown(ctx)
}

func ExampleOpenSubscription() {
	// This example is used in https://gocloud.dev/howto/pubsub/subscribe/#gcp-ctor

	// Variables set up elsewhere:
	ctx := context.Background()

	// Your GCP credentials.
	// See https://cloud.google.com/docs/authentication/production
	// for more info on alternatives.
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		log.Fatal(err)
	}
	// Get the project ID from the credentials (required by OpenSubscription).
	projectID, err := gcp.DefaultProjectID(creds)
	if err != nil {
		log.Fatal(err)
	}

	// Open a gRPC connection to the GCP Pub/Sub API.
	conn, cleanup, err := gcppubsub.Dial(ctx, creds.TokenSource)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	// Construct a SubscriberClient using the connection.
	subClient, err := gcppubsub.SubscriberClient(ctx, conn)
	if err != nil {
		log.Fatal(err)
	}
	defer subClient.Close()

	// Construct a *pubsub.Subscription.
	subscription := gcppubsub.OpenSubscription(
		subClient, projectID, "example-subscription", nil)
	defer subscription.Shutdown(ctx)
}

func Example_openSubscriptionFromURL() {
	// This example is used in https://gocloud.dev/howto/pubsub/subscribe/#gcp

	// import _ "gocloud.dev/pubsub/gcppubsub"

	// Variables set up elsewhere:
	ctx := context.Background()

	subscription, err := pubsub.OpenSubscription(ctx,
		"gcppubsub://my-project/my-subscription")
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)
}
