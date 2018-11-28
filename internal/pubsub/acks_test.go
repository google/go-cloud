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
package pubsub_test

import (
	"context"
	"math/rand"
	"sync"
	"testing"

	"github.com/google/go-cloud/internal/pubsub"
	"github.com/google/go-cloud/internal/pubsub/driver"
)

type ackingDriverSub struct {
	q        []*driver.Message
	sendAcks func(context.Context, []driver.AckID) error
}

func (s *ackingDriverSub) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	if len(s.q) <= maxMessages {
		ms := s.q
		s.q = nil
		return ms, nil
	}
	ms := s.q[:maxMessages]
	s.q = s.q[maxMessages:]
	return ms, nil
}

func (s *ackingDriverSub) SendAcks(ctx context.Context, ackIDs []driver.AckID) error {
	return s.sendAcks(ctx, ackIDs)
}

func (s *ackingDriverSub) Close() error {
	return nil
}

func (s *ackingDriverSub) IsRetryable(error) bool { return false }

func TestAckTriggersDriverSendAcksForOneMessage(t *testing.T) {
	ctx := context.Background()
	var mu sync.Mutex
	var sentAcks []driver.AckID
	id := rand.Int()
	m := &driver.Message{AckID: id}
	ackChan := make(chan struct{})
	ds := &ackingDriverSub{
		q: []*driver.Message{m},
		sendAcks: func(_ context.Context, ackIDs []driver.AckID) error {
			mu.Lock()
			defer mu.Unlock()
			sentAcks = ackIDs
			ackChan <- struct{}{}
			return nil
		},
	}
	sub := pubsub.NewSubscription(ds)
	defer sub.Close()
	m2, err := sub.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	m2.Ack()
	<-ackChan
	if len(sentAcks) != 1 {
		t.Fatalf("len(sentAcks) = %d, want exactly 1", len(sentAcks))
	}
	if sentAcks[0] != id {
		t.Errorf("sentAcks[0] = %d, want %d", sentAcks[0], id)
	}
}

func TestMultipleAcksCanGoIntoASingleBatch(t *testing.T) {
	ctx := context.Background()
	var wg sync.WaitGroup
	var mu sync.Mutex
	sentAcks := make(map[driver.AckID]int)
	ids := []int{1, 2}
	ds := &ackingDriverSub{
		q: []*driver.Message{{AckID: ids[0]}, {AckID: ids[1]}},
		sendAcks: func(_ context.Context, ackIDs []driver.AckID) error {
			mu.Lock()
			defer mu.Unlock()
			for _, id := range ackIDs {
				sentAcks[id]++
				wg.Done()
			}
			return nil
		},
	}
	sub := pubsub.NewSubscription(ds)
	defer sub.Close()

	// Receive and ack the messages concurrently.
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			mr, err := sub.Receive(ctx)
			if err != nil {
				t.Error(err)
				return
			}
			mr.Ack()
		}()
	}
	wg.Wait()

	if len(sentAcks) != 2 {
		t.Errorf("len(sentAcks) = %d, want exactly 2", len(sentAcks))
	}
	for _, id := range ids {
		if sentAcks[id] != 1 {
			t.Errorf("sentAcks[%v] = %d, want 1", id, sentAcks[id])
		}
	}
}

func TestTooManyAcksForASingleBatchGoIntoMultipleBatches(t *testing.T) {
	ctx := context.Background()
	var mu sync.Mutex
	var wg sync.WaitGroup
	var sentAckBatches [][]driver.AckID
	// This value of n is chosen large enough that it should create more
	// than one batch. Admittedly, there is currently no explicit guarantee
	// of this.
	n := 1000
	var ms []*driver.Message
	for i := 0; i < n; i++ {
		ms = append(ms, &driver.Message{AckID: i})
	}
	ds := &ackingDriverSub{
		q: ms,
		sendAcks: func(_ context.Context, ackIDs []driver.AckID) error {
			mu.Lock()
			defer mu.Unlock()
			sentAckBatches = append(sentAckBatches, ackIDs)
			for i := 0; i < len(ackIDs); i++ {
				wg.Done()
			}
			return nil
		},
	}
	sub := pubsub.NewSubscription(ds)
	defer sub.Close()

	// Receive and ack the messages concurrently.
	recv := func() {
		mr, err := sub.Receive(ctx)
		if err != nil {
			t.Fatal(err)
		}
		mr.Ack()
	}
	wg.Add(n)
	for i := 0; i < n; i++ {
		go recv()
	}
	wg.Wait()

	if len(sentAckBatches) < 2 {
		t.Errorf("got %d batches, want at least 2", len(sentAckBatches))
	}
}

func TestAckDoesNotBlock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &driver.Message{}
	ds := &ackingDriverSub{
		q: []*driver.Message{m},
		sendAcks: func(_ context.Context, ackIDs []driver.AckID) error {
			<-ctx.Done()
			return nil
		},
	}
	sub := pubsub.NewSubscription(ds)
	defer sub.Close()
	defer cancel()
	mr, err := sub.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// If Ack blocks here, waiting for sendAcks to finish, then the
	// deferred cancel() will never run, so sendAcks can never finish. That
	// would cause the test to hang. Thus hanging is how this test signals
	// failure.
	mr.Ack()
}

func TestDoubleAckCausesPanic(t *testing.T) {
	ctx := context.Background()
	m := &driver.Message{}
	ds := &ackingDriverSub{
		q: []*driver.Message{m},
		sendAcks: func(_ context.Context, ackIDs []driver.AckID) error {
			return nil
		},
	}
	sub := pubsub.NewSubscription(ds)
	defer sub.Close()
	mr, err := sub.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mr.Ack()
	defer func() {
		if r := recover(); r != nil {
			// ok, panic was expected.
			return
		}
		t.Errorf("second ack failed to panic")
	}()
	mr.Ack()
}

// For best results, run this test with -race.
func TestConcurrentDoubleAckCausesPanic(t *testing.T) {
	ctx := context.Background()
	m := &driver.Message{}
	ds := &ackingDriverSub{
		q: []*driver.Message{m},
		sendAcks: func(_ context.Context, ackIDs []driver.AckID) error {
			return nil
		},
	}
	sub := pubsub.NewSubscription(ds)
	defer sub.Close()
	mr, err := sub.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Spin up some goroutines to ack the message.
	var mu sync.Mutex
	panics := 0
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					mu.Lock()
					defer mu.Unlock()
					panics++
				}
			}()
			mr.Ack()
		}()
	}
	wg.Wait()

	// Check that one of the goroutines panicked.
	if panics != 1 {
		t.Errorf("panics = %d, want %d", panics, 1)
	}
}
