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

	"github.com/google/go-cloud/pubsub/driver"
	"google.golang.org/api/support/bundler"
)

// Message contains data to be published.
type Message struct {
	// Body contains the content of the message.
	Body []byte

	// Attributes has key/value metadata for the message.
	Attributes map[string]string

	// AckID identifies the message on the server.
	// It can be used to ack the message after it has been received.
	ackID AckID

	// sub is the Subscription this message was received from.
	sub *Subscription
}

// AckID identifies a message for acknowledgement.
type AckID interface{}

// Ack acknowledges the message, telling the server that it does not need to
// be sent again to the associated Subscription. This method blocks until
// the message has been confirmed as acknowledged on the server, or failure
// occurs.
func (m *Message) Ack(ctx context.Context) error {
	// Send the message back to the subscription for ack batching.
	mec := msgErrChan{
		msg:     m,
		errChan: make(chan error),
	}
	size := 8
	if err := m.sub.ackBatcher.AddWait(ctx, mec, size); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		// Spin up a goroutine to receive the err result from the
		// message errChan. Otherwise the bundler handler could hang
		// while trying to send to it.
		go func() { <-mec.errChan }()
		return ctx.Err()
	case err := <-mec.errChan:
		return err
	}
}

type msgErrChan struct {
	msg     *Message
	errChan chan error
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

	// BatchSize specifies the maximum number of messages that can go in a batch
	// for sending.
	BatchSize int
}

// SendDelayDefault is the value for TopicOptions.SendDelay if it is not set.
const SendDelayDefault = time.Millisecond

// Send publishes a message. It only returns after the message has been
// sent, or failed to be sent. Send can be called from multiple goroutines
// at once.
func (t *Topic) Send(ctx context.Context, m *Message) error {
	mec := msgErrChan{
		msg:     m,
		errChan: make(chan error),
	}
	size := len(m.Body)
	for k, v := range m.Attributes {
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
func NewTopic(ctx context.Context, d driver.Topic, opts TopicOptions) *Topic {
	handler := func(item interface{}) {
		mecs, ok := item.([]msgErrChan)
		if !ok {
			panic("failed conversion to []msgErrChan in bundler handler")
		}
		var dms []*driver.Message
		for _, mec := range mecs {
			m := mec.msg
			dm := &driver.Message{
				Body:       m.Body,
				Attributes: m.Attributes,
				AckID:      m.ackID,
			}
			dms = append(dms, dm)
		}
		err := d.SendBatch(ctx, dms)
		for _, mec := range mecs {
			mec.errChan <- err
		}
	}
	b := bundler.NewBundler(msgErrChan{}, handler)
	if opts.SendDelay == 0 {
		b.DelayThreshold = SendDelayDefault
	} else {
		b.DelayThreshold = opts.SendDelay
	}
	if opts.BatchSize != 0 {
		b.BundleCountThreshold = opts.BatchSize
	}
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

	// q is the local queue of messages downloaded from the server.
	sem chan struct{}
	q   []*Message
}

// SubscriptionOptions contains configuration for Subscriptions.
type SubscriptionOptions struct {
	// AckDelay tells the max duration to wait before sending the next batch
	// of acknowledgements back to the server.
	AckDelay time.Duration

	// AckBatchSize is the maximum number of acks that should be sent to
	// the server in a batch.
	AckBatchSize int

	// AckDeadline tells how long the server should wait before assuming a
	// received message has failed to be processed.
	AckDeadline time.Duration
}

// AckDelayDefault is the value for SubscriptionOptions.AckDelay if it is not set.
const AckDelayDefault = time.Millisecond

// Receive receives and returns the next message from the Subscription's queue,
// blocking and polling if none are available. This method can be called
// concurrently from multiple goroutines. On systems that support acks, the
// Ack() method of the returned Message has to be called once the message has
// been processed, to prevent it from being received again.
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

// getNextBatch gets the next batch of messages from the server.
func (s *Subscription) getNextBatch(ctx context.Context) error {
	msgs, err := s.driver.ReceiveBatch(ctx)
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		if len(msgs) == 0 {
			return errors.New("subscription driver bug: received empty batch")
		}
		s.q = make([]*Message, len(msgs))
		for i, m := range msgs {
			s.q[i] = &Message{
				Body:       m.Body,
				Attributes: m.Attributes,
				ackID:      m.AckID,
				sub:        s,
			}
		}
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
func NewSubscription(ctx context.Context, d driver.Subscription, opts SubscriptionOptions) *Subscription {
	handler := func(item interface{}) {
		mecs, ok := item.([]msgErrChan)
		if !ok {
			panic("failed conversion to []msgErrChan in bundler handler")
		}
		var ids []driver.AckID
		for _, mec := range mecs {
			m := mec.msg
			id := m.ackID
			ids = append(ids, id)
		}
		err := d.SendAcks(ctx, ids)
		for _, mec := range mecs {
			mec.errChan <- err
		}
	}
	ab := bundler.NewBundler(msgErrChan{}, handler)
	if opts.AckDelay == 0 {
		ab.DelayThreshold = AckDelayDefault
	} else {
		ab.DelayThreshold = opts.AckDelay
	}
	if opts.AckBatchSize != 0 {
		ab.BundleCountThreshold = opts.AckBatchSize
	}
	s := &Subscription{
		driver:     d,
		ackBatcher: ab,
		sem:        make(chan struct{}, 1),
	}
	s.sem <- struct{}{}
	return s
}
