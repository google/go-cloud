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
	"errors"
	"math/rand"
	"sync"
	"testing"
	"time"

	"golang.org/x/xerrors"

	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
)

type ackingDriverSub struct {
	driver.Subscription
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

func (*ackingDriverSub) IsRetryable(error) bool             { return false }
func (*ackingDriverSub) ErrorCode(error) gcerrors.ErrorCode { return gcerrors.Internal }
func (*ackingDriverSub) CanNack() bool                      { return false }
func (*ackingDriverSub) Close() error                       { return nil }

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
	sub := pubsub.NewSubscription(ds, nil, nil)
	defer sub.Shutdown(ctx)
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
	sub := pubsub.NewSubscription(ds, nil, nil)
	defer sub.Shutdown(ctx)

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
	sub := pubsub.NewSubscription(ds, nil, nil)
	defer sub.Shutdown(ctx)

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
	m := &driver.Message{AckID: 0} // the batcher doesn't like nil interfaces
	ds := &ackingDriverSub{
		q: []*driver.Message{m},
		sendAcks: func(_ context.Context, ackIDs []driver.AckID) error {
			<-ctx.Done()
			return nil
		},
	}
	sub := pubsub.NewSubscription(ds, nil, nil)
	defer sub.Shutdown(ctx)
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
	m := &driver.Message{AckID: 0} // the batcher doesn't like nil interfaces
	ds := &ackingDriverSub{
		q: []*driver.Message{m},
		sendAcks: func(_ context.Context, ackIDs []driver.AckID) error {
			return nil
		},
	}
	sub := pubsub.NewSubscription(ds, nil, nil)
	defer sub.Shutdown(ctx)
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
	m := &driver.Message{AckID: 0} // the batcher doesn't like nil interfaces
	ds := &ackingDriverSub{
		q: []*driver.Message{m},
		sendAcks: func(_ context.Context, ackIDs []driver.AckID) error {
			return nil
		},
	}
	sub := pubsub.NewSubscription(ds, nil, nil)
	defer sub.Shutdown(ctx)
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

func TestSubShutdownCanBeCanceledEvenWithHangingSendAcks(t *testing.T) {
	ctx := context.Background()
	m := &driver.Message{AckID: 0} // the batcher doesn't like nil interfaces
	ds := &ackingDriverSub{
		q: []*driver.Message{m},
		sendAcks: func(ctx context.Context, ackIDs []driver.AckID) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}
	sub := pubsub.NewSubscription(ds, nil, nil)
	mr, err := sub.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mr.Ack()

	done := make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()
		sub.Shutdown(ctx)
		close(done)
	}()
	tooLong := 5 * time.Second
	select {
	case <-done:
	case <-time.After(tooLong):
		t.Fatalf("waited too long (%v) for Shutdown to run", tooLong)
	}
}

func TestReceiveReturnsErrorFromSendAcks(t *testing.T) {
	// If SendAcks fails, the error is returned via receive.
	ctx := context.Background()
	serr := errors.New("SendAcks failed")
	ackChan := make(chan struct{})
	ds := &ackingDriverSub{
		q: []*driver.Message{
			{AckID: 0},
			{AckID: 1},
			{AckID: 2},
			{AckID: 3},
		},
		sendAcks: func(context.Context, []driver.AckID) error {
			close(ackChan)
			return serr
		},
	}
	sub := pubsub.NewSubscription(ds, nil, nil)
	defer sub.Shutdown(ctx)
	m, err := sub.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	m.Ack()
	// Wait for the ack to be sent.
	<-ackChan
	// It might take a bit longer for the logic after SendAcks returns to happen, so
	// keep calling Receive.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		_, err = sub.Receive(ctx)
		if gcerrors.Code(err) == gcerrors.Internal && err.(*gcerr.Error).Unwrap() == serr {
			break // success
		}
		if err != nil {
			t.Fatalf("got %v, want %v", err, serr)
		}
	}
}

// callbackDriverSub implements driver.Subscription and allows something like
// monkey patching of both its ReceiveBatch and SendAcks methods.
type callbackDriverSub struct {
	driver.Subscription
	mu           sync.Mutex
	receiveBatch func(context.Context) ([]*driver.Message, error)
	sendAcks     func(context.Context, []driver.AckID) error
}

func (s *callbackDriverSub) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	return s.receiveBatch(ctx)
}

func (s *callbackDriverSub) SendAcks(ctx context.Context, acks []driver.AckID) error {
	return s.sendAcks(ctx, acks)
}

func (*callbackDriverSub) IsRetryable(error) bool             { return false }
func (*callbackDriverSub) ErrorCode(error) gcerrors.ErrorCode { return gcerrors.Internal }
func (*callbackDriverSub) CanNack() bool                      { return false }
func (*callbackDriverSub) Close() error                       { return nil }

// This test detects the root cause of
// https://github.com/google/go-cloud/issues/1238.
// If the issue is present, this test times out. The problem was that when
// there were no messages available from the driver,
// pubsub.Subscription.Receive would spin trying to get more messages without
// checking to see if an unrecoverable error had occurred while sending a batch
// of acks to the driver.
func TestReceiveReturnsAckErrorOnNoMoreMessages(t *testing.T) {
	// If SendAcks fails, the error is returned via receive.
	ctx := context.Background()
	serr := errors.New("unrecoverable error")
	receiveHappened := make(chan struct{})
	ackHappened := make(chan struct{})
	var ds = &callbackDriverSub{
		// First call to receiveBatch will return a single message.
		receiveBatch: func(context.Context) ([]*driver.Message, error) {
			ms := []*driver.Message{{AckID: 1}}
			return ms, nil
		},
		sendAcks: func(context.Context, []driver.AckID) error {
			ackHappened <- struct{}{}
			return serr
		},
	}
	sub := pubsub.NewSubscription(ds, nil, nil)
	defer sub.Shutdown(ctx)
	m, err := sub.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	m.Ack()

	// Second call to receiveBatch will wait for the pull from the
	// receiveHappened channel below, and return a nil slice of messages.
	ds.mu.Lock()
	ds.receiveBatch = func(context.Context) ([]*driver.Message, error) {
		ds.mu.Lock()
		// Subsequent calls to receiveBatch won't wait on receiveHappened,
		// and will also return nil slices of messages.
		ds.receiveBatch = func(context.Context) ([]*driver.Message, error) {
			return nil, nil
		}
		ds.mu.Unlock()
		receiveHappened <- struct{}{}
		return nil, nil
	}
	ds.mu.Unlock()

	errc := make(chan error)
	go func() {
		_, err := sub.Receive(ctx)
		errc <- err
	}()

	// sub.Receive has to start running first and then we need to trigger the unrecoverable error.
	<-receiveHappened

	// Trigger the unrecoverable error.
	<-ackHappened

	// Wait for sub.Receive to return so we can check the error it returns against serr.
	err = <-errc

	// Check the error returned from sub.Receive.
	if got := gcerrors.Code(err); got != gcerrors.Internal {
		t.Fatalf("error code = %v; want %v", got, gcerrors.Internal)
	}
	if got := xerrors.Unwrap(err); got != serr {
		t.Errorf("error = %v; want %v", got, serr)
	}
}
