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
	"sync"
	"time"

	"github.com/google/go-cloud/internal/pubsub/driver"
	"github.com/google/go-cloud/internal/retry"
	gax "github.com/googleapis/gax-go"
	"google.golang.org/api/support/bundler"
)

// Message contains data to be published.
type Message struct {
	// Body contains the content of the message.
	Body []byte

	// Metadata has key/value metadata for the message.
	Metadata map[string]string

	// ack is a closure that queues this message for acknowledgement.
	ack func()
}

// Ack acknowledges the message, telling the server that it does not need to be
// sent again to the associated Subscription. It returns immediately, but the
// actual ack is sent in the background, and is not guaranteed to succeed.
func (m *Message) Ack() {
	go m.ack()
}

// Topic publishes messages to all its subscribers.
type Topic struct {
	driver  driver.Topic
	batcher *bundler.Bundler
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
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
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
func NewTopic(d driver.Topic, b *bundler.Bundler) *Topic {
	return &Topic{
		driver:  d,
		batcher: b,
	}
}

// NewSendBatcher creates a batcher for message sends, given a driver.Topic.
// It is for use by provider implementations.
func NewSendBatcher(d driver.Topic) *bundler.Bundler {
	handle := func(item interface{}) {
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
			}
			dms = append(dms, dm)
		}

		callCtx := context.TODO()
		err := retry.Call(callCtx, gax.Backoff{}, d.IsRetryable, func() error {
			return d.SendBatch(callCtx, dms)
		})
		for _, mec := range mecs {
			mec.errChan <- err
		}
	}
	return bundler.NewBundler(msgErrChan{}, handle)
}

// Subscription receives published messages.
type Subscription struct {
	driver driver.Subscription

	// ackBatcher makes batches of acks and sends them to the server.
	ackBatcher *bundler.Bundler

	mu sync.Mutex

	// q is the local queue of messages downloaded from the server.
	q []*Message
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
	if len(msgs) == 0 {
		return errors.New("subscription driver bug: received empty batch")
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
				s.ackBatcher.Add(ackIDBox{id}, size)
			},
		})
	}
	return nil
}

// Close flushes pending ack sends and disconnects the Subscription.
func (s *Subscription) Close() error {
	s.ackBatcher.Flush()
	return s.driver.Close()
}

// ackIDBox makes it possible to use a driver.AckID with bundler.
type ackIDBox struct {
	ackID driver.AckID
}

// NewSubscription creates a Subscription from a driver.Subscription and opts to
// tune sending and receiving of acks and messages. Behind the scenes,
// NewSubscription spins up a goroutine to gather acks into batches and
// periodically send them to the server.
// It is for use by provider implementations.
func NewSubscription(d driver.Subscription) *Subscription {
	handler := func(item interface{}) {
		boxes := item.([]ackIDBox)
		var ids []driver.AckID
		for _, box := range boxes {
			id := box.ackID
			ids = append(ids, id)
		}
		// TODO: Consider providing a way to stop this call. See #766.
		callCtx := context.Background()
		err := retry.Call(callCtx, gax.Backoff{}, d.IsRetryable, func() error {
			return d.SendAcks(callCtx, ids)
		})
		// TODO(#695): Do something sensible if SendAcks returns an error.
		_ = err
	}
	ab := bundler.NewBundler(ackIDBox{}, handler)
	ab.DelayThreshold = time.Millisecond
	return &Subscription{
		driver:     d,
		ackBatcher: ab,
	}
}
