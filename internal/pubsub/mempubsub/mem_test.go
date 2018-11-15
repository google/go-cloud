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
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/google/go-cloud/internal/pubsub/driver"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestSendReceive(t *testing.T) {
	ctx := context.Background()
	top := newTopic("t")
	sub := newSubscription(top, 3*time.Second)
	want := []*driver.Message{
		{Body: []byte("a")},
		{Body: []byte("b")},
		{Body: []byte("c")},
	}
	if err := top.SendBatch(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := sub.ReceiveBatch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	less := func(x, y *driver.Message) bool { return bytes.Compare(x.Body, y.Body) < 0 }
	if diff := cmp.Diff(got, want, cmpopts.SortSlices(less)); diff != "" {
		t.Error(diff)
	}
}

func TestReceive(t *testing.T) {
	ctx := context.Background()
	top := newTopic("t")
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

func TestErrors(t *testing.T) {
	ctx := context.Background()
	wantErr := func(err error) {
		t.Helper()
		if err == nil {
			t.Error("got nil, want error")
		}
	}

	top := newTopic("t")
	top.Close()
	wantErr(top.SendBatch(ctx, nil)) // topic closed

	top = newTopic("t")
	sub := newSubscription(top, time.Second)
	sub.Close()
	_, err := sub.ReceiveBatch(ctx)
	wantErr(err) // sub closed
}

func TestCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	wantCanceled := func(err error) {
		t.Helper()
		if err != context.Canceled {
			t.Errorf("got %v, want context.Canceled", err)
		}
	}
	top := newTopic("t")

	wantCanceled(top.SendBatch(ctx, nil))
	sub := newSubscription(top, time.Second)
	_, err := sub.ReceiveBatch(ctx)
	wantCanceled(err)
	wantCanceled(sub.SendAcks(ctx, nil))
}
