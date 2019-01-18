// Copyright 2019 The Go Cloud Authors
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
	"flag"
	"testing"

	"gocloud.dev/gcp"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/gcppubsub"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

var (
	projectID = flag.String("benchmark-project", "", "project ID")
	topicName = flag.String("benchmark-topic", "", "topic name")
)

func BenchmarkReceive(b *testing.B) {
	if *projectID == "" || *topicName == "" {
		b.Skip("missing -benchmark-project or -benchmark-topic")
	}
	attrs := map[string]string{"label": "value"}
	body := []byte("hello, world")
	const nMessages = 10000
	topic, sub, cleanup, err := openGCPTopicAndSub()
	if err != nil {
		b.Fatal(err)
	}
	defer cleanup()
	b.Run("gcp", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			if err := publishN(topic, nMessages, 100, attrs, body); err != nil {
				b.Fatalf("publishing: %v", err)
			}
			b.Logf("published %d messages", nMessages)
			b.StartTimer()
			if err := receiveN(sub, nMessages, 1); err != nil {
				b.Fatalf("receiving: %v", err)
			}
			b.SetBytes(nMessages * 1e6)
			b.Log("MB/s is actually number of messages received per second")
		}
	})
}

func openGCPTopicAndSub() (*pubsub.Topic, *pubsub.Subscription, func(), error) {
	ctx := context.Background()
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	conn, cleanup, err := gcppubsub.Dial(ctx, gcp.CredentialsTokenSource(creds))
	if err != nil {
		return nil, nil, nil, err
	}
	pc, err := gcppubsub.PublisherClient(ctx, conn)
	if err != nil {
		cleanup()
		return nil, nil, nil, err
	}
	sc, err := gcppubsub.SubscriberClient(ctx, conn)
	if err != nil {
		cleanup()
		return nil, nil, nil, err
	}
	topic := gcppubsub.OpenTopic(ctx, pc, gcp.ProjectID(*projectID), *topicName, nil)
	sub := gcppubsub.OpenSubscription(ctx, sc, gcp.ProjectID(*projectID), *topicName, nil)
	return topic, sub, cleanup, nil
}

func publishN(topic *pubsub.Topic, nMessages, nGoroutines int, attrs map[string]string, body []byte) error {
	return runConcurrently(nMessages, nGoroutines, func(ctx context.Context) error {
		return topic.Send(ctx, &pubsub.Message{Metadata: attrs, Body: body})
	})
}

func receiveN(sub *pubsub.Subscription, nMessages, nGoroutines int) error {
	return runConcurrently(nMessages, nGoroutines, func(ctx context.Context) error {
		m, err := sub.Receive(ctx)
		if err == nil {
			m.Ack()
		}
		return err
	})
}

// Call function f n times concurrently, with a maximum number of maxg goroutines.
// Wait until all calls complete. If any fail, cancel the remaining ones.
func runConcurrently(n, maxg int, f func(context.Context) error) error {
	g, ctx := errgroup.WithContext(context.Background())
	sem := semaphore.NewWeighted(int64(maxg))
	for i := 0; i < n; i++ {
		if err := sem.Acquire(ctx, 1); err != nil {
			break // Don't return the context cancelation, return the error that caused it.
		}
		g.Go(func() error {
			defer sem.Release(1)
			return f(ctx)
		})
	}
	return g.Wait()
}
