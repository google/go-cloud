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
	"strings"
	"sync/atomic"
	"testing"

	raw "cloud.google.com/go/pubsub/apiv1"
	"gocloud.dev/internal/testing/setup"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
	"gocloud.dev/pubsub/drivertest"
	pubsubpb "google.golang.org/genproto/googleapis/pubsub/v1"
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
	// 1c. Create a second subscription the same way.
	// 2. Update the topicName constant to your topic name, and the
	//    subscriptionName0 and subscriptionName1 constants to your
	//    subscription names.
	topicName         = "test-topic"
	subscriptionName0 = "test-subscription-1"
	subscriptionName1 = "test-subscription-2"
	projectID         = "go-cloud-test-216917"
)

type harness struct {
	closer    func()
	pubClient *raw.PublisherClient
	subClient *raw.SubscriberClient
	numTopics uint32
	numSubs   uint32
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	conn, done := setup.NewGCPgRPCConn(ctx, t, endPoint, "pubsub")
	pubClient, err := PublisherClient(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("making publisher client: %v", err)
	}
	subClient, err := SubscriberClient(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("making subscription client: %v", err)
	}
	return &harness{closer: done, pubClient: pubClient, subClient: subClient, numTopics: 0, numSubs: 0}, nil
}

func (h *harness) CreateTopic(ctx context.Context, testName string) (dt driver.Topic, cleanup func(), err error) {
	topicName := fmt.Sprintf("%s-topic-%d", sanitize(testName), atomic.AddUint32(&h.numTopics, 1))
	topicPath := fmt.Sprintf("projects/%s/topics/%s", projectID, topicName)
	_, err = h.pubClient.CreateTopic(ctx, &pubsubpb.Topic{Name: topicPath})
	if err != nil {
		return nil, nil, fmt.Errorf("creating topic: %v", err)
	}
	dt = openTopic(ctx, h.pubClient, projectID, topicName)
	cleanup = func() {
		h.pubClient.DeleteTopic(ctx, &pubsubpb.DeleteTopicRequest{Topic: topicPath})
	}
	return dt, cleanup, nil
}

func (h *harness) MakeNonexistentTopic(ctx context.Context) (driver.Topic, error) {
	dt := openTopic(ctx, h.pubClient, projectID, "nonexistent-topic")
	return dt, nil
}

func (h *harness) CreateSubscription(ctx context.Context, dt driver.Topic, testName string) (ds driver.Subscription, cleanup func(), err error) {
	subName := fmt.Sprintf("%s-subscription-%d", sanitize(testName), atomic.AddUint32(&h.numSubs, 1))
	subPath := fmt.Sprintf("projects/%s/subscriptions/%s", projectID, subName)
	t := dt.(*topic)
	_, err = h.subClient.CreateSubscription(ctx, &pubsubpb.Subscription{
		Name:  subPath,
		Topic: t.path,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("creating subscription: %v", err)
	}
	ds = openSubscription(ctx, h.subClient, projectID, subName)
	cleanup = func() {
		h.subClient.DeleteSubscription(ctx, &pubsubpb.DeleteSubscriptionRequest{Subscription: subPath})
	}
	return ds, cleanup, nil
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

func sanitize(testName string) string {
	return strings.Replace(testName, "/", "_", -1)
}
