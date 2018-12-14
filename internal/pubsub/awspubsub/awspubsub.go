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

// Package awspubsub provides an implementation of pubsub that uses AWS
// SNS (Simple Notification Service) and SQS (Simple Queueing Service).
//
// It exposes the following types for As:
// Topic: *sns.SNS
// Subscription: *sqs.SQS
package awspubsub

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sqs"
	"gocloud.dev/internal/pubsub"
	"gocloud.dev/internal/pubsub/driver"
)

type topic struct {
	mu     sync.Mutex
	client *sns.SNS
	arn    string
}

// TopicOptions will contain configuration for topics.
type TopicOptions struct{}

// OpenTopic opens the topic on AWS SNS for the given SNS client and topic ARN.
func OpenTopic(ctx context.Context, client *sns.SNS, topicARN string, opts *TopicOptions) *pubsub.Topic {
	dt := openTopic(ctx, client, topicARN)
	return pubsub.NewTopic(dt)
}

// openTopic returns the driver for OpenTopic. This function exists so the test
// harness can get the driver interface implementation if it needs to.
func openTopic(ctx context.Context, client *sns.SNS, topicARN string) driver.Topic {
	return &topic{client: client, arn: topicARN}
}

// SendBatch implements driver.Topic.SendBatch.
func (t *topic) SendBatch(ctx context.Context, dms []*driver.Message) error {
	for _, dm := range dms {
		attrs := map[string]*sns.MessageAttributeValue{}
		for k, v := range dm.Metadata {
			attrs[k] = &sns.MessageAttributeValue{
				DataType:    aws.String("String"),
				StringValue: aws.String(v),
			}
		}
		params := sns.PublishInput{
			Message:           aws.String(string(dm.Body)),
			MessageAttributes: attrs,
			TopicArn:          &t.arn,
		}
		if err := t.send(&params); err != nil {
			return err
		}
	}
	return nil
}

func (t *topic) send(params *sns.PublishInput) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	req, _ := t.client.PublishRequest(params)
	return req.Send()
}

// IsRetryable implements driver.Topic.IsRetryable.
func (t *topic) IsRetryable(error) bool {
	// The client handles retries.
	return false
}

// As implements driver.Topic.As.
func (t *topic) As(i interface{}) bool {
	c, ok := i.(**sns.SNS)
	if !ok {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	*c = t.client
	return true
}

type subscription struct {
	mu     sync.Mutex
	client *sqs.SQS
	qURL   string
}

// SubscriptionOptions will contain configuration for subscriptions.
type SubscriptionOptions struct{}

func OpenSubscription(ctx context.Context, client *sqs.SQS, qURL string, opts *SubscriptionOptions) *pubsub.Subscription {
	ds := openSubscription(ctx, client, qURL)
	return pubsub.NewSubscription(ds)
}

// openSubscription returns a driver.Subscription.
func openSubscription(ctx context.Context, client *sqs.SQS, qURL string) driver.Subscription {
	return &subscription{client: client, qURL: qURL}
}

// ReceiveBatch implements driver.Subscription.ReceiveBatch.
func (s *subscription) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	output, err := s.receiveMessages()
	if err != nil {
		return nil, err
	}
	var ms []*driver.Message
	for _, m := range output.Messages {
		type Attribute struct {
			Value string
		}
		type MsgBody struct {
			Message           string
			MessageAttributes map[string]Attribute
		}
		var body MsgBody
		if err := json.Unmarshal([]byte(*m.Body), &body); err != nil {
			return nil, err
		}
		attrs := map[string]string{}
		for k, v := range body.MessageAttributes {
			attrs[k] = v.Value
		}
		m := &driver.Message{
			Body:     []byte(body.Message),
			Metadata: attrs,
			AckID:    *m.ReceiptHandle,
		}
		ms = append(ms, m)
	}
	return ms, nil
}

func (s *subscription) receiveMessages() (*sqs.ReceiveMessageOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	params := sqs.ReceiveMessageInput{QueueUrl: &s.qURL}
	req, output := s.client.ReceiveMessageRequest(&params)
	err := req.Send()
	return output, err
}

// SendAcks implements driver.Subscription.SendAcks.
func (s *subscription) SendAcks(ctx context.Context, ids []driver.AckID) error {
	for _, id := range ids {
		rh := id.(string)
		err := s.deleteMessage(rh)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *subscription) deleteMessage(receiptHandle string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.client.DeleteMessage(&sqs.DeleteMessageInput{
		QueueUrl:      &s.qURL,
		ReceiptHandle: &receiptHandle,
	})
	return err
}

// IsRetryable implements driver.Subscription.IsRetryable.
func (s *subscription) IsRetryable(error) bool {
	// The client handles retries.
	return false
}

// As implements driver.Subscription.As.
func (s *subscription) As(i interface{}) bool {
	c, ok := i.(**sqs.SQS)
	if !ok {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	*c = s.client
	return true
}
