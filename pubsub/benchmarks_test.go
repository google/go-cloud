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
	"errors"
	"flag"
	"os"
	"testing"

	"gocloud.dev/pubsub/azurepubsub"

	"gocloud.dev/gcp"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/gcppubsub"
	"golang.org/x/sync/errgroup"
)

var (
	projectID = flag.String("benchmark-project", "", "project ID")
	topicName = flag.String("benchmark-topic", "benchmark", "topic name")

	providerName = flag.String("benchmark-provider", "", "provider name")

	sbConnString = os.Getenv("SERVICEBUS_CONNECTION_STRING")	
)

func BenchmarkReceive(b *testing.B) {

	if *providerName == "" {
		b.Skip("missing -benchmark-provider, accepted values: gcp, asb")
	}

	attrs := map[string]string{"label": "value"}
	body := []byte("hello, world")
	const nMessages = 10000

	var topic *pubsub.Topic
	var sub *pubsub.Subscription
	var cleanup func()
	var err error

	switch *providerName {
	case "gcp":
		if *topicName == "" {
			b.Skip("missing -benchmark-project or -benchmark-topic")
		}
		topic, sub, cleanup, err = openGCPTopicAndSub()

	case "asb": // Use "asb" for Azure Service Bus pubsub provider.
		if sbConnString == "" {
			b.Skip("missing EnvironmentVariable SERVICEBUS_CONNECTION_STRING")
		}
		if *topicName == "" {
			b.Skip("missing -benchmark-topic")
		}
		topic, sub, cleanup, err = openAzureSbTopicAndSub()
	}

	if err != nil {
		b.Fatal(err)
	}
	defer cleanup()
	b.Run(*providerName, func(b *testing.B) {
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

func openAzureSbTopicAndSub() (*pubsub.Topic, *pubsub.Subscription, func(), error) {
	ctx := context.Background()

	t := azurepubsub.OpenTopic(ctx, *topicName, sbConnString, nil)

	// Use the topicName token for both SB Topic and Subscription Entities.
	sub, err := azurepubsub.OpenSubscription(ctx, *topicName, *topicName, sbConnString, nil)

	return t, sub, func() {}, err
}

func openGCPTopicAndSub() (*pubsub.Topic, *pubsub.Subscription, func(), error) {
	ctx := context.Background()
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	var projID gcp.ProjectID
	if *projectID != "" {
		projID = gcp.ProjectID(*projectID)
	} else {
		projID, err = gcp.DefaultProjectID(creds)
		if err != nil {
			return nil, nil, nil, err
		}
		if projID == "" {
			return nil, nil, nil, errors.New("could not get project ID")
		}
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
	topic := gcppubsub.OpenTopic(ctx, pc, projID, *topicName, nil)
	sub := gcppubsub.OpenSubscription(ctx, sc, projID, *topicName, nil)
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

// Call function f n times concurrently, using g goroutines. g must divide n.
// Wait until all calls complete. If any fail, cancel the remaining ones.
func runConcurrently(n, g int, f func(context.Context) error) error {
	gr, ctx := errgroup.WithContext(context.Background())
	ng := n / g
	for i := 0; i < g; i++ {
		gr.Go(func() error {
			for j := 0; j < ng; j++ {
				if err := f(ctx); err != nil {
					return err
				}
			}
			return nil
		})
	}
	return gr.Wait()

}
