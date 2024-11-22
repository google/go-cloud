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

	raw "cloud.google.com/go/pubsub/apiv1"
	pb "cloud.google.com/go/pubsub/apiv1/pubsubpb"
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
	topic, err := gcppubsub.OpenTopicByPath(pubClient, "projects/myprojectID/topics/example-topic", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer topic.Shutdown(ctx)
}

func Example_openTopicFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/pubsub/gcppubsub"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	topic, err := pubsub.OpenTopic(ctx, "gcppubsub://projects/myproject/topics/mytopic")
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
	subscription, err := gcppubsub.OpenSubscriptionByPath(
		subClient, "projects/myprojectID/subscriptions/example-subscription", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)
}

func ExampleExtendingAckDeadline() {
	ctx := context.Background()

	// Construct a *pubsub.Subscription, in this example using a URL.
	const subName = "projects/myprojectID/subscriptions/example-subscription"
	subscription, err := pubsub.OpenSubscription(ctx, "gcppubsub://"+subName)
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)

	// Get the underlying SubscriberClient. If you used the constructor to create
	// the subscription (e.g., gcppubsub.OpenSubscriptionByPath), you may already
	// have the client.
	var client *raw.SubscriberClient
	if !subscription.As(&client) {
		log.Fatal("Couldn't get SubscriberClient using As")
	}

	// Now assume you've got a message, and processing is going to take a long time;
	// you want to extend the default Ack deadline.
	msg, err := subscription.Receive(ctx)
	if err != nil {
		log.Fatalf("Failed to receive message: %v", err)
	}

	// Get the underlying ReceivedMessage.
	var rm *pb.ReceivedMessage
	if !msg.As(&rm) {
		log.Fatal("Couldn't get ReceivedMessage using As")
	}

	// Call ModifyAckDeadline.
	if err := client.ModifyAckDeadline(ctx, &pb.ModifyAckDeadlineRequest{
		Subscription:       subName,
		AckIds:             []string{rm.AckId},
		AckDeadlineSeconds: 30 * 60, // 30m, or whatever you need
	}); err != nil {
		log.Fatalf("Failed to ModifyAckDeadline: %v", err)
	}
	// ... eventually Ack the message.
	msg.Ack()
}

func Example_openSubscriptionFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/pubsub/gcppubsub"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	subscription, err := pubsub.OpenSubscription(ctx,
		"gcppubsub://projects/my-project/subscriptions/my-subscription")
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)
}
