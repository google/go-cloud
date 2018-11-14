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
	"reflect"
	"sync"
	"testing"

	"github.com/google/go-cloud/internal/pubsub"
	"github.com/google/go-cloud/internal/pubsub/driver"
)

type ackingDriverSub struct {
	q        []*driver.Message
	sendAcks func(context.Context, []driver.AckID) error
}

func (s *ackingDriverSub) ReceiveBatch(ctx context.Context) ([]*driver.Message, error) {
	ms := s.q
	s.q = nil
	return ms, nil
}

func (s *ackingDriverSub) SendAcks(ctx context.Context, ackIDs []driver.AckID) error {
	return s.sendAcks(ctx, ackIDs)
}

func (s *ackingDriverSub) Close() error {
	return nil
}

func TestAckTriggersDriverSendAcksForOneMessage(t *testing.T) {
	ctx := context.Background()
	var sentAcks []driver.AckID
	id := rand.Int()
	m := &driver.Message{AckID: id}
	ackChan := make(chan struct{})
	ds := &ackingDriverSub{
		q: []*driver.Message{m},
		sendAcks: func(_ context.Context, ackIDs []driver.AckID) error {
			sentAcks = ackIDs
			ackChan <- struct{}{}
			return nil
		},
	}
	sub := pubsub.NewSubscription(ctx, ds, nil)
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
	sentAcks := make(map[driver.AckID]int)
	ids := []int{1, 2}
	ds := &ackingDriverSub{
		q: []*driver.Message{{AckID: ids[0]}, {AckID: ids[1]}},
		sendAcks: func(_ context.Context, ackIDs []driver.AckID) error {
			for _, id := range ackIDs {
				sentAcks[id]++
				wg.Done()
			}
			return nil
		},
	}
	sopts := pubsub.DefaultSubscriptionOptions
	sopts.AckBatchCountThreshold = 2
	sub := pubsub.NewSubscription(ctx, ds, &sopts)
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
	var wg sync.WaitGroup
	var sentAckBatches [][]driver.AckID
	ids := []int{rand.Int(), rand.Int()}
	ds := &ackingDriverSub{
		q: []*driver.Message{{AckID: ids[0]}, {AckID: ids[1]}},
		sendAcks: func(_ context.Context, ackIDs []driver.AckID) error {
			sentAckBatches = append(sentAckBatches, ackIDs)
			for i := 0; i < len(ackIDs); i++ {
				wg.Done()
			}
			return nil
		},
	}
	sopts := pubsub.DefaultSubscriptionOptions
	sopts.AckBatchCountThreshold = 1
	sub := pubsub.NewSubscription(ctx, ds, &sopts)
	defer sub.Close()

	// Receive and ack the messages concurrently.
	recv := func() {
		mr, err := sub.Receive(ctx)
		if err != nil {
			t.Fatal(err)
		}
		mr.Ack()
	}
	wg.Add(2)
	go recv()
	go recv()
	wg.Wait()

	want := [][]driver.AckID{
		{ids[0]},
		{ids[1]},
	}
	if !reflect.DeepEqual(sentAckBatches, want) {
		t.Errorf("got %v, want %v", sentAckBatches, want)
	}
}

func TestAckDoesNotBlock(t *testing.T) {
	ctx := context.Background()
	m := &driver.Message{}
	ds := &ackingDriverSub{
		q: []*driver.Message{m},
		sendAcks: func(_ context.Context, ackIDs []driver.AckID) error {
			select {}
			return nil
		},
	}
	sub := pubsub.NewSubscription(ctx, ds, nil)
	defer sub.Close()
	mr, err := sub.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mr.Ack()
}
