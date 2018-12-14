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
	pubsubpb "google.golang.org/genproto/googleapis/pubsub/v1"
	"google.golang.org/grpc"
	grpccreds "google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
)

// makeGCPSubscription creates a subscription to the given topic.
func makeGCPSubscription(ctx context.Context, topicID, subID string) error {
	subClient, cleanup, err := openGCPSubscriberClient(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	proj, err := getGCPProjectID()
	if err != nil {
		return err
	}
	_, err = subClient.CreateSubscription(ctx, &pubsubpb.Subscription{
		Topic: fmt.Sprintf("projects/%s/topics/%s", proj, topicID),
		Name:  gcpSubscriptionName(proj, subID),
	})
	return err
}

// openGCPSubscription returns the GCP topic based on the subscription ID.
func openGCPSubscription(ctx context.Context, subID string) (*pubsub.Subscription, func(), error) {
	subClient, cleanup, err := openGCPSubscriberClient(ctx)
	if err != nil {
		return nil, nil, err
	}
	proj, err := getGCPProjectID()
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

// deleteGCPSubscription deletes the given GCP topic.
func deleteGCPSubscription(ctx context.Context, subID string) error {
	subClient, cleanup, err := openGCPSubscriberClient(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	proj, err := getGCPProjectID()
	if err != nil {
		return err
	}
	return subClient.DeleteSubscription(ctx, &pubsubpb.DeleteSubscriptionRequest{
		Subscription: gcpSubscriptionName(proj, subID),
	})
}

func getGCPProjectID() (gcp.ProjectID, error) {
	p := os.Getenv("PROJECT_ID")
	if p == "" {
		return gcp.ProjectID(""), errors.New("$PROJECT_ID should be set to the GCP project")
	}
	return gcp.ProjectID(p), nil
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
