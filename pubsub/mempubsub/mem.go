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

// Package mempubsub provides an in-memory pubsub implementation.
// Use NewTopic to construct a *pubsub.Topic, and/or NewSubscription
// to construct a *pubsub.Subscription.
//
// mempubsub should not be used for production: it is intended for local
// development and testing.
//
// # URLs
//
// For pubsub.OpenTopic and pubsub.OpenSubscription, mempubsub registers
// for the scheme "mem".
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// # Message Delivery Semantics
//
// mempubsub supports at-least-once semantics; applications must
// call Message.Ack after processing a message, or it will be redelivered.
// See https://godoc.org/gocloud.dev/pubsub#hdr-At_most_once_and_At_least_once_Delivery
// for more background.
//
// # As
//
// mempubsub does not support any types for As.
package mempubsub // import "gocloud.dev/pubsub/mempubsub"

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"path"
	"sync"
	"time"

	"gocloud.dev/gcerrors"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/batcher"
	"gocloud.dev/pubsub/driver"
)

func init() {
	o := new(URLOpener)
	pubsub.DefaultURLMux().RegisterTopic(Scheme, o)
	pubsub.DefaultURLMux().RegisterSubscription(Scheme, o)
}

// Scheme is the URL scheme mempubsub registers its URLOpeners under on pubsub.DefaultMux.
const Scheme = "mem"

// URLOpener opens mempubsub URLs like "mem://topic".
//
// The URL's host+path is used as the topic to create or subscribe to.
//
// Query parameters:
//   - ackdeadline: The ack deadline for OpenSubscription, in time.ParseDuration formats.
//     Defaults to 1m.
type URLOpener struct {
	mu     sync.Mutex
	topics map[string]*pubsub.Topic
}

// OpenTopicURL opens a pubsub.Topic based on u.
func (o *URLOpener) OpenTopicURL(ctx context.Context, u *url.URL) (*pubsub.Topic, error) {
	for param := range u.Query() {
		return nil, fmt.Errorf("open topic %v: invalid query parameter %q", u, param)
	}
	topicName := path.Join(u.Host, u.Path)
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.topics == nil {
		o.topics = map[string]*pubsub.Topic{}
	}
	t := o.topics[topicName]
	if t == nil {
		t = NewTopic()
		o.topics[topicName] = t
	}
	return t, nil
}

// OpenSubscriptionURL opens a pubsub.Subscription based on u.
func (o *URLOpener) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*pubsub.Subscription, error) {
	q := u.Query()

	ackDeadline := 1 * time.Minute
	if s := q.Get("ackdeadline"); s != "" {
		var err error
		ackDeadline, err = time.ParseDuration(s)
		if err != nil {
			return nil, fmt.Errorf("open subscription %v: invalid ackdeadline %q: %v", u, s, err)
		}
		q.Del("ackdeadline")
	}
	for param := range q {
		return nil, fmt.Errorf("open subscription %v: invalid query parameter %q", u, param)
	}
	topicName := path.Join(u.Host, u.Path)
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.topics == nil {
		o.topics = map[string]*pubsub.Topic{}
	}
	t := o.topics[topicName]
	if t == nil {
		return nil, fmt.Errorf("open subscription %v: no topic %q has been created", u, topicName)
	}
	return NewSubscription(t, ackDeadline), nil
}

var errNotExist = errors.New("mempubsub: topic does not exist")

type topic struct {
	mu        sync.Mutex
	subs      []*subscription
	nextAckID int
}

// TopicOptions contains configuration options for topics.
type TopicOptions struct {
	// BatcherOptions adds constraints to the default batching done for sends.
	BatcherOptions batcher.Options
}

// NewTopic creates a new in-memory topic.
func NewTopic() *pubsub.Topic {
	return NewTopicWithOptions(nil)
}

// NewTopicWithOptions is similar to NewTopic, but supports TopicOptions.
func NewTopicWithOptions(opts *TopicOptions) *pubsub.Topic {
	if opts == nil {
		opts = &TopicOptions{}
	}
	return pubsub.NewTopic(&topic{}, &opts.BatcherOptions)
}

// SendBatch implements driver.Topic.SendBatch.
// It is error if the topic is closed or has no subscriptions.
func (t *topic) SendBatch(ctx context.Context, ms []*driver.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if t == nil {
		return errNotExist
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	// Log a warning if there are no subscribers.
	if len(t.subs) == 0 {
		log.Print("warning: message sent to topic with no subscribers")
	}

	// Associate ack IDs with messages here. It would be a bit better if each subscription's
	// messages had their own ack IDs, so we could catch one subscription using ack IDs from another,
	// but that would require copying all the messages.
	for i, m := range ms {
		m.AckID = t.nextAckID + i
		m.LoggableID = fmt.Sprintf("msg #%d", m.AckID)
		m.AsFunc = func(interface{}) bool { return false }

		if m.BeforeSend != nil {
			if err := m.BeforeSend(func(interface{}) bool { return false }); err != nil {
				return err
			}
		}
		if m.AfterSend != nil {
			if err := m.AfterSend(func(interface{}) bool { return false }); err != nil {
				return err
			}
		}
	}
	t.nextAckID += len(ms)
	for _, s := range t.subs {
		s.add(ms)
	}
	return nil
}

// IsRetryable implements driver.Topic.IsRetryable.
func (*topic) IsRetryable(error) bool { return false }

// As implements driver.Topic.As.
// It supports *topic so that NewSubscription can recover a *topic
// from the portable type (see below). External users won't be able
// to use As because topic isn't exported.
func (t *topic) As(i interface{}) bool {
	x, ok := i.(**topic)
	if !ok {
		return false
	}
	*x = t
	return true
}

// ErrorAs implements driver.Topic.ErrorAs
func (*topic) ErrorAs(error, interface{}) bool {
	return false
}

// ErrorCode implements driver.Topic.ErrorCode
func (*topic) ErrorCode(err error) gcerrors.ErrorCode {
	if err == errNotExist {
		return gcerrors.NotFound
	}
	return gcerrors.Unknown
}

// Close implements driver.Topic.Close.
func (*topic) Close() error { return nil }

// SubscriptionOptions will contain configuration for subscriptions.
type SubscriptionOptions struct {
	// ReceiveBatcherOptions adds constraints to the default batching done for receives.
	ReceiveBatcherOptions batcher.Options

	// AckBatcherOptions adds constraints to the default batching done for acks.
	AckBatcherOptions batcher.Options
}

type subscription struct {
	mu          sync.Mutex
	topic       *topic
	ackDeadline time.Duration
	msgs        map[driver.AckID]*message // all unacknowledged messages
}

// NewSubscription creates a new subscription for the given topic.
// It panics if the given topic did not come from mempubsub.
// If a message is not acked within in the given ack deadline from when
// it is received, then it will be redelivered.
func NewSubscription(pstopic *pubsub.Topic, ackDeadline time.Duration) *pubsub.Subscription {
	return NewSubscriptionWithOptions(pstopic, ackDeadline, nil)
}

// NewSubscriptionWithOptions is similar to NewSubscription, but supports SubscriptionOptions.
func NewSubscriptionWithOptions(pstopic *pubsub.Topic, ackDeadline time.Duration, opts *SubscriptionOptions) *pubsub.Subscription {
	if opts == nil {
		opts = &SubscriptionOptions{}
	}
	var t *topic
	if !pstopic.As(&t) {
		panic("mempubsub: NewSubscription passed a Topic not from mempubsub")
	}
	return pubsub.NewSubscription(newSubscription(t, ackDeadline), &opts.ReceiveBatcherOptions, &opts.AckBatcherOptions)
}

func newSubscription(topic *topic, ackDeadline time.Duration) *subscription {
	s := &subscription{
		topic:       topic,
		ackDeadline: ackDeadline,
		msgs:        map[driver.AckID]*message{},
	}
	if topic != nil {
		topic.mu.Lock()
		defer topic.mu.Unlock()
		topic.subs = append(topic.subs, s)
	}
	return s
}

type message struct {
	msg        *driver.Message
	expiration time.Time
}

func (s *subscription) add(ms []*driver.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range ms {
		// The new message will expire at the zero time, which means it will be
		// immediately eligible for delivery.
		s.msgs[m.AckID] = &message{msg: m}
	}
}

// Collect some messages available for delivery. Since we're iterating over a map,
// the order of the messages won't match the publish order, which mimics the actual
// behavior of most pub/sub services.
func (s *subscription) receiveNoWait(now time.Time, max int) []*driver.Message {
	var msgs []*driver.Message
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.msgs {
		if now.After(m.expiration) {
			msgs = append(msgs, m.msg)
			m.expiration = now.Add(s.ackDeadline)
			if len(msgs) == max {
				return msgs
			}
		}
	}
	return msgs
}

// How long ReceiveBatch should wait if no messages are available, to avoid
// spinning.
const pollDuration = 250 * time.Millisecond

// ReceiveBatch implements driver.ReceiveBatch.
func (s *subscription) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	// Check for closed or cancelled before doing any work.
	if err := s.wait(ctx, 0); err != nil {
		return nil, err
	}
	msgs := s.receiveNoWait(time.Now(), maxMessages)
	if len(msgs) == 0 {
		// When we return no messages and no error, the portable type will call
		// ReceiveBatch again immediately. Sleep for a bit to avoid spinning.
		time.Sleep(pollDuration)
	}
	return msgs, nil
}

func (s *subscription) wait(ctx context.Context, dur time.Duration) error {
	if s.topic == nil {
		return errNotExist
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(dur):
		return nil
	}
}

// SendAcks implements driver.SendAcks.
func (s *subscription) SendAcks(ctx context.Context, ackIDs []driver.AckID) error {
	if s.topic == nil {
		return errNotExist
	}
	// Check for context done before doing any work.
	if err := ctx.Err(); err != nil {
		return err
	}
	// Acknowledge messages by removing them from the map.
	// Since there is a single map, this correctly handles the case where a message
	// is redelivered, but the first receiver acknowledges it.
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ackIDs {
		// It is OK if the message is not in the map; that just means it has been
		// previously acked.
		delete(s.msgs, id)
	}
	return nil
}

// CanNack implements driver.CanNack.
func (s *subscription) CanNack() bool { return true }

// SendNacks implements driver.SendNacks.
func (s *subscription) SendNacks(ctx context.Context, ackIDs []driver.AckID) error {
	if s.topic == nil {
		return errNotExist
	}
	// Check for context done before doing any work.
	if err := ctx.Err(); err != nil {
		return err
	}
	// Nack messages by setting their expiration to the zero time.
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ackIDs {
		if m := s.msgs[id]; m != nil {
			m.expiration = time.Time{}
		}
	}
	return nil
}

// IsRetryable implements driver.Subscription.IsRetryable.
func (*subscription) IsRetryable(error) bool { return false }

// As implements driver.Subscription.As.
func (s *subscription) As(i interface{}) bool { return false }

// ErrorAs implements driver.Subscription.ErrorAs
func (*subscription) ErrorAs(error, interface{}) bool {
	return false
}

// ErrorCode implements driver.Subscription.ErrorCode
func (*subscription) ErrorCode(err error) gcerrors.ErrorCode {
	if err == errNotExist {
		return gcerrors.NotFound
	}
	return gcerrors.Unknown
}

// Close implements driver.Subscription.Close.
func (*subscription) Close() error { return nil }
