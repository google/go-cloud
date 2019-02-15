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

package main

import (
	"context"
	"fmt"

	raw "cloud.google.com/go/pubsub/apiv1"
	"gocloud.dev/gcp"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/gcppubsub"
	"golang.org/x/oauth2/google"
)

// openGCPSubscription returns the GCP topic based on the subscription ID.
func openGCPSubscription(ctx context.Context, proj, subID string) (*pubsub.Subscription, func(), error) {
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("getting default credentials: %v", err)
	}
	subClient, cleanup, err := openGCPSubscriberClient(ctx, creds)
	if err != nil {
		return nil, nil, err
	}
	sub := gcppubsub.OpenSubscription(subClient, gcp.ProjectID(proj), subID, nil)
	cleanup2 := func() {
		sub.Shutdown(ctx)
		cleanup()
	}
	return sub, cleanup2, nil
}

// openGCPSubscriberClient opens a GCP SubscriberClient with default credentials.
func openGCPSubscriberClient(ctx context.Context, creds *google.Credentials) (*raw.SubscriberClient, func(), error) {
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("getting default credentials: %v", err)
	}
	ts := gcp.CredentialsTokenSource(creds)
	conn, cleanup, err := gcppubsub.Dial(ctx, ts)
	if err != nil {
		return nil, nil, fmt.Errorf("dialing grpc endpoint: %v", err)
	}
	subClient, err := gcppubsub.SubscriberClient(ctx, conn)
	if err != nil {
		return nil, nil, fmt.Errorf("making publisher client: %v", err)
	}
	cleanup2 := func() {
		subClient.Close()
		cleanup()
	}
	return subClient, cleanup2, nil
}

func gcpSubscriptionName(proj gcp.ProjectID, subscriptionID string) string {
	return fmt.Sprintf("projects/%s/subscriptions/%s", proj, subscriptionID)
}

// openGCPTopic returns the GCP topic based on the project and topic ID.
func openGCPTopic(ctx context.Context, proj, topicID string) (*pubsub.Topic, func(), error) {
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("getting default credentials: %v", err)
	}
	pubClient, cleanup, err := openGCPPubClient(ctx, creds)
	if err != nil {
		return nil, nil, err
	}
	topic := gcppubsub.OpenTopic(pubClient, gcp.ProjectID(proj), topicID, nil)
	cleanup2 := func() {
		topic.Shutdown(ctx)
		cleanup()
	}
	return topic, cleanup2, nil
}

func openGCPPubClient(ctx context.Context, creds *google.Credentials) (*raw.PublisherClient, func(), error) {
	ts := gcp.CredentialsTokenSource(creds)
	conn, cleanup, err := gcppubsub.Dial(ctx, ts)
	if err != nil {
		return nil, nil, fmt.Errorf("dialing grpc endpoint: %v", err)
	}
	pubClient, err := gcppubsub.PublisherClient(ctx, conn)
	if err != nil {
		return nil, nil, fmt.Errorf("making publisher client: %v", err)
	}
	cleanup2 := func() {
		pubClient.Close()
		cleanup()
	}
	return pubClient, cleanup2, nil
}
