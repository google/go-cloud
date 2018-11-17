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
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/google/go-cloud/internal/pubsub"
	"github.com/google/go-cloud/internal/pubsub/mempubsub"
)

func Example_sendReceive() {
	// Open a topic and corresponding subscription.
	ctx := context.Background()
	dt := mempubsub.OpenTopic()
	ds := mempubsub.OpenSubscription(dt, time.Second)
	t := pubsub.NewTopic(ctx, dt)
	defer t.Close()
	s := pubsub.NewSubscription(ctx, ds)
	defer s.Close()

	// Send a message to the topic.
	if err := t.Send(ctx, &pubsub.Message{Body: []byte("Hello, world!")}); err != nil {
		log.Fatal(err)
	}

	// Receive a message from the subscription.
	m, err := s.Receive(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Print out the received message.
	fmt.Printf("%s\n", m.Body)

	// Acknowledge the message.
	m.Ack()

	// Output:
	// Hello, world!
}

func Example_sendReceiveMultipleMessages() {
	// Open a topic and corresponding subscription.
	ctx := context.Background()
	dt := mempubsub.OpenTopic()
	ds := mempubsub.OpenSubscription(dt, time.Second)
	t := pubsub.NewTopic(ctx, dt)
	defer t.Close()
	s := pubsub.NewSubscription(ctx, ds)
	defer s.Close()

	// Send messages to the topic.
	ms := []*pubsub.Message{
		{Body: []byte("a")},
		{Body: []byte("b")},
		{Body: []byte("c")},
	}
	for _, m := range ms {
		if err := t.Send(ctx, m); err != nil {
			log.Fatal(err)
		}
	}

	// Receive messages from the subscription.
	ms2 := []*pubsub.Message{}
	for i := 0; i < len(ms); i++ {
		m2, err := s.Receive(ctx)
		if err != nil {
			log.Fatal(err)
		}
		ms2 = append(ms2, m2)
	}

	// The messages may be received in a different order than they were
	// sent.
	sort.Slice(ms2, func(i, j int) bool { return string(ms2[i].Body) < string(ms2[j].Body) })

	// Print out and acknowledge the received messages.
	for _, m2 := range ms2 {
		fmt.Printf("%s\n", m2.Body)
		m2.Ack()
	}

	// Output:
	// a
	// b
	// c
}
