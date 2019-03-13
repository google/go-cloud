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

package pubsub_test

import (
	"context"
	"fmt"
	"log"

	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/mempubsub"
)

func ExampleOpenTopic() {
	ctx := context.Background()

	// Create a Topic using a URL.
	// This example uses "mempubsub", the in-memory implementation.
	// We need to add a blank import line to register the mempubsub provider's
	// URLOpener, which implements pubsub.TopicURLOpener:
	// import _ "gocloud.dev/secrets/mempubsub"
	// mempubsub registers for the "mem" scheme.
	topic, err := pubsub.OpenTopic(ctx, "mem://mytopic")
	if err != nil {
		log.Fatal(err)
	}
	// Similarly, we can open a subscription using a URL.
	sub, err := pubsub.OpenSubscription(ctx, "mem://mytopic")
	if err != nil {
		log.Fatal(err)
	}

	// Now we can use topic to send messages.
	if err := topic.Send(ctx, &pubsub.Message{Body: []byte("Hello, world!")}); err != nil {
		log.Fatal(err)
	}

	// And receive it from sub.
	m, err := sub.Receive(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s\n", m.Body)
	m.Ack()

	// Output:
	// Hello, world!
}
