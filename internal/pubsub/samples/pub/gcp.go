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
	"errors"
	"fmt"
	"os"

	raw "cloud.google.com/go/pubsub/apiv1"
	"gocloud.dev/gcp"
	"gocloud.dev/internal/pubsub"
	"gocloud.dev/internal/pubsub/gcppubsub"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	grpccreds "google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
)

// openGCPTopic returns the GCP topic based on the topic ID.
func openGCPTopic(ctx context.Context, topicID string) (*pubsub.Topic, func(), error) {
	pubClient, cleanup, err := openGCPPubClient(ctx)
	if err != nil {
		return nil, nil, err
	}
	proj, err := getGCPProjectID()
	if err != nil {
		return nil, nil, err
	}
	topic := gcppubsub.OpenTopic(ctx, pubClient, proj, topicID, nil)
	cleanup2 := func() {
		topic.Shutdown(ctx)
		cleanup()
	}
	return topic, cleanup2, nil
}

func getGCPProjectID() (gcp.ProjectID, error) {
	p := os.Getenv("PROJECT_ID")
	if p == "" {
		return gcp.ProjectID(""), errors.New("$PROJECT_ID should be set to the GCP project")
	}
	return gcp.ProjectID(p), nil
}

func openGCPPubClient(ctx context.Context) (*raw.PublisherClient, func(), error) {
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

func gcpTopicName(proj gcp.ProjectID, topicID string) string {
	return fmt.Sprintf("projects/%s/topics/%s", proj, topicID)
}
