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
	path   string
	client *rawgcppubsub.PublisherClient
}

// Close closes the connection to the API service. The user should invoke this
// when the client is no longer required.
func (t *topic) Close() error {
	return t.client.Close()
}

// SendBatch publishes all the messages in dms. This method returns only after
// all the messages are sent, an error occurs, or the context is cancelled.
func (t *topic) SendBatch(ctx context.Context, dms []*driver.Message) error {
	var ms []*pubsubpb.PubsubMessage
	for _, dm := range dms {
		m := &pubsubpb.PubsubMessage{
			Data:       dm.Body,
			Attributes: dm.Metadata,
		}
		ms = append(ms, m)
	}
	req := &pubsubpb.PublishRequest{
		Topic:    t.path,
		Messages: ms,
	}
	_, err := t.client.Publish(ctx, req)
	return err
}

type subscription struct {
	client *rawgcppubsub.SubscriberClient
	path   string
}

// ReceiveBatch returns a batch of messages that have queued up for the
// subscription on the server. If no messages are available yet, it blocks
// until there is at least one, or the context is done.
func (s *subscription) ReceiveBatch(ctx context.Context) ([]*driver.Message, error) {
	req := &pubsubpb.PullRequest{
		Subscription:      s.path,
		ReturnImmediately: false,
	}
	resp, err := s.client.Pull(ctx, req)
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
	return ms, err
}

// SendAcks acknowledges the messages with the given ackIDs on the server so
// that they will not be received again for this subscription if the server
// gets the acks before their deadlines. This method returns only after all
// the ackIDs are sent, an error occurs, or the context is cancelled.
func (s *subscription) SendAcks(ctx context.Context, ids []driver.AckID) error {
	var ids2 []string
	for _, id := range ids {
		id2, ok := id.(string)
		if !ok {
			return fmt.Errorf("gcppubsub driver bug: cast from driver.AckID to string failed on %v", id)
		}
		ids2 = append(ids2, id2)
	}
	req := &pubsubpb.AcknowledgeRequest{
		Subscription: s.path,
		AckIds:       ids2,
	}
	return s.client.Acknowledge(ctx, req)
}

// Close closes the connection to the API service. The user should invoke this
// when the client is no longer required.
func (s *subscription) Close() error {
	return s.client.Close()
}

// OpenTopic opens the topic on GCP PubSub for the given projectID and
// topicName. If the topic does not exist then failure will occur when messages
// are sent to it.
func OpenTopic(ctx context.Context, client *rawgcppubsub.PublisherClient, projectID, topicName string) *pubsub.Topic {
	path := fmt.Sprintf("projects/%s/topics/%s", projectID, topicName)
	dt := &topic{path, client}
	t := pubsub.NewTopic(ctx, dt)
	return t
}

// OpenSubscription opens the subscription on GCP PubSub for the given
// projectID and subscriptionName. If the subscription does not exist then
// failure will occur when an attempt is made to receive messages from it.
func OpenSubscription(ctx context.Context, client *rawgcppubsub.SubscriberClient, projectID, subscriptionName string) *pubsub.Subscription {
	path := fmt.Sprintf("projects/%s/subscriptions/%s", projectID, subscriptionName)
	ds := &subscription{client, path}
	s := pubsub.NewSubscription(ctx, ds)
	return s
}
