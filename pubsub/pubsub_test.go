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
	"sync"
	"testing"
	"time"

	"github.com/google/go-cloud/pubsub"
	"github.com/google/go-cloud/pubsub/driver"
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

func (s *driverSub) ReceiveBatch(ctx context.Context) ([]*driver.Message, error) {
	for {
		select {
		case <-s.sem:
			ms := s.grabQueue()
			if len(ms) != 0 {
				return ms, nil
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		time.Sleep(time.Microsecond)
	}
}

func (s *driverSub) grabQueue() []*driver.Message {
	defer func() { s.sem <- struct{}{} }()
	if len(s.q) > 0 {
		ms := s.q
		s.q = nil
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

func TestSendReceive(t *testing.T) {
	ctx := context.Background()
	ds := NewDriverSub()
	dt := &driverTopic{
		subs: []*driverSub{ds},
	}
	topic := pubsub.NewTopic(ctx, dt, nil)
	m := &pubsub.Message{Body: []byte("user signed up")}
	if err := topic.Send(ctx, m); err != nil {
		t.Fatal(err)
	}

	sub := pubsub.NewSubscription(ctx, ds, nil)
	m2, err := sub.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if string(m2.Body) != string(m.Body) {
		t.Fatalf("received message has body %q, want %q", m2.Body, m.Body)
	}
	if err := topic.Close(); err != nil {
		t.Fatal(err)
	}
	if err := sub.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestLotsOfMessagesAndSubscriptions(t *testing.T) {
	howManyToSend := int(1e3)
	ctx := context.Background()
	dt := &driverTopic{}

	// Make a subscription and start goroutines to receive from it.
	var wg sync.WaitGroup
	ds := NewDriverSub()
	dt.subs = append(dt.subs, ds)
	s := pubsub.NewSubscription(ctx, ds, nil)
	ng := 10
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < howManyToSend/ng; j++ {
				if _, err := s.Receive(ctx); err != nil {
					panic(err)
				}
			}
		}()
	}

	// Send messages.
	topic := pubsub.NewTopic(ctx, dt, nil)
	for i := 0; i < howManyToSend; i++ {
		m := &pubsub.Message{Body: []byte("user signed up")}
		if err := topic.Send(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	// Wait for all the goroutines to finish processing all the messages.
	wg.Wait()

	// Clean up.
	if err := topic.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCancelSend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ds := NewDriverSub()
	dt := &driverTopic{
		subs: []*driverSub{ds},
	}
	topic := pubsub.NewTopic(ctx, dt, nil)
	m := &pubsub.Message{}

	// Intentionally break the driver subscription by acquiring its semaphore.
	// Now topic.Send will have to wait for cancellation.
	<-ds.sem

	cancel()
	if err := topic.Send(ctx, m); err == nil {
		t.Error("got nil, want cancellation error")
	}
	if err := topic.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCancelReceive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ds := NewDriverSub()
	s := pubsub.NewSubscription(ctx, ds, nil)
	cancel()
	// Without cancellation, this Receive would hang.
	if _, err := s.Receive(ctx); err == nil {
		t.Error("got nil, want cancellation error")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}
