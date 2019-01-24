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
	// Open a gRPC connection to the GCP Pub Sub API.
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

	t := gcppubsub.OpenTopic(ctx, pubClient, proj, "example-topic", nil)
	if err := t.Send(ctx, &pubsub.Message{Body: []byte("example message")}); err != nil {
		log.Fatal(err)
	}

	s := gcppubsub.OpenSubscription(ctx, subClient, proj, "example-subscription", nil)
	_, err = s.Receive(ctx)
	if err != nil {
		log.Fatal(err)
	}
}
