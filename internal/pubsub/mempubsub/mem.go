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

// Package mempubsub provides an in-memory pubsub implementation.
// This should not be used for production: it is intended for local
// development and testing.
//
// mempubsub does not support any types for As.
package mempubsub

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/go-cloud/internal/pubsub"
	"github.com/google/go-cloud/internal/pubsub/driver"
)

var errNotExist = errors.New("mempubsub: topic does not exist")

type Broker struct {
	mu     sync.Mutex
	topics map[string]*topic
}

func NewBroker(topicNames []string) *Broker {
	topics := map[string]*topic{}
	for _, n := range topicNames {
		topics[n] = &topic{name: n}
	}
	return &Broker{topics: topics}
}

func (b *Broker) topic(name string) *topic {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.topics[name]
}

type topic struct {
	name      string
	mu        sync.Mutex
	subs      []*subscription
	nextAckID int
}

// OpenTopic establishes a new topic.
// Open subscribers for the topic before publishing.
func OpenTopic(b *Broker, name string) *pubsub.Topic {
	return pubsub.NewTopic(b.topic(name))
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

type subscription struct {
	mu          sync.Mutex
	topic       *topic
	ackDeadline time.Duration
	msgs        map[driver.AckID]*message // all unacknowledged messages
}

// OpenSubscription creates a new subscription for the given topic.

func OpenSubscription(b *Broker, topicName string, ackDeadline time.Duration) *pubsub.Subscription {
	b.mu.Lock()
	t := b.topics[topicName]
	b.mu.Unlock()
	return pubsub.NewSubscription(newSubscription(t, ackDeadline))
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
