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
// URLs
//
// For pubsub.OpenTopic/Subscription URLs, mempubsub registers for the scheme "mem".
// See URLOpener for more details.
//
// As
//
// mempubsub does not support any types for As.
package mempubsub // import "gocloud.dev/pubsub/mempubsub"

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"sync"
	"time"

	"gocloud.dev/gcerrors"
	"gocloud.dev/pubsub"
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
// The URL's host+path is used as the topic to create or subscribe to.
// Query parameters:
//   - ackdeadline: The ack deadline for OpenSubscription, in time.ParseDuration formats.
//       Defaults to 1m.
type URLOpener struct {
	mu     sync.Mutex
	topics map[string]*pubsub.Topic
}

// OpenTopicURL opens the GCP Pub/Sub topic with the same name as the URL's host.
func (o *URLOpener) OpenTopicURL(ctx context.Context, u *url.URL) (*pubsub.Topic, error) {
	for param := range u.Query() {
		return nil, fmt.Errorf("open topic %q: invalid query parameter %q", u, param)
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

// OpenSubscriptionURL opens the GCS bucket with the same name as the URL's host.
func (o *URLOpener) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*pubsub.Subscription, error) {
	q := u.Query()

	ackDeadline := 1 * time.Minute
	if s := q.Get("ackdeadline"); s != "" {
		var err error
		ackDeadline, err = time.ParseDuration(s)
		if err != nil {
			return nil, fmt.Errorf("open subscription %q: invalid ackdeadline: %v", u, err)
		}
		q.Del("ackdeadline")
	}
	for param := range q {
		return nil, fmt.Errorf("open subscription %q: invalid query parameter %q", u, param)
	}
	topicName := path.Join(u.Host, u.Path)
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.topics == nil {
		o.topics = map[string]*pubsub.Topic{}
	}
	t := o.topics[topicName]
	if t == nil {
		return nil, fmt.Errorf("open subscription %q: no topic %q has been created", u, topicName)
	}
	return NewSubscription(t, ackDeadline), nil
}

var errNotExist = errors.New("mempubsub: topic does not exist")

type topic struct {
	mu        sync.Mutex
	subs      []*subscription
	nextAckID int
}

// NewTopic creates a new in-memory topic.
func NewTopic() *pubsub.Topic {
	return pubsub.NewTopic(&topic{}, nil)
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
	// Associate ack IDs with messages here. It would be a bit better if each subscription's
	// messages had their own ack IDs, so we could catch one subscription using ack IDs from another,
	// but that would require copying all the messages.
	for i, m := range ms {
		m.AckID = t.nextAckID + i
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
func (*topic) ErrorCode(error) gcerrors.ErrorCode { return gcerrors.Unknown }

type subscription struct {
	mu          sync.Mutex
	topic       *topic
	ackDeadline time.Duration
	msgs        map[driver.AckID]*message // all unacknowledged messages
}

// NewSubscription creates a new subscription for the given topic.
// It panics if the given topic did not come from mempubsub.
func NewSubscription(top *pubsub.Topic, ackDeadline time.Duration) *pubsub.Subscription {
	var t *topic
	if !top.As(&t) {
		panic("mempubsub: NewSubscription passed a Topic not from mempubsub")
	}
	return pubsub.NewSubscription(newSubscription(t, ackDeadline), nil)
}

func newSubscription(t *topic, ackDeadline time.Duration) *subscription {
	s := &subscription{
		topic:       t,
		ackDeadline: ackDeadline,
		msgs:        map[driver.AckID]*message{},
	}
	if t != nil {
		t.mu.Lock()
		defer t.mu.Unlock()
		t.subs = append(t.subs, s)
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
		m.AsFunc = func(interface{}) bool { return false }
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

const (
	// How often ReceiveBatch should poll.
	pollDuration = 250 * time.Millisecond
)

// ReceiveBatch implements driver.ReceiveBatch.
func (s *subscription) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	// Check for closed or cancelled before doing any work.
	if err := s.wait(ctx, 0); err != nil {
		return nil, err
	}
	// Loop until at least one message is available. Polling is inelegant, but the
	// alternative would be complicated by the need to recognize expired messages
	// promptly.
	for {
		if msgs := s.receiveNoWait(time.Now(), maxMessages); len(msgs) > 0 {
			return msgs, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollDuration):
		}
	}
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

// IsRetryable implements driver.Subscription.IsRetryable.
func (*subscription) IsRetryable(error) bool { return false }

// As implements driver.Subscription.As.
func (s *subscription) As(i interface{}) bool { return false }

// ErrorAs implements driver.Subscription.ErrorAs
func (*subscription) ErrorAs(error, interface{}) bool {
	return false
}

// ErrorCode implements driver.Subscription.ErrorCode
func (*subscription) ErrorCode(error) gcerrors.ErrorCode { return gcerrors.Unknown }

// AckFunc implements driver.Subscription.AckFunc.
func (*subscription) AckFunc() func() { return nil }
