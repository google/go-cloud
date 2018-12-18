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

// Package pubsub provides an easy and portable way to interact with publish/
// subscribe systems.
package pubsub // import "gocloud.dev/internal/pubsub"

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"

	gax "github.com/googleapis/gax-go"
	"gocloud.dev/internal/batcher"
	"gocloud.dev/internal/pubsub/driver"
	"gocloud.dev/internal/retry"
)

// Message contains data to be published.
type Message struct {
	// Body contains the content of the message.
	Body []byte

	// Metadata has key/value metadata for the message.
	Metadata map[string]string

	// ack is a closure that queues this message for acknowledgement.
	ack func()

	// mu guards isAcked in case Ack() is called concurrently.
	mu sync.Mutex

	// isAcked tells whether this message has already had its Ack method
	// called.
	isAcked bool
}

func (m *Message) String() string {
	return fmt.Sprintf("{%q %v}", m.Body, m.Metadata)
}

// Ack acknowledges the message, telling the server that it does not need to be
// sent again to the associated Subscription. It returns immediately, but the
// actual ack is sent in the background, and is not guaranteed to succeed.
func (m *Message) Ack() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.isAcked {
		panic(fmt.Sprintf("Ack() called twice on message: %+v", m))
	}
	m.ack()
	m.isAcked = true
}

// Topic publishes messages to all its subscribers.
type Topic struct {
	driver  driver.Topic
	batcher driver.Batcher
	mu      sync.Mutex
	err     error

	// cancel cancels all SendBatch calls.
	cancel func()
}

type msgErrChan struct {
	msg     *Message
	errChan chan error
}

// Send publishes a message. It only returns after the message has been
// sent, or failed to be sent. Send can be called from multiple goroutines
// at once.
func (t *Topic) Send(ctx context.Context, m *Message) error {
	// Check for doneness before we do any work.
	if err := ctx.Err(); err != nil {
		return err
	}
	t.mu.Lock()
	err := t.err
	t.mu.Unlock()
	if err != nil {
		return err
	}
	return t.batcher.Add(ctx, m)
}

// Shutdown flushes pending message sends and disconnects the Topic.
// It only returns after all pending messages have been sent.
func (t *Topic) Shutdown(ctx context.Context) error {
	t.mu.Lock()
	t.err = errors.New("pubsub: Topic closed")
	t.mu.Unlock()
	c := make(chan struct{})
	go func() {
		defer close(c)
		t.batcher.Shutdown()
	}()
	select {
	case <-ctx.Done():
	case <-c:
	}
	t.cancel()
	return ctx.Err()
}

// As converts i to provider-specific types. See provider documentation for
// which type(s) are supported.
//
// See
// https://github.com/google/go-cloud/blob/master/internal/docs/design.md#as
// for more background.
func (t *Topic) As(i interface{}) bool {
	return t.driver.As(i)
}

// NewTopic makes a pubsub.Topic from a driver.Topic.
// It is for use by provider implementations.
func NewTopic(d driver.Topic) *Topic {
	callCtx, cancel := context.WithCancel(context.Background())
	handler := func(item interface{}) error {
		ms := item.([]*Message)
		var dms []*driver.Message
		for _, m := range ms {
			dm := &driver.Message{
				Body:     m.Body,
				Metadata: m.Metadata,
			}
			dms = append(dms, dm)
		}

		return retry.Call(callCtx, gax.Backoff{}, d.IsRetryable, func() error {
			return d.SendBatch(callCtx, dms)
		})
	}
	maxHandlers := 1
	b := batcher.New(reflect.TypeOf(&Message{}), maxHandlers, handler)
	t := &Topic{
		driver:  d,
		batcher: b,
		cancel:  cancel,
	}
	return t
}

// Subscription receives published messages.
type Subscription struct {
	driver driver.Subscription

	// ackBatcher makes batches of acks and sends them to the server.
	ackBatcher driver.Batcher

	mu sync.Mutex

	// q is the local queue of messages downloaded from the server.
	q   []*Message
	err error

	// cancel cancels all SendAcks calls.
	cancel func()
}

// Receive receives and returns the next message from the Subscription's queue,
// blocking and polling if none are available. This method can be called
// concurrently from multiple goroutines. The Ack() method of the returned
// Message has to be called once the message has been processed, to prevent it
// from being received again.
func (s *Subscription) Receive(ctx context.Context) (*Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
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
	var msgs []*driver.Message
	for len(msgs) == 0 {
		err := retry.Call(ctx, gax.Backoff{}, s.driver.IsRetryable, func() error {
			var err error
			// TODO(#691): dynamically adjust maxMessages
			const maxMessages = 10
			msgs, err = s.driver.ReceiveBatch(ctx, maxMessages)
			return err
		})
		if err != nil {
			return err
		}
	}
	s.q = nil
	for _, m := range msgs {
		id := m.AckID
		// size is an estimate of the size of a single AckID in bytes.
		const size = 8
		s.q = append(s.q, &Message{
			Body:     m.Body,
			Metadata: m.Metadata,
			ack: func() {
				s.ackBatcher.AddNoWait(id)
			},
		})
	}
	return nil
}

// Shutdown flushes pending ack sends and disconnects the Subscription.
func (s *Subscription) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.err = errors.New("pubsub: Subscription closed")
	s.mu.Unlock()
	c := make(chan struct{})
	go func() {
		defer close(c)
		s.ackBatcher.Shutdown()
	}()
	select {
	case <-ctx.Done():
	case <-c:
	}
	s.cancel()
	return ctx.Err()
}

// As converts i to provider-specific types. See provider documentation for
// which type(s) are supported.
//
// See
// https://github.com/google/go-cloud/blob/master/internal/docs/design.md#as
// for more background.
func (s *Subscription) As(i interface{}) bool {
	return s.driver.As(i)
}

// NewSubscription creates a Subscription from a driver.Subscription.
// It is for use by provider implementations.
func NewSubscription(d driver.Subscription) *Subscription {
	callCtx, cancel := context.WithCancel(context.Background())
	handler := func(items interface{}) error {
		ids := items.([]driver.AckID)
		// TODO: Consider providing a way to stop this call. See #766.
		return retry.Call(callCtx, gax.Backoff{}, d.IsRetryable, func() error {
			return d.SendAcks(callCtx, ids)
		})
	}
	const maxHandlers = 1
	ab := batcher.New(reflect.TypeOf([]driver.AckID{}).Elem(), maxHandlers, handler)
	return &Subscription{
		driver:     d,
		ackBatcher: ab,
		cancel:     cancel,
	}
}
