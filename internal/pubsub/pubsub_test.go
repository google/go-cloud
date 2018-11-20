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
	"math/rand"
	"sync"
	"testing"

	"github.com/google/go-cloud/internal/pubsub"
	"github.com/google/go-cloud/internal/pubsub/driver"
	"github.com/google/go-cloud/internal/retry"
)

type driverTopic struct {
	subs []*driverSub
}

func (t *driverTopic) SendBatch(ctx context.Context, ms []*driver.Message) error {
	for _, s := range t.subs {
		select {
		case <-s.sem:
			s.q = append(s.q, ms...)
			s.sem <- struct{}{}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (t *driverTopic) Close() error {
	return nil
}

func (s *driverTopic) IsRetryable(error) bool { return false }

type driverSub struct {
	sem chan struct{}
	// Normally this queue would live on a separate server in the cloud.
	q []*driver.Message
}

func NewDriverSub() *driverSub {
	ds := &driverSub{
		sem: make(chan struct{}, 1),
	}
	ds.sem <- struct{}{}
	return ds
}

func (s *driverSub) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	for {
		select {
		case <-s.sem:
			ms := s.grabQueue(maxMessages)
			if len(ms) != 0 {
				return ms, nil
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
}

func (s *driverSub) grabQueue(maxMessages int) []*driver.Message {
	defer func() { s.sem <- struct{}{} }()
	if len(s.q) > 0 {
		if len(s.q) <= maxMessages {
			ms := s.q
			s.q = nil
			return ms
		}
		ms := s.q[:maxMessages]
		s.q = s.q[maxMessages:]
		return ms
	}
	return nil
}

func (s *driverSub) SendAcks(ctx context.Context, ackIDs []driver.AckID) error {
	return nil
}

func (s *driverSub) Close() error {
	return nil
}

func (s *driverSub) IsRetryable(error) bool { return false }

func TestSendReceive(t *testing.T) {
	ctx := context.Background()
	ds := NewDriverSub()
	dt := &driverTopic{
		subs: []*driverSub{ds},
	}
	topic := pubsub.NewTopic(dt)
	defer topic.Close()
	m := &pubsub.Message{Body: []byte("user signed up")}
	if err := topic.Send(ctx, m); err != nil {
		t.Fatal(err)
	}

	sub := pubsub.NewSubscription(ds)
	defer sub.Close()
	m2, err := sub.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if string(m2.Body) != string(m.Body) {
		t.Fatalf("received message has body %q, want %q", m2.Body, m.Body)
	}
}

func TestConcurrentReceivesGetAllTheMessages(t *testing.T) {
	howManyToSend := int(1e3)
	ctx, cancel := context.WithCancel(context.Background())
	dt := &driverTopic{}

	// Make a subscription and start goroutines to receive from it.
	var wg sync.WaitGroup
	wg.Add(howManyToSend)
	ds := NewDriverSub()
	dt.subs = append(dt.subs, ds)
	s := pubsub.NewSubscription(ds)
	defer s.Close()
	var mu sync.Mutex
	receivedMsgs := make(map[string]int)
	for i := 0; i < 10; i++ {
		go func() {
			for {
				m, err := s.Receive(ctx)
				if err != nil {
					if isCanceled(err) {
						return
					}
					t.Fatal(err)
				}
				mu.Lock()
				receivedMsgs[string(m.Body)]++
				mu.Unlock()
				wg.Done()
			}
		}()
	}

	// Send messages.
	topic := pubsub.NewTopic(dt)
	defer topic.Close()
	sentMsgs := make(map[string]int)
	for i := 0; i < howManyToSend; i++ {
		bod := fmt.Sprintf("%d", rand.Int())
		m := &pubsub.Message{Body: []byte(bod)}
		sentMsgs[string(m.Body)]++
		if err := topic.Send(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	// Wait for all the goroutines to finish processing all the messages.
	wg.Wait()
	cancel()

	// Check that all the messages were received.
	sum := 0
	for _, n := range receivedMsgs {
		sum += n
	}
	if sum != howManyToSend {
		t.Errorf("received %d messages, want %d", sum, howManyToSend)
	}
	for k, v := range sentMsgs {
		v2 := receivedMsgs[k]
		if v2 != v {
			t.Errorf("got %d for %q, want %d", v2, k, v)
		}
	}
}

func TestCancelSend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ds := NewDriverSub()
	dt := &driverTopic{
		subs: []*driverSub{ds},
	}
	topic := pubsub.NewTopic(dt)
	defer topic.Close()
	m := &pubsub.Message{}

	// Intentionally break the driver subscription by acquiring its semaphore.
	// Now topic.Send will have to wait for cancellation.
	<-ds.sem

	cancel()
	if err := topic.Send(ctx, m); err == nil {
		t.Error("got nil, want cancellation error")
	}
}

func TestCancelReceive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ds := NewDriverSub()
	s := pubsub.NewSubscription(ds)
	defer s.Close()
	cancel()
	// Without cancellation, this Receive would hang.
	if _, err := s.Receive(ctx); err == nil {
		t.Error("got nil, want cancellation error")
	}
}

func TestRetryTopic(t *testing.T) {
	// Test that Send is retried if the driver returns a retryable error.
	ft := &failTopic{}
	top := pubsub.NewTopic(ft)
	err := top.Send(context.Background(), &pubsub.Message{})
	if err != nil {
		t.Errorf("Send: got %v, want nil", err)
	}
	if got, want := ft.calls, nRetryCalls+1; got != want {
		t.Errorf("calls: got %d, want %d", got, want)
	}
}

var errRetry = errors.New("retry")

func isRetryable(err error) bool {
	return err == errRetry
}

const nRetryCalls = 2

type failTopic struct {
	driver.Topic
	calls int
}

func (t *failTopic) SendBatch(ctx context.Context, ms []*driver.Message) error {
	t.calls++
	if t.calls <= nRetryCalls {
		return errRetry
	}
	return nil
}

func (t *failTopic) IsRetryable(err error) bool { return isRetryable(err) }

func TestRetryReceive(t *testing.T) {
	fs := &failSub{}
	sub := pubsub.NewSubscription(fs)
	_, err := sub.Receive(context.Background())
	if err != nil {
		t.Errorf("Receive: got %v, want nil", err)
	}
	if got, want := fs.calls, nRetryCalls+1; got != want {
		t.Errorf("calls: got %d, want %d", got, want)
	}
}

type failSub struct {
	driver.Subscription
	calls int
}

func (t *failSub) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	t.calls++
	if t.calls <= nRetryCalls {
		return nil, errRetry
	}
	return []*driver.Message{{Body: []byte("")}}, nil
}

func (t *failSub) IsRetryable(err error) bool { return isRetryable(err) }

// TODO(jba): add a test for retry of SendAcks.

func isCanceled(err error) bool {
	if cerr, ok := err.(*retry.ContextError); ok {
		err = cerr.CtxErr
	}
	return err == context.Canceled
}
