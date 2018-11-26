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
	"sync"
	"time"

	"github.com/google/go-cloud/internal/pubsub"
	"github.com/google/go-cloud/internal/pubsub/mempubsub"
)

func Example_sendReceive() {
	// Open a topic and corresponding subscription.
	ctx := context.Background()
	b := mempubsub.NewBroker([]string{"myTopic"})
	t := mempubsub.OpenTopic(b, "myTopic")
	defer t.Close()
	s := mempubsub.OpenSubscription(b, "myTopic", time.Second)
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
	b := mempubsub.NewBroker([]string{"myTopic"})
	t := mempubsub.OpenTopic(b, "myTopic")
	defer t.Close()
	s := mempubsub.OpenSubscription(b, "myTopic", time.Second)
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
	ms2 := []string{}
	for i := 0; i < len(ms); i++ {
		m2, err := s.Receive(ctx)
		if err != nil {
			log.Fatal(err)
		}
		ms2 = append(ms2, string(m2.Body))
		m2.Ack()
	}

	// The messages may be received in a different order than they were
	// sent.
	sort.Strings(ms2)

	// Print out and acknowledge the received messages.
	for _, m2 := range ms2 {
		fmt.Println(m2)
	}

	// Output:
	// a
	// b
	// c
}

func Example_receiveWithInvertedWorkerPool() {
	// Open a topic and corresponding subscription.
	ctx := context.Background()
	b := mempubsub.NewBroker([]string{"myTopic"})
	t := mempubsub.OpenTopic(b, "myTopic")
	defer t.Close()
	s := mempubsub.OpenSubscription(b, "myTopic", time.Second)
	defer s.Close()

	// Send a bunch of messages to the topic.
	const nMessages = 100
	for n := 0; n < nMessages; n++ {
		m := &pubsub.Message{
			Body: []byte(fmt.Sprintf("message %d", n)),
		}
		if err := t.Send(ctx, m); err != nil {
			log.Fatal(err)
		}
	}

	// In order to make our test exit, we keep track of how many messages were
	// processed with wg, and cancel the receiveCtx when we've processed them all.
	// A more realistic application would not need this WaitGroup.
	var wg sync.WaitGroup
	wg.Add(nMessages)
	receiveCtx, cancel := context.WithCancel(ctx)
	go func() {
		wg.Wait()
		cancel()
	}()

	// Process messages using an inverted worker pool, as described here:
	// https://www.youtube.com/watch?v=5zXAHh5tJqQ&t=26m58s
	// It uses a buffered channel, sem, as a semaphore to manage the maximum
	// number of workers.
	const poolSize = 10
	sem := make(chan struct{}, poolSize)
	for {
		// Read a message. Receive will block until a message is available.
		msg, err := s.Receive(receiveCtx)
		if err != nil {
			// An error from Receive is fatal; Receive will never succeed again
			// so the application should exit.
			// In this example, we expect to get a error here when we've read all the
			// messages and receiveCtx is canceled.
			break
		}

		// Write a token to the semaphore; if there are already poolSize workers
		// active, this will block until one of them completes.
		sem <- struct{}{}
		// Process the message. For many applications, this can be expensive, so
		// we do it in a goroutine, allowing this loop to continue and Receive more
		// messages.
		go func() {
			// Record that we've processed this message, and Ack it.
			msg.Ack()
			wg.Done()
			// Read a token from the semaphore before exiting this goroutine, freeing
			// up the slot for another goroutine.
			<-sem
		}()
	}

	// Wait for all workers to finish.
	for n := poolSize; n > 0; n-- {
		sem <- struct{}{}
	}
	fmt.Printf("Read %d messages", nMessages)

	// Output:
	// Read 100 messages
}

func Example_receiveWithTraditionalWorkerPool() {
	// Open a topic and corresponding subscription.
	ctx := context.Background()
	b := mempubsub.NewBroker([]string{"myTopic"})
	t := mempubsub.OpenTopic(b, "myTopic")
	defer t.Close()
	s := mempubsub.OpenSubscription(b, "myTopic", time.Second)
	defer s.Close()

	// Send a bunch of messages to the topic.
	const nMessages = 100
	for n := 0; n < nMessages; n++ {
		m := &pubsub.Message{
			Body: []byte(fmt.Sprintf("message %d", n)),
		}
		if err := t.Send(ctx, m); err != nil {
			log.Fatal(err)
		}
	}

	// In order to make our test exit, we keep track of how many messages were
	// processed with wg, and cancel the receiveCtx when we've processed them all.
	// A more realistic application would not need this WaitGroup.
	var wg sync.WaitGroup
	wg.Add(nMessages)
	receiveCtx, cancel := context.WithCancel(ctx)
	go func() {
		wg.Wait()
		cancel()
	}()

	// Process messages using a traditional worker pool. Consider using an
	// inverted pool instead (see the other example).
	const poolSize = 10
	var workerWg sync.WaitGroup
	for n := 0; n < poolSize; n++ {
		workerWg.Add(1)
		go func() {
			for {
				// Read a message. Receive will block until a message is available.
				// It's fine to call Receive from many goroutines.
				msg, err := s.Receive(receiveCtx)
				if err != nil {
					// An error from Receive is fatal; Receive will never succeed again
					// so the application should exit.
					// In this example, we expect to get a error here when we've read all
					// the messages and receiveCtx is canceled.
					workerWg.Done()
					return
				}

				// Process the message and Ack it.
				msg.Ack()
				wg.Done()
			}
		}()
	}

	// Wait for all workers to finish.
	workerWg.Wait()
	fmt.Printf("Read %d messages", nMessages)

	// Output:
	// Read 100 messages
}
