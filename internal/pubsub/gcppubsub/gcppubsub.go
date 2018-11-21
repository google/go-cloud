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

// Package gcppubsub provides an implementation of pubsub that uses GCP
// PubSub.
package gcppubsub

import (
	"context"
	"fmt"

	raw "cloud.google.com/go/pubsub/apiv1"
	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/internal/pubsub"
	"github.com/google/go-cloud/internal/pubsub/driver"
	pb "google.golang.org/genproto/googleapis/pubsub/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

type topic struct {
	path   string
	client *raw.PublisherClient
}

// TopicOptions will contain configuration for topics.
type TopicOptions struct{}

// OpenTopic opens the topic on GCP PubSub for the given projectID and
// topicName. If the topic does not exist then an error will be returned.
func OpenTopic(ctx context.Context, client *raw.PublisherClient, proj gcp.ProjectID, topicName string, opts *TopicOptions) (*pubsub.Topic, error) {
	dt, err := openTopic(ctx, client, proj, topicName)
	if err != nil {
		return nil, fmt.Errorf("gcppubsub: opening topic: %v", err)
	}
	t := pubsub.NewTopic(dt)
	return t, nil
}

// openTopic returns the driver for OpenTopic. This function exists so the test
// harness can get the driver interface implementation if it needs to.
func openTopic(ctx context.Context, client *raw.PublisherClient, proj gcp.ProjectID, topicName string) (driver.Topic, error) {
	client, err := raw.NewPublisherClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("gcppubsub: creating publisher client: %v", err)
	}
	ok, err := topicExists(ctx, client, topicName)
	if err != nil {
		return nil, fmt.Errorf("gcppubsub: checking existence of topic: %v", err)
	}
	if !ok {
		return nil, fmt.Errorf("gcppubsub: topic named %q does not exist", topicName)
	}
	path := fmt.Sprintf("projects/%s/topics/%s", proj, topicName)
	return &topic{path, client}, nil
}

func topicExists(ctx context.Context, client *raw.PublisherClient, topicName string) (bool, error) {
	if topicName == "_deleted-topic_" {
		return false, nil
	}
	_, err := client.GetTopic(ctx, &pb.GetTopicRequest{Topic: topicName})
	if err == nil {
		return true, nil
	}
	if grpc.Code(err) == codes.NotFound {
		return false, nil
	}
	return false, err
}

// SendBatch implements driver.Topic.SendBatch.
func (t *topic) SendBatch(ctx context.Context, dms []*driver.Message) error {
	var ms []*pb.PubsubMessage
	for _, dm := range dms {
		ms = append(ms, &pb.PubsubMessage{
			Data:       dm.Body,
			Attributes: dm.Metadata,
		})
	}
	req := &pb.PublishRequest{
		Topic:    t.path,
		Messages: ms,
	}
	_, err := t.client.Publish(ctx, req)
	return err
}

// IsRetryable implements driver.Topic.IsRetryable.
func (t *topic) IsRetryable(error) bool {
	// The client handles retries.
	return false
}

type subscription struct {
	client *raw.SubscriberClient
	path   string
}

// SubscriptionOptions will contain configuration for subscriptions.
type SubscriptionOptions struct{}

// OpenSubscription opens the subscription on GCP PubSub for the given
// projectID and subscriptionName. If the subscription does not exist then an
// error is returned.
func OpenSubscription(ctx context.Context, client *raw.SubscriberClient, proj gcp.ProjectID, subscriptionName string, opts *SubscriptionOptions) (*pubsub.Subscription, error) {
	ds, err := openSubscription(ctx, client, proj, subscriptionName)
	if err != nil {
		return nil, fmt.Errorf("gcppubsub: opening subscription: %v", err)
	}
	s := pubsub.NewSubscription(ds)
	return s, nil
}

func openSubscription(ctx context.Context, client *raw.SubscriberClient, projectID gcp.ProjectID, subscriptionName string) (driver.Subscription, error) {
	client, err := raw.NewSubscriberClient(ctx)
	ok, err := subscriptionExists(ctx, client, subscriptionName)
	if err != nil {
		return nil, fmt.Errorf("gcppubsub: checking for existence of subscription: %v", err)
	}
	if !ok {
		return nil, fmt.Errorf("gcppubsub: subscription named %q does not exist", subscriptionName)
	}
	path := fmt.Sprintf("projects/%s/subscriptions/%s", projectID, subscriptionName)
	return &subscription{client, path}, nil
}

func subscriptionExists(ctx context.Context, client *raw.SubscriberClient, subscriptionName string) (bool, error) {
	_, err := client.GetSubscription(ctx, &pb.GetSubscriptionRequest{Subscription: subscriptionName})
	if err == nil {
		return true, nil
	}
	if grpc.Code(err) == codes.NotFound {
		return false, nil
	}
	return false, err
}

// ReceiveBatch implements driver.Subscription.ReceiveBatch.
func (s *subscription) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	req := &pb.PullRequest{
		Subscription:      s.path,
		ReturnImmediately: false,
		MaxMessages:       int32(maxMessages),
	}
	resp, err := s.client.Pull(ctx, req)
	if err != nil {
		return nil, err
	}
	var ms []*driver.Message
	for _, rm := range resp.ReceivedMessages {
		rmm := rm.Message
		m := &driver.Message{
			Body:     rmm.Data,
			Metadata: rmm.Attributes,
			AckID:    rm.AckId,
		}
		ms = append(ms, m)
	}
	return ms, nil
}

// SendAcks implements driver.Subscription.SendAcks.
func (s *subscription) SendAcks(ctx context.Context, ids []driver.AckID) error {
	var ids2 []string
	for _, id := range ids {
		ids2 = append(ids2, id.(string))
	}
	return s.client.Acknowledge(ctx, &pb.AcknowledgeRequest{
		Subscription: s.path,
		AckIds:       ids2,
	})
}

// IsRetryable implements driver.Subscription.IsRetryable.
func (s *subscription) IsRetryable(error) bool {
	// The client handles retries.
	return false
}
