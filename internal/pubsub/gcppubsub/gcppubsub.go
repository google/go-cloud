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

	rawgcppubsub "cloud.google.com/go/pubsub/apiv1"
	"github.com/google/go-cloud/internal/pubsub"
	"github.com/google/go-cloud/internal/pubsub/driver"
	pubsubpb "google.golang.org/genproto/googleapis/pubsub/v1"
)

type topic struct {
	raw *pubsubpb.Topic
}

func (t *topic) Close() error {
	return nil
}

func (t *topic) SendBatch(context.Context, []*driver.Message) error {
	return nil
}

type subscription struct {
	raw *pubsubpb.Subscription
}

func (s *subscription) ReceiveBatch(ctx context.Context) ([]*driver.Message, error) {
	return nil, nil
}

func (s *subscription) SendAcks(ctx context.Context, acks []driver.AckID) error {
	return nil
}

func (s *subscription) Close() error {
	return nil
}

func OpenTopic(ctx context.Context, client *rawgcppubsub.PublisherClient, projectID, topicName string) (*pubsub.Topic, error) {
	path := fmt.Sprintf("projects/%s/topics/%s", projectID, topicName)
	req := pubsubpb.GetTopicRequest{Topic: path}
	rt, err := client.GetTopic(ctx, &req)
	if err != nil {
		return nil, fmt.Errorf("getting topic named %q: %v", topicName, err)
	}
	dt := &topic{raw: rt}
	t := pubsub.NewTopic(ctx, dt)
	return t, nil
}

func OpenSubscription(ctx context.Context, client *rawgcppubsub.SubscriberClient, projectID, subscriptionName string) (*pubsub.Subscription, error) {
	req := pubsubpb.GetSubscriptionRequest{Subscription: fmt.Sprintf("projects/%s/subscriptions/%s", projectID, subscriptionName)}
	rs, err := client.GetSubscription(ctx, &req)
	if err != nil {
		return nil, fmt.Errorf("getting subscription named %q: %v", subscriptionName, err)
	}
	ds := &subscription{raw: rs}
	s := pubsub.NewSubscription(ctx, ds)
	return s, nil
}
