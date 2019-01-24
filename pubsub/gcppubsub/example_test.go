// Copyright 2018 The Go Cloud Authors
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
	"fmt"
	"log"

	"gocloud.dev/gcp"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/gcppubsub"
	"golang.org/x/oauth2/google"
)

func Example() {
	ctx := context.Background()
	scope := "https://www.googleapis.com/auth/cloud-platform"
	creds, err := google.FindDefaultCredentials(ctx, scope)
	if err != nil {
		log.Fatal(err)
	}
	conn, cleanup, err := gcppubsub.Dial(ctx, creds.TokenSource)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()
	pubClient, err := gcppubsub.PublisherClient(ctx, conn)
	if err != nil {
		log.Fatal(err)
	}
	defer pubClient.Close()
	subClient, err := gcppubsub.SubscriberClient(ctx, conn)
	if err != nil {
		log.Fatal(err)
	}
	defer subClient.Close()
	proj := gcp.ProjectID("gcppubsub-example-project")

	// Create a *pubsub.Topic
	t := gcppubsub.OpenTopic(ctx, pubClient, proj, "example-topic", nil)

	// Create a *pubsub.Subscription
	s := gcppubsub.OpenSubscription(ctx, subClient, proj, "example-subscription", nil)

	if err := t.Send(ctx, &pubsub.Message{Body: []byte("example message")}); err != nil {
		fmt.Println(err)
	}

	_, err = s.Receive(ctx)
	if err != nil {
		fmt.Println(err)
	}

	// Error messages are expected because we don't expect a project named gcppubsub-example-project to exist.

	// Output:
	// rpc error: code = NotFound desc = Requested project not found or user does not have access to it (project=gcppubsub-example-project). Make sure to specify the unique project identifier and not the Google Cloud Console display name.
	// rpc error: code = NotFound desc = Requested project not found or user does not have access to it (project=gcppubsub-example-project). Make sure to specify the unique project identifier and not the Google Cloud Console display name.
}
