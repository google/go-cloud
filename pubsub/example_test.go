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
	"fmt"
	"log"

	"gocloud.dev/pubsub"

	pbraw "cloud.google.com/go/pubsub/apiv1"
	pbapi "google.golang.org/genproto/googleapis/pubsub/v1"
	"google.golang.org/grpc/status"
)

func ExampleTopic_Send() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()
	var topic *pubsub.Topic

	err := topic.Send(ctx, &pubsub.Message{
		Body: []byte("Hello, World!\n"),
		// Metadata is optional and can be nil.
		Metadata: map[string]string{
			// These are examples of metadata.
			// There is nothing special about the key names.
			"language":   "en",
			"importance": "high",
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}

func ExampleSubscription_Receive() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()
	var subscription *pubsub.Subscription

	// Loop on received messages.
	for {
		msg, err := subscription.Receive(ctx)
		if err != nil {
			// Errors from Receive indicate that Receive will no longer succeed.
			log.Printf("Receiving message: %v", err)
			break
		}
		// Do work based on the message, for example:
		fmt.Printf("Got message: %q\n", msg.Body)
		// Messages must always be acknowledged with Ack.
		msg.Ack()
	}
}

func ExampleSubscription_Receive_concurrent() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()
	var subscription *pubsub.Subscription

	// Loop on received messages. We can use a channel as a semaphore to limit how
	// many goroutines we have active at a time as well as wait on the goroutines
	// to finish before exiting.
	const maxHandlers = 10
	sem := make(chan struct{}, maxHandlers)
recvLoop:
	for {
		msg, err := subscription.Receive(ctx)
		if err != nil {
			// Errors from Receive indicate that Receive will no longer succeed.
			log.Printf("Receiving message: %v", err)
			break
		}

		// Wait if there are too many active handle goroutines and acquire the
		// semaphore. If the context is canceled, stop waiting and start shutting
		// down.
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			break recvLoop
		}

		// Handle the message in a new goroutine.
		go func() {
			defer func() { <-sem }() // Release the semaphore.
			defer msg.Ack()          // Messages must always be acknowledged with Ack.

			// Do work based on the message, for example:
			fmt.Printf("Got message: %q\n", msg.Body)
		}()
	}

	// We're no longer receiving messages. Wait to finish handling any
	// unacknowledged messages by totally acquiring the semaphore.
	for n := 0; n < maxHandlers; n++ {
		sem <- struct{}{}
	}
}

func ExampleMessage_As() {
	// This example is specific to the gcppubsub implementation; it demonstrates
	// access to the underlying PubsubMessage type.
	// The types exposed for As by gcppubsub are documented in
	// https://godoc.org/gocloud.dev/pubsub/gcppubsub#hdr-As

	ctx := context.Background()
	sub, err := pubsub.OpenSubscription(ctx, "gcppubsub://project/topic")
	if err != nil {
		log.Fatal(err)
	}
	defer sub.Shutdown(ctx)

	msg, err := sub.Receive(ctx)
	if err != nil {
		log.Fatal(err)
	}
	var pm *pbapi.PubsubMessage
	if msg.As(&pm) {
		_ = pm.GetAttributes()
	}
	msg.Ack()
}

func ExampleSubscription_As() {
	// This example is specific to the gcppubsub implementation; it demonstrates
	// access to the underlying SubscriberClient type.
	// The types exposed for As by gcppubsub are documented in
	// https://godoc.org/gocloud.dev/pubsub/gcppubsub#hdr-As

	ctx := context.Background()
	sub, err := pubsub.OpenSubscription(ctx, "gcppubsub://project/topic")
	if err != nil {
		log.Fatal(err)
	}
	defer sub.Shutdown(ctx)

	var sc *pbraw.SubscriberClient
	if sub.As(&sc) {
		_ = sc.CallOptions
	}
}

func ExampleSubscription_ErrorAs() {
	// This example is specific to the gcppubsub implementation; it demonstrates
	// access to the underlying Status type.
	// The types exposed for As by gcppubsub are documented in
	// https://godoc.org/gocloud.dev/pubsub/gcppubsub#hdr-As

	ctx := context.Background()
	sub, err := pubsub.OpenSubscription(ctx, "gcppubsub://project/badtopic")
	if err != nil {
		log.Fatal(err)
	}
	defer sub.Shutdown(ctx)

	msg, err := sub.Receive(ctx)
	if err != nil {
		var s *status.Status
		if sub.ErrorAs(err, &s) {
			_ = s.Code()
		}
		log.Fatal(err)
	}
	msg.Ack()
}

func ExampleTopic_As() {
	// This example is specific to the gcppubsub implementation; it demonstrates
	// access to the underlying PublisherClient type.
	// The types exposed for As by gcppubsub are documented in
	// https://godoc.org/gocloud.dev/pubsub/gcppubsub#hdr-As

	ctx := context.Background()
	topic, err := pubsub.OpenTopic(ctx, "gcppubsub://project/topic")
	if err != nil {
		log.Fatal(err)
	}
	defer topic.Shutdown(ctx)

	var pc *pbraw.PublisherClient
	if topic.As(&pc) {
		_ = pc
	}
}

func ExampleTopic_ErrorAs() {
	// This example is specific to the gcppubsub implementation; it demonstrates
	// access to the underlying Status type.
	// The types exposed for As by gcppubsub are documented in
	// https://godoc.org/gocloud.dev/pubsub/gcppubsub#hdr-As

	ctx := context.Background()
	topic, err := pubsub.OpenTopic(ctx, "gcppubsub://project/topic")
	if err != nil {
		log.Fatal(err)
	}
	defer topic.Shutdown(ctx)

	err = topic.Send(ctx, &pubsub.Message{Body: []byte("hello")})
	if err != nil {
		var s *status.Status
		if topic.ErrorAs(err, &s) {
			_ = s.Code()
		}
		log.Fatal(err)
	}
}
