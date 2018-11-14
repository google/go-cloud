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
package pubsub

import (
	"context"
	"errors"
	"time"

	"github.com/google/go-cloud/internal/pubsub/driver"
	"google.golang.org/api/support/bundler"
)

// Message contains data to be published.
type Message struct {
	// Body contains the content of the message.
	Body []byte

	// Metadata has key/value metadata for the message.
	Metadata map[string]string

	// AckID identifies the message on the server.
	// It can be used to ack the message after it has been received.
	ackID driver.AckID

	// sub is the Subscription this message was received from.
	sub *Subscription
}

// Ack acknowledges the message, telling the server that it does not need to
// be sent again to the associated Subscription. This method blocks until
// the message has been confirmed as acknowledged on the server, or failure
// occurs.
func (m *Message) Ack() {
	// Send the message back to the subscription for ack batching.
	// size is an estimate of the size of a single AckID in bytes.
	const size = 8
	go m.sub.ackBatcher.Add(m, size)
}

// Topic publishes messages to all its subscribers.
type Topic struct {
	driver  driver.Topic
	batcher *bundler.Bundler
}

// TopicOptions contains configuration for Topics.
type TopicOptions struct {
	// SendDelay tells the max duration to wait before sending the next batch of
	// messages to the server.
	SendDelay time.Duration

	// BatchCountThreshold specifies the maximum number of messages that can go in a batch
	// for sending.
	BatchCountThreshold int
}

// SendDelayDefault is the value for TopicOptions.SendDelay if it is not set.
const SendDelayDefault = time.Millisecond

var DefaultTopicOptions = TopicOptions{
	SendDelay:           time.Millisecond,
	BatchCountThreshold: bundler.DefaultBundleCountThreshold,
}

type msgErrChan struct {
	msg     *Message
	errChan chan error
}

// Send publishes a message. It only returns after the message has been
// sent, or failed to be sent. Send can be called from multiple goroutines
// at once.
func (t *Topic) Send(ctx context.Context, m *Message) error {
	mec := msgErrChan{
		msg:     m,
		errChan: make(chan error),
	}
	size := len(m.Body)
	for k, v := range m.Metadata {
		size += len(k)
		size += len(v)
	}
	if err := t.batcher.AddWait(ctx, mec, size); err != nil {
		return err
	}
	return <-mec.errChan
}

// Close flushes pending message sends and disconnects the Topic.
// It only returns after all pending messages have been sent.
func (t *Topic) Close() error {
	t.batcher.Flush()
	return t.driver.Close()
}

// NewTopic makes a pubsub.Topic from a driver.Topic and opts to
// tune how messages are sent. Behind the scenes, NewTopic spins up a goroutine
// to bundle messages into batches and send them to the server.
// It is for use by provider implementations.
func NewTopic(ctx context.Context, d driver.Topic, opts *TopicOptions) *Topic {
	handler := func(item interface{}) {
		mecs, ok := item.([]msgErrChan)
		if !ok {
			panic("failed conversion to []msgErrChan in bundler handler")
		}
		var dms []*driver.Message
		for _, mec := range mecs {
			m := mec.msg
			dm := &driver.Message{
				Body:     m.Body,
				Metadata: m.Metadata,
				AckID:    m.ackID,
			}
			dms = append(dms, dm)
		}
		err := d.SendBatch(ctx, dms)
		for _, mec := range mecs {
			mec.errChan <- err
		}
	}
	b := bundler.NewBundler(msgErrChan{}, handler)
	if opts == nil {
		opts = &DefaultTopicOptions
	}
	b.DelayThreshold = opts.SendDelay
	b.BundleCountThreshold = opts.BatchCountThreshold
	t := &Topic{
		driver:  d,
		batcher: b,
	}
	return t
}

// Subscription receives published messages.
type Subscription struct {
	driver driver.Subscription

	// ackBatcher makes batches of acks and sends them to the server.
	ackBatcher *bundler.Bundler

	// sem is a semaphore guarding q. It is used instead of a mutex to work
	// with context cancellation.
	sem chan struct{}

	// q is the local queue of messages downloaded from the server.
	q []*Message
}

// SubscriptionOptions contains configuration for Subscriptions.
type SubscriptionOptions struct {
	// AckDelay tells the max duration to wait before sending the next batch
	// of acknowledgements back to the server.
	AckDelay time.Duration

	// AckBatchCountThreshold is the maximum number of acks that should be sent to
	// the server in a batch.
	AckBatchCountThreshold int
}

var DefaultSubscriptionOptions = SubscriptionOptions{
	AckDelay:               time.Millisecond,
	AckBatchCountThreshold: bundler.DefaultBundleCountThreshold,
}

// AckDelayDefault is the value for SubscriptionOptions.AckDelay if it is not set.
const AckDelayDefault = time.Millisecond

// Receive receives and returns the next message from the Subscription's queue,
// blocking and polling if none are available. This method can be called
// concurrently from multiple goroutines. The Ack() method of the returned
// Message has to be called once the message has been processed, to prevent it
// from being received again.
func (s *Subscription) Receive(ctx context.Context) (*Message, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.sem:
	}
	defer func() {
		s.sem <- struct{}{}
	}()
	if len(s.q) == 0 {
		if err := s.getNextBatch(ctx); err != nil {
			return nil, err
		}
	}
	m := s.q[0]
	s.q = s.q[1:]
	return m, nil
}

// getNextBatch gets the next batch of messages from the server and saves it in
// s.q.
func (s *Subscription) getNextBatch(ctx context.Context) error {
	msgs, err := s.driver.ReceiveBatch(ctx)
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return errors.New("subscription driver bug: received empty batch")
	}
	s.q = nil
	for _, m := range msgs {
		m := &Message{
			Body:     m.Body,
			Metadata: m.Metadata,
			ackID:    m.AckID,
			sub:      s,
		}
		s.q = append(s.q, m)
	}
	return nil
}

// Close flushes pending ack sends and disconnects the Subscription.
func (s *Subscription) Close() error {
	s.ackBatcher.Flush()
	return s.driver.Close()
}

// NewSubscription creates a Subscription from a driver.Subscription and opts to
// tune sending and receiving of acks and messages. Behind the scenes,
// NewSubscription spins up a goroutine to gather acks into batches and
// periodically send them to the server.
// It is for use by provider implementations.
func NewSubscription(ctx context.Context, d driver.Subscription, opts *SubscriptionOptions) *Subscription {
	handler := func(item interface{}) {
		ms := item.([]*Message)
		var ids []driver.AckID
		for _, m := range ms {
			id := m.ackID
			ids = append(ids, id)
		}
		// TODO(#695): Do something sensible if SendAcks returns an error.
		_ = d.SendAcks(ctx, ids)
	}
	ab := bundler.NewBundler(&Message{}, handler)
	if opts == nil {
		opts = &DefaultSubscriptionOptions
	}
	ab.DelayThreshold = opts.AckDelay
	ab.BundleCountThreshold = opts.AckBatchCountThreshold
	s := &Subscription{
		driver:     d,
		ackBatcher: ab,
		sem:        make(chan struct{}, 1),
	}
	s.sem <- struct{}{}
	return s
}
