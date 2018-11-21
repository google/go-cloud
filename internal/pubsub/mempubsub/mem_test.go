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

package mempubsub

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cloud/internal/pubsub"
	"github.com/google/go-cloud/internal/pubsub/driver"
)

func TestReceive(t *testing.T) {
	b := NewBroker([]string{"t"})
	ctx := context.Background()
	top := b.topic("t")
	sub := newSubscription(top, 3*time.Second)
	if err := top.SendBatch(ctx, []*driver.Message{
		{Body: []byte("a")},
		{Body: []byte("b")},
		{Body: []byte("c")},
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	// We should get only two of the published messages.
	msgs := sub.receiveNoWait(now, 2)
	if got, want := len(msgs), 2; got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
	// We should get the remaining message.
	msgs = sub.receiveNoWait(now, 2)
	if got, want := len(msgs), 1; got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
	// Since all the messages are outstanding, we shouldn't get any.
	msgs2 := sub.receiveNoWait(now, 10)
	if got, want := len(msgs2), 0; got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
	// Advance time past expiration, and we should get all the messages again,
	// since we didn't ack any.
	now = now.Add(time.Hour)
	msgs = sub.receiveNoWait(now, 10)
	if got, want := len(msgs), 3; got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
	// Again, since all the messages are outstanding, we shouldn't get any.
	msgs2 = sub.receiveNoWait(now, 10)
	if got, want := len(msgs2), 0; got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
	// Now ack the messages.
	var ackIDs []driver.AckID
	for _, m := range msgs {
		ackIDs = append(ackIDs, m.AckID)
	}
	sub.SendAcks(ctx, ackIDs)
	// They will never be delivered again, even if we wait past the ack deadline.
	now = now.Add(time.Hour)
	msgs = sub.receiveNoWait(now, 10)
	if got, want := len(msgs), 0; got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
}

func TestNotExist(t *testing.T) {
	ctx := context.Background()
	b := NewBroker([]string{"t"})
	top := OpenTopic(b, "x")
	err := top.Send(ctx, &pubsub.Message{Body: []byte("x")})
	if err != errNotExist {
		t.Errorf("got %v, want %v", err, errNotExist)
	}
	sub := OpenSubscription(b, "x", time.Second)
	_, err = sub.Receive(ctx)
	if err != errNotExist {
		t.Errorf("got %v, want %v", err, errNotExist)
	}
}
