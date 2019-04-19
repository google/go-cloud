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
	// Your GCP credentials.
	// See https://cloud.google.com/docs/authentication/production
	// for more info on alternatives.
	ctx := context.Background()
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		log.Fatal(err)
	}
	// Get the ProjectID from the credentials (it's required by OpenTopic).
	projID, err := gcp.DefaultProjectID(creds)
	if err != nil {
		log.Fatal(err)
	}

	// Open a gRPC connection to the GCP Pub Sub API.
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
	t := gcppubsub.OpenTopic(pubClient, projID, "example-topic", nil)
	defer t.Shutdown(ctx)

	// Now we can use t to send messages.
	err = t.Send(ctx, &pubsub.Message{Body: []byte("example message")})
}

func ExampleOpenSubscription() {
	// Your GCP credentials.
	// See https://cloud.google.com/docs/authentication/production
	// for more info on alternatives.
	ctx := context.Background()
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		log.Fatal(err)
	}
	// Get the ProjectID from the credentials (it's required by OpenTopic).
	projID, err := gcp.DefaultProjectID(creds)
	if err != nil {
		log.Fatal(err)
	}

	// Open a gRPC connection to the GCP Pub Sub API.
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
	s := gcppubsub.OpenSubscription(subClient, projID, "example-subscription", nil)
	defer s.Shutdown(ctx)

	// Now we can use s to receive messages.
	msg, err := s.Receive(ctx)
	if err != nil {
		// Handle error....
	}
	// Handle message....
	msg.Ack()
}

func Example_openFromURL() {
	ctx := context.Background()

	// OpenTopic creates a *pubsub.Topic from a URL.
	// This URL will open the topic "mytopic" in the project "myproject"
	// using default credentials.
	t, err := pubsub.OpenTopic(ctx, "gcppubsub://myproject/mytopic")

	// Similarly, OpenSubscription creates a *pubsub.Subscription from a URL.
	// This URL will open the subscription "mysubscription" in the project
	// "myproject" using default credentials.
	s, err := pubsub.OpenSubscription(ctx, "gcppubsub://myproject/mysubscription")
	_, _, _ = t, s, err
}
