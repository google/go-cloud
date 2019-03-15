// Copyright 2019 The Go Cloud Development Kit Authors
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

package mempubsub_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/mempubsub"
)

func Example() {
	// Construct a *pubsub.Topic.
	ctx := context.Background()
	t := mempubsub.NewTopic()
	defer t.Shutdown(ctx)

	// Construct a *pubsub.Subscription for the topic.
	s := mempubsub.NewSubscription(t, 1*time.Minute /* ack deadline */)
	defer s.Shutdown(ctx)

	// Now we can use t to send messages and s will receive them.
	err := t.Send(ctx, &pubsub.Message{Body: []byte("Hello World")})
	if err != nil {
		log.Fatal(err)
	}

	msg, err := s.Receive(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(msg.Body))
	msg.Ack()

	// Output:
	// Hello World
}

func Example_openfromURL() {
	ctx := context.Background()

	// OpenTopic creates a *pubsub.Topic from a URL.
	// This URL will open the topic "mytopic".
	t, err := pubsub.OpenTopic(ctx, "mem://mytopic")

	// Similarly, OpenSubscription creates a *pubsub.Subscription from a URL.
	// This URL will open a subscription to the topic "mytopic", which must
	// have been previously opened using OpenTopic.
	s, err := pubsub.OpenSubscription(ctx, "mem://mytopic")
	_, _, _ = t, s, err
}
