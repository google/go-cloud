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

package gcppubsub

import (
	"context"
	"fmt"
	"testing"

	raw "cloud.google.com/go/pubsub/apiv1"
	"github.com/google/go-cloud/internal/pubsub"
	"github.com/google/go-cloud/internal/pubsub/driver"
	"github.com/google/go-cloud/internal/pubsub/drivertest"
	"github.com/google/go-cloud/internal/testing/setup"
	"google.golang.org/api/option"
	pb "google.golang.org/genproto/googleapis/pubsub/v1"
)

const (
	// These constants capture values that were used during the last -record.
	//
	// If you want to use --record mode,
	// 1a. Create a topic in your GCP project:
	//    https://console.cloud.google.com/cloudpubsub, then
	//    "Enable API", "Create a topic".
	// 1b. Create a subscription by clicking on the topic, then clicking on
	//    the icon at the top with a "Create subscription" tooltip.
	// 2. Update the topicName constant to your topic name, and the
	//    subscriptionName to your subscription name.
	topicName        = "test-topic"
	subscriptionName = "test-subscription-1"
	projectID        = "go-cloud-test-216917"
)

type harness struct {
	closer    func()
	pubClient *raw.PublisherClient
	subClient *raw.SubscriberClient
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	conn, done := setup.NewGCPgRPCConn(ctx, t, EndPoint)
	pubClient, err := raw.NewPublisherClient(ctx, option.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("making publisher client: %v", err)
	}
	subClient, err := raw.NewSubscriberClient(ctx, option.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("making subscription client: %v", err)
	}
	return &harness{done, pubClient, subClient}, nil
}

func (h *harness) MakeTopic(ctx context.Context) (driver.Topic, error) {
	dt := openTopic(ctx, h.pubClient, projectID, topicName)
	return dt, nil
}

func (h *harness) MakeNonexistentTopic(ctx context.Context) (driver.Topic, error) {
	dt := openTopic(ctx, h.pubClient, projectID, "nonexistent-topic")
	return dt, nil
}

func (h *harness) MakeSubscription(ctx context.Context, dt driver.Topic) (driver.Subscription, error) {
	ds := openSubscription(ctx, h.subClient, projectID, subscriptionName)
	return ds, nil
}

func (h *harness) MakeNonexistentSubscription(ctx context.Context) (driver.Subscription, error) {
	ds := openSubscription(ctx, h.subClient, projectID, "nonexistent-subscription")
	return ds, nil

}

func (h *harness) Close() {
	h.pubClient.Close()
	h.subClient.Close()
	h.closer()
}

func TestConformance(t *testing.T) {
	asTests := []drivertest.AsTest{gcpAsTest{}}
	drivertest.RunConformanceTests(t, newHarness, asTests)
}

type gcpAsTest struct{}

func (gcpAsTest) Name() string {
	return "gcp test"
}

func (gcpAsTest) TopicCheck(top *pubsub.Topic) error {
	var c2 raw.PublisherClient
	if top.As(&c2) {
		return fmt.Errorf("cast succeeded for %T, want failure", &c2)
	}
	var c3 *raw.PublisherClient
	if !top.As(&c3) {
		return fmt.Errorf("cast failed for %T", &c3)
	}
	return nil
}

func (gcpAsTest) SubscriptionCheck(sub *pubsub.Subscription) error {
	var c2 raw.SubscriberClient
	if sub.As(&c2) {
		return fmt.Errorf("cast succeeded for %T, want failure", &c2)
	}
	var c3 *raw.SubscriberClient
	if !sub.As(&c3) {
		return fmt.Errorf("cast failed for %T", &c3)
	}
	return nil
}

func (gcpAsTest) MessageCheck(top *pubsub.Topic, sub *pubsub.Subscription, m *pubsub.Message) error {
	var pm pb.PubsubMessage
	var ppm *pb.PubsubMessage
	if top.MessageAs(m, &ppm) {
		return fmt.Errorf("message check: top.MessageAs succeeded for %T, want failure", &ppm)
	}
	if !top.MessageAs(m, &pm) {
		return fmt.Errorf("message check: top.MessageAs failed for %T", &pm)
	}

	if sub.MessageAs(m, &ppm) {
		return fmt.Errorf("message check: sub.MessageAs succeeded for %T, want failure", &ppm)
	}
	if !sub.MessageAs(m, &pm) {
		return fmt.Errorf("message check: top.MessageAs failed for %T", &pm)
	}
	return nil
}
