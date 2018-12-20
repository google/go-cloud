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

package main

import (
	"context"
	"fmt"
	"os"

	raw "cloud.google.com/go/pubsub/apiv1"
	"gocloud.dev/gcp"
	"gocloud.dev/internal/pubsub"
	"gocloud.dev/internal/pubsub/gcppubsub"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	grpccreds "google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
)

// openGCPSubscription returns the GCP topic based on the subscription ID.
func openGCPSubscription(ctx context.Context, subID string) (*pubsub.Subscription, func(), error) {
	subClient, cleanup, err := openGCPSubscriberClient(ctx)
	if err != nil {
		return nil, nil, err
	}
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("getting default credentials: %v", err)
	}
	proj, err := gcpProjectID(creds)
	if err != nil {
		return nil, nil, err
	}
	sub := gcppubsub.OpenSubscription(ctx, subClient, proj, subID, nil)
	cleanup2 := func() {
		sub.Shutdown(ctx)
		cleanup()
	}
	return sub, cleanup2, nil
}

// openGCPSubscriberClient opens a GCP SubscriberClient with default credentials.
func openGCPSubscriberClient(ctx context.Context) (*raw.SubscriberClient, func(), error) {
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("getting default credentials: %v", err)
	}
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(grpccreds.NewClientTLSFromCert(nil, "")),
		grpc.WithPerRPCCredentials(oauth.TokenSource{TokenSource: gcp.CredentialsTokenSource(creds)}),
	}
	conn, err := grpc.DialContext(ctx, gcppubsub.EndPoint, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("dialing grpc endpoint: %v", err)
	}
	subClient, err := raw.NewSubscriberClient(ctx, option.WithGRPCConn(conn))
	if err != nil {
		return nil, nil, fmt.Errorf("making publisher client: %v", err)
	}
	cleanup := func() {
		conn.Close()
		subClient.Close()
	}
	return subClient, cleanup, nil
}

func gcpSubscriptionName(proj gcp.ProjectID, subscriptionID string) string {
	return fmt.Sprintf("projects/%s/subscriptions/%s", proj, subscriptionID)
}

// openGCPTopic returns the GCP topic based on the topic ID.
func openGCPTopic(ctx context.Context, topicID string) (*pubsub.Topic, func(), error) {
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("getting default credentials: %v", err)
	}
	pubClient, cleanup, err := openGCPPubClient(ctx, creds)
	if err != nil {
		return nil, nil, err
	}
	proj, err := gcpProjectID(creds)
	if err != nil {
		return nil, nil, fmt.Errorf("getting default project: %v", err)
	}
	topic := gcppubsub.OpenTopic(ctx, pubClient, proj, topicID, nil)
	cleanup2 := func() {
		topic.Shutdown(ctx)
		cleanup()
	}
	return topic, cleanup2, nil
}

func openGCPPubClient(ctx context.Context, creds *google.Credentials) (*raw.PublisherClient, func(), error) {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(grpccreds.NewClientTLSFromCert(nil, "")),
		grpc.WithPerRPCCredentials(oauth.TokenSource{TokenSource: gcp.CredentialsTokenSource(creds)}),
	}
	conn, err := grpc.DialContext(ctx, gcppubsub.EndPoint, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("dialing grpc endpoint: %v", err)
	}
	pubClient, err := raw.NewPublisherClient(ctx, option.WithGRPCConn(conn))
	if err != nil {
		return nil, nil, fmt.Errorf("making publisher client: %v", err)
	}
	cleanup := func() {
		conn.Close()
		pubClient.Close()
	}
	return pubClient, cleanup, nil
}

func gcpProjectID(creds *google.Credentials) (gcp.ProjectID, error) {
	p := os.Getenv("PROJECT_ID")
	if p == "" {
		return gcp.DefaultProjectID(creds)
	}
	return gcp.ProjectID(p), nil
}
