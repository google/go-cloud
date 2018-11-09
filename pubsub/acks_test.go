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
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/golang/go/src/pkg/math/rand"
	"github.com/google/go-cloud/pubsub"
	"github.com/google/go-cloud/pubsub/driver"
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
	f := func(ctx context.Context, ackIDs []driver.AckID) error {
		sentAcks = ackIDs
		return nil
	}
	id := rand.Int()
	m := &driver.Message{AckID: id}
	ds := &ackingDriverSub{
		q:        []*driver.Message{m},
		sendAcks: f,
	}
	sub := pubsub.NewSubscription(ctx, ds, pubsub.SubscriptionOptions{})
	m2, err := sub.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := m2.Ack(ctx); err != nil {
		t.Fatal(err)
	}
	if len(sentAcks) != 1 {
		t.Fatalf("len(sentAcks) = %d, want exactly 1", len(sentAcks))
	}
	if sentAcks[0] != id {
		t.Errorf("sentAcks[0] = %d, want %d", sentAcks[0], id)
	}
	if err := sub.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMultipleAcksCanGoIntoASingleBatch(t *testing.T) {
	ctx := context.Background()
	sentAcks := make(map[driver.AckID]int)
	f := func(ctx context.Context, ackIDs []driver.AckID) error {
		for _, id := range ackIDs {
			sentAcks[id]++
		}
		return nil
	}
	ids := []int{rand.Int(), rand.Int()}
	ds := &ackingDriverSub{
		q:        []*driver.Message{{AckID: ids[0]}, {AckID: ids[1]}},
		sendAcks: f,
	}
	sopts := pubsub.SubscriptionOptions{
		AckBatchSize: 2,
	}
	sub := pubsub.NewSubscription(ctx, ds, sopts)

	// Receive and ack the messages concurrently.
	var wg sync.WaitGroup
	recv := func() {
		defer wg.Done()
		mr, err := sub.Receive(ctx)
		if err != nil {
			t.Error(err)
			return
		}
		if err := mr.Ack(ctx); err != nil {
			t.Error(err)
			return
		}
	}
	wg.Add(2)
	go recv()
	go recv()
	wg.Wait()

	if len(sentAcks) != 2 {
		t.Errorf("len(sentAcks) = %d, want exactly 2", len(sentAcks))
	}
	for _, id := range ids {
		if sentAcks[id] != 1 {
			t.Errorf("sentAcks[%v] = %d, want 1", id, sentAcks[id])
		}
	}
	if err := sub.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestTooManyAcksForASingleBatchGoIntoMultipleBatches(t *testing.T) {
	ctx := context.Background()
	var sentAckBatches [][]driver.AckID
	f := func(ctx context.Context, ackIDs []driver.AckID) error {
		sentAckBatches = append(sentAckBatches, ackIDs)
		return nil
	}
	ids := []int{rand.Int(), rand.Int()}
	ds := &ackingDriverSub{
		q:        []*driver.Message{{AckID: ids[0]}, {AckID: ids[1]}},
		sendAcks: f,
	}
	sopts := pubsub.SubscriptionOptions{
		AckBatchSize: 1,
	}
	sub := pubsub.NewSubscription(ctx, ds, sopts)

	// Receive and ack the messages concurrently.
	var wg sync.WaitGroup
	recv := func() {
		mr, err := sub.Receive(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := mr.Ack(ctx); err != nil {
			t.Fatal(err)
		}
		wg.Done()
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

	if err := sub.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMsgAckReturnsErrorFromSendAcks(t *testing.T) {
	ctx := context.Background()
	e := fmt.Sprintf("%d", rand.Int())
	f := func(ctx context.Context, ackIDs []driver.AckID) error {
		return errors.New(e)
	}
	m := &driver.Message{}
	ds := &ackingDriverSub{
		q:        []*driver.Message{m},
		sendAcks: f,
	}
	sub := pubsub.NewSubscription(ctx, ds, pubsub.SubscriptionOptions{})
	mr, err := sub.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = mr.Ack(ctx)
	if err == nil {
		t.Fatal("got nil, want error")
	}
	if err.Error() != e {
		t.Errorf("got error %q, want %q", err.Error(), e)
	}
	if err := sub.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCancelAck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := func(ctx context.Context, ackIDs []driver.AckID) error {
		// Hang.
		c := make(chan struct{})
		<-c
		return nil
	}
	m := &driver.Message{}
	ds := &ackingDriverSub{
		q:        []*driver.Message{m},
		sendAcks: f,
	}
	sub := pubsub.NewSubscription(ctx, ds, pubsub.SubscriptionOptions{})
	mr, err := sub.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := mr.Ack(ctx); err == nil {
		t.Error("got nil, want cancellation error")
	}
	if err := sub.Close(); err != nil {
		t.Fatal(err)
	}
}
