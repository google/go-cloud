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

// Package gcppubsub provides a pubsub implementation that uses GCP
// PubSub. Use OpenTopic to construct a *pubsub.Topic, and/or OpenSubscription
// to construct a *pubsub.Subscription.
//
// As
//
// gcppubsub exposes the following types for As:
//  - Topic: *raw.PublisherClient
//  - Subscription: *raw.SubscriberClient
//  - Message: *pb.PubsubMessage
//  - Error: *google.golang.org/grpc/status.Status
package gcppubsub // import "gocloud.dev/pubsub/gcppubsub"

import (
	"context"
	"fmt"

	raw "cloud.google.com/go/pubsub/apiv1"
	"gocloud.dev/gcerrors"
	"gocloud.dev/gcp"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/internal/useragent"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
	"google.golang.org/api/option"
	pb "google.golang.org/genproto/googleapis/pubsub/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
	"google.golang.org/grpc/status"
)

const endPoint = "pubsub.googleapis.com:443"

type topic struct {
	path   string
	client *raw.PublisherClient
}

// Dial opens a gRPC connection to the GCP Pub Sub API.
//
// The second return value is a function that can be called to clean up
// the connection opened by Dial.
func Dial(ctx context.Context, ts gcp.TokenSource) (*grpc.ClientConn, func(), error) {
	conn, err := grpc.DialContext(ctx, endPoint,
		grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")),
		grpc.WithPerRPCCredentials(oauth.TokenSource{TokenSource: ts}),
		useragent.GRPCDialOption("pubsub"),
	)
	if err != nil {
		return nil, nil, err
	}
	return conn, func() { conn.Close() }, nil
}

// PublisherClient returns a *raw.PublisherClient that can be used in OpenTopic.
func PublisherClient(ctx context.Context, conn *grpc.ClientConn) (*raw.PublisherClient, error) {
	return raw.NewPublisherClient(ctx, option.WithGRPCConn(conn))
}

// SubscriberClient returns a *raw.SubscriberClient that can be used in OpenSubscription.
func SubscriberClient(ctx context.Context, conn *grpc.ClientConn) (*raw.SubscriberClient, error) {
	return raw.NewSubscriberClient(ctx, option.WithGRPCConn(conn))
}

// TopicOptions will contain configuration for topics.
type TopicOptions struct{}

// OpenTopic returns a *pubsub.Topic backed by an existing GCP PubSub topic
// topicName in the given projectID. See the package documentation for an
// example.
func OpenTopic(client *raw.PublisherClient, proj gcp.ProjectID, topicName string, opts *TopicOptions) *pubsub.Topic {
	dt := openTopic(client, proj, topicName)
	return pubsub.NewTopic(dt, nil)
}

// openTopic returns the driver for OpenTopic. This function exists so the test
// harness can get the driver interface implementation if it needs to.
func openTopic(client *raw.PublisherClient, proj gcp.ProjectID, topicName string) driver.Topic {
	path := fmt.Sprintf("projects/%s/topics/%s", proj, topicName)
	return &topic{path, client}
}

// SendBatch implements driver.Topic.SendBatch.
func (t *topic) SendBatch(ctx context.Context, dms []*driver.Message) error {
	// The PubSub service limits the number of messages in a single Publish RPC.
	const maxPublishCount = 1000
	for len(dms) > 0 {
		n := len(dms)
		if n > maxPublishCount {
			n = maxPublishCount
		}
		batch := dms[:n]
		dms = dms[n:]
		var ms []*pb.PubsubMessage
		for _, dm := range batch {
			ms = append(ms, &pb.PubsubMessage{
				Data:       dm.Body,
				Attributes: dm.Metadata,
			})
		}
		req := &pb.PublishRequest{
			Topic:    t.path,
			Messages: ms,
		}
		if _, err := t.client.Publish(ctx, req); err != nil {
			return err
		}
	}
	return nil
}

// IsRetryable implements driver.Topic.IsRetryable.
func (t *topic) IsRetryable(error) bool {
	// The client handles retries.
	return false
}

// As implements driver.Topic.As.
func (t *topic) As(i interface{}) bool {
	c, ok := i.(**raw.PublisherClient)
	if !ok {
		return false
	}
	*c = t.client
	return true
}

// ErrorAs implements driver.Topic.ErrorAs
func (*topic) ErrorAs(err error, i interface{}) bool {
	return errorAs(err, i)
}

func errorAs(err error, i interface{}) bool {
	s, ok := status.FromError(err)
	if !ok {
		return false
	}
	p, ok := i.(**status.Status)
	if !ok {
		return false
	}
	*p = s
	return true
}

func (*topic) ErrorCode(err error) gcerrors.ErrorCode {
	return gcerr.GRPCCode(err)
}

type subscription struct {
	client *raw.SubscriberClient
	path   string
}

// SubscriptionOptions will contain configuration for subscriptions.
type SubscriptionOptions struct{}

// OpenSubscription returns a *pubsub.Subscription backed by an existing GCP
// PubSub subscription subscriptionName in the given projectID. See the package
// documentation for an example.
func OpenSubscription(client *raw.SubscriberClient, proj gcp.ProjectID, subscriptionName string, opts *SubscriptionOptions) *pubsub.Subscription {
	ds := openSubscription(client, proj, subscriptionName)
	return pubsub.NewSubscription(ds, nil)
}

// openSubscription returns a driver.Subscription.
func openSubscription(client *raw.SubscriberClient, projectID gcp.ProjectID, subscriptionName string) driver.Subscription {
	path := fmt.Sprintf("projects/%s/subscriptions/%s", projectID, subscriptionName)
	return &subscription{client, path}
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
			AsFunc:   messageAsFunc(rmm),
		}
		ms = append(ms, m)
	}
	return ms, nil
}

func messageAsFunc(pm *pb.PubsubMessage) func(interface{}) bool {
	return func(i interface{}) bool {
		p, ok := i.(**pb.PubsubMessage)
		if !ok {
			return false
		}
		*p = pm
		return true
	}
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

// As implements driver.Subscription.As.
func (s *subscription) As(i interface{}) bool {
	c, ok := i.(**raw.SubscriberClient)
	if !ok {
		return false
	}
	*c = s.client
	return true
}

// ErrorAs implements driver.Subscription.ErrorAs
func (*subscription) ErrorAs(err error, i interface{}) bool {
	return errorAs(err, i)
}

func (*subscription) ErrorCode(err error) gcerrors.ErrorCode {
	return gcerr.GRPCCode(err)
}

// AckFunc implements driver.Subscription.AckFunc.
func (*subscription) AckFunc() func() { return nil }
