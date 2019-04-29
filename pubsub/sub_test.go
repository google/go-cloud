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
package pubsub_test

import (
	"context"
	"testing"

	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
)

// scriptedSub returns batches of messages in a predefined order from
// ReceiveBatch.
type scriptedSub struct {
	driver.Subscription
	// batches contains slices of messages to return from ReceiveBatch, one
	// after the other.
	batches [][]*driver.Message

	// calls counts how many times ReceiveBatch has been called.
	calls int

	// closed records if Close was called.
	closed bool
}

func (s *scriptedSub) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	b := s.batches[s.calls]
	s.calls++
	return b, nil
}

func (s *scriptedSub) SendAcks(ctx context.Context, ackIDs []driver.AckID) error {
	return nil
}

func (*scriptedSub) CanNack() bool { return false }
func (s *scriptedSub) Close() error {
	s.closed = true
	return nil
}

func TestReceiveWithEmptyBatchReturnedFromDriver(t *testing.T) {
	ctx := context.Background()
	ds := &scriptedSub{
		batches: [][]*driver.Message{
			// First call gets an empty batch.
			{},
			// Second call gets a non-empty batch.
			{&driver.Message{}},
		},
	}
	sub := pubsub.NewSubscription(ds, nil, nil)
	defer sub.Shutdown(ctx)
	m, err := sub.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	m.Ack()
}

func TestSubscriptionCloseIsCalled(t *testing.T) {
	ctx := context.Background()
	ds := &scriptedSub{}
	sub := pubsub.NewSubscription(ds, nil, nil)
	sub.Shutdown(ctx)
	if !ds.closed {
		t.Error("want Subscription.Close to have been called")
	}
}
