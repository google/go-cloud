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

package mempubsub

import (
	"context"
	"testing"
	"time"

	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
)

func TestReceive(t *testing.T) {
	ctx := context.Background()
	topic := &topic{}
	sub := newSubscription(topic, 3*time.Second)
	if err := topic.SendBatch(ctx, []*driver.Message{
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

func TestOpenTopicFromURL(t *testing.T) {
	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"mem://mytopic", false},
		// Invalid parameter.
		{"mem://mytopic?param=value", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		topic, err := pubsub.OpenTopic(ctx, test.URL)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
		if topic != nil {
			topic.Shutdown(ctx)
		}
	}
}

func TestOpenSubscriptionFromURL(t *testing.T) {
	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"mem://mytopic", false},
		// OK with ackdeadline
		{"mem://mytopic?ackdeadline=30s", false},
		// Invalid ackdeadline
		{"mem://mytopic?ackdeadline=notaduration", true},
		// Nonexistent topic.
		{"mem://nonexistenttopic", true},
		// Invalid parameter.
		{"mem://myproject/mysub?param=value", true},
	}

	ctx := context.Background()
	pubsub.OpenTopic(ctx, "mem://mytopic")
	for _, test := range tests {
		sub, err := pubsub.OpenSubscription(ctx, test.URL)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
		if sub != nil {
			sub.Shutdown(ctx)
		}
	}
}
