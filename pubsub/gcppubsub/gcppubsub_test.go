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

package gcppubsub

import (
	"context"
	"fmt"
	"path"
	"strings"
	"sync/atomic"
	"testing"

	raw "cloud.google.com/go/pubsub/apiv1"
	"gocloud.dev/gcp"
	"gocloud.dev/internal/testing/setup"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
	"gocloud.dev/pubsub/drivertest"
	pubsubpb "google.golang.org/genproto/googleapis/pubsub/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// projectID is the project ID that was used during the last test run using
// --record.
const projectID = "go-cloud-test-216917"

type harness struct {
	closer    func()
	pubClient *raw.PublisherClient
	subClient *raw.SubscriberClient
	numTopics uint32 // atomic
	numSubs   uint32 // atomic
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
	// We may encounter topics that were created by a previous test run and were
	// not properly cleaned up. In such a case delete the existing topic and create
	// a new topic with a higher topic number (to avoid cool-off issues between
	// deletion and re-creation).
	for {
		topicName := fmt.Sprintf("%s-topic-%d", sanitize(testName), atomic.AddUint32(&h.numTopics, 1))
		topicPath := fmt.Sprintf("projects/%s/topics/%s", projectID, topicName)
		dt, cleanup, err := createTopic(ctx, h.pubClient, topicName, topicPath)
		if err != nil && status.Code(err) == codes.AlreadyExists {
			// Delete the topic and retry.
			h.pubClient.DeleteTopic(ctx, &pubsubpb.DeleteTopicRequest{Topic: topicPath})
			continue
		}
		return dt, cleanup, err
	}
}

func createTopic(ctx context.Context, pubClient *raw.PublisherClient, topicName, topicPath string) (dt driver.Topic, cleanup func(), err error) {
	_, err = pubClient.CreateTopic(ctx, &pubsubpb.Topic{Name: topicPath})
	if err != nil {
		return nil, nil, err
	}
	dt = openTopic(pubClient, path.Join("projects", projectID, "topics", topicName))
	cleanup = func() {
		pubClient.DeleteTopic(ctx, &pubsubpb.DeleteTopicRequest{Topic: topicPath})
	}
	return dt, cleanup, nil
}

func (h *harness) MakeNonexistentTopic(ctx context.Context) (driver.Topic, error) {
	return openTopic(h.pubClient, path.Join("projects", projectID, "topics", "nonexistent-topic")), nil
}

func (h *harness) CreateSubscription(ctx context.Context, dt driver.Topic, testName string) (ds driver.Subscription, cleanup func(), err error) {
	// We may encounter subscriptions that were created by a previous test run
	// and were not properly cleaned up. In such a case delete the existing
	// subscription and create a new subscription with a higher subscription
	// number (to avoid cool-off issues between deletion and re-creation).
	for {
		subName := fmt.Sprintf("%s-subscription-%d", sanitize(testName), atomic.AddUint32(&h.numSubs, 1))
		subPath := fmt.Sprintf("projects/%s/subscriptions/%s", projectID, subName)
		ds, cleanup, err := createSubscription(ctx, h.subClient, dt, subName, subPath)
		if err != nil && status.Code(err) == codes.AlreadyExists {
			// Delete the subscription and retry.
			h.subClient.DeleteSubscription(ctx, &pubsubpb.DeleteSubscriptionRequest{Subscription: subPath})
			continue
		}
		return ds, cleanup, err
	}
}

func createSubscription(ctx context.Context, subClient *raw.SubscriberClient, dt driver.Topic, subName, subPath string) (ds driver.Subscription, cleanup func(), err error) {
	t := dt.(*topic)
	_, err = subClient.CreateSubscription(ctx, &pubsubpb.Subscription{
		Name:  subPath,
		Topic: t.path,
	})
	if err != nil {
		return nil, nil, err
	}
	ds = openSubscription(subClient, path.Join("projects", projectID, "subscriptions", subName))
	cleanup = func() {
		subClient.DeleteSubscription(ctx, &pubsubpb.DeleteSubscriptionRequest{Subscription: subPath})
	}
	return ds, cleanup, nil
}

func (h *harness) MakeNonexistentSubscription(ctx context.Context) (driver.Subscription, error) {
	return openSubscription(h.subClient, path.Join("projects", projectID, "subscriptions", "nonexistent-subscription")), nil
}

func (h *harness) Close() {
	h.pubClient.Close()
	h.subClient.Close()
	h.closer()
}

func (h *harness) MaxBatchSizes() (int, int) {
	return sendBatcherOpts.MaxBatchSize, ackBatcherOpts.MaxBatchSize
}

func (*harness) SupportsMultipleSubscriptions() bool { return true }

func TestConformance(t *testing.T) {
	asTests := []drivertest.AsTest{gcpAsTest{}}
	drivertest.RunConformanceTests(t, newHarness, asTests)
}

func BenchmarkGcpPubSub(b *testing.B) {
	ctx := context.Background()
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		b.Fatal(err)
	}

	// Connect.
	conn, cleanup, err := Dial(ctx, gcp.CredentialsTokenSource(creds))
	if err != nil {
		b.Fatal(err)
	}
	defer cleanup()

	// Make topic.
	pc, err := PublisherClient(ctx, conn)
	if err != nil {
		b.Fatal(err)
	}
	topicName := fmt.Sprintf("%s-topic", b.Name())
	topicPath := fmt.Sprintf("projects/%s/topics/%s", projectID, topicName)
	dt, cleanup1, err := createTopic(ctx, pc, topicName, topicPath)
	if err != nil {
		b.Fatal(err)
	}
	defer cleanup1()
	topic := pubsub.NewTopic(dt, nil)
	defer topic.Shutdown(ctx)

	// Make subscription.
	sc, err := SubscriberClient(ctx, conn)
	if err != nil {
		b.Fatal(err)
	}
	subName := fmt.Sprintf("%s-subscription", b.Name())
	subPath := fmt.Sprintf("projects/%s/subscriptions/%s", projectID, subName)
	ds, cleanup2, err := createSubscription(ctx, sc, dt, subName, subPath)
	if err != nil {
		b.Fatal(err)
	}
	defer cleanup2()
	sub := pubsub.NewSubscription(ds, recvBatcherOpts, ackBatcherOpts)
	defer sub.Shutdown(ctx)

	drivertest.RunBenchmarks(b, topic, sub)
}

type gcpAsTest struct{}

func (gcpAsTest) Name() string {
	return "gcp test"
}

func (gcpAsTest) TopicCheck(topic *pubsub.Topic) error {
	var c2 raw.PublisherClient
	if topic.As(&c2) {
		return fmt.Errorf("cast succeeded for %T, want failure", &c2)
	}
	var c3 *raw.PublisherClient
	if !topic.As(&c3) {
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

func (gcpAsTest) TopicErrorCheck(t *pubsub.Topic, err error) error {
	var s *status.Status
	if !t.ErrorAs(err, &s) {
		return fmt.Errorf("failed to convert %v (%T) to a gRPC Status", err, err)
	}
	if s.Code() != codes.NotFound {
		return fmt.Errorf("got code %s, want NotFound", s.Code())
	}
	return nil
}

func (gcpAsTest) SubscriptionErrorCheck(sub *pubsub.Subscription, err error) error {
	var s *status.Status
	if !sub.ErrorAs(err, &s) {
		return fmt.Errorf("failed to convert %v (%T) to a gRPC Status", err, err)
	}
	if s.Code() != codes.NotFound {
		return fmt.Errorf("got code %s, want NotFound", s.Code())
	}
	return nil
}

func (gcpAsTest) MessageCheck(m *pubsub.Message) error {
	var pm pubsubpb.PubsubMessage
	if m.As(&pm) {
		return fmt.Errorf("cast succeeded for %T, want failure", &pm)
	}
	var ppm *pubsubpb.PubsubMessage
	if !m.As(&ppm) {
		return fmt.Errorf("cast failed for %T", &ppm)
	}
	return nil
}

func (gcpAsTest) BeforeSend(as func(interface{}) bool) error {
	var ppm *pubsubpb.PubsubMessage
	if !as(&ppm) {
		return fmt.Errorf("cast failed for %T", &ppm)
	}
	return nil
}

func sanitize(testName string) string {
	return strings.Replace(testName, "/", "_", -1)
}

func TestOpenTopic(t *testing.T) {
	ctx := context.Background()
	creds, err := setup.FakeGCPCredentials(ctx)
	if err != nil {
		t.Fatal(err)
	}
	projID, err := gcp.DefaultProjectID(creds)
	if err != nil {
		t.Fatal(err)
	}
	conn, cleanup, err := Dial(ctx, gcp.CredentialsTokenSource(creds))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	pc, err := PublisherClient(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}
	topic := OpenTopic(pc, projID, "my-topic", nil)
	defer topic.Shutdown(ctx)
	err = topic.Send(ctx, &pubsub.Message{Body: []byte("hello world")})
	if err == nil {
		t.Error("got nil, want error")
	}

	// Repeat with OpenTopicByPath.
	topic, err = OpenTopicByPath(pc, path.Join("projects", string(projID), "topics", "my-topic"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer topic.Shutdown(ctx)
	err = topic.Send(ctx, &pubsub.Message{Body: []byte("hello world")})
	if err == nil {
		t.Error("got nil, want error")
	}

	// Try an invalid path.
	_, err = OpenTopicByPath(pc, "my-topic", nil)
	if err == nil {
		t.Error("got nil, want error")
	}
}

func TestOpenSubscription(t *testing.T) {
	ctx := context.Background()
	creds, err := setup.FakeGCPCredentials(ctx)
	if err != nil {
		t.Fatal(err)
	}
	projID, err := gcp.DefaultProjectID(creds)
	if err != nil {
		t.Fatal(err)
	}
	conn, cleanup, err := Dial(ctx, gcp.CredentialsTokenSource(creds))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	sc, err := SubscriberClient(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}
	sub := OpenSubscription(sc, projID, "my-subscription", nil)
	defer sub.Shutdown(ctx)
	_, err = sub.Receive(ctx)
	if err == nil {
		t.Error("got nil, want error")
	}

	// Repeat with OpenSubscriptionByPath.
	sub, err = OpenSubscriptionByPath(sc, path.Join("projects", string(projID), "subscriptions", "my-subscription"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Shutdown(ctx)
	_, err = sub.Receive(ctx)
	if err == nil {
		t.Error("got nil, want error")
	}

	// Try an invalid path.
	_, err = OpenSubscriptionByPath(sc, "my-subscription", nil)
	if err == nil {
		t.Error("got nil, want error")
	}
}

func TestOpenTopicFromURL(t *testing.T) {
	cleanup := setup.FakeGCPDefaultCredentials(t)
	defer cleanup()

	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK, short form.
		{"gcppubsub://myproject/mytopic", false},
		// OK, long form.
		{"gcppubsub://projects/myproject/topic/mytopic", false},
		// Invalid parameter.
		{"gcppubsub://myproject/mytopic?param=value", true},
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
	cleanup := setup.FakeGCPDefaultCredentials(t)
	defer cleanup()

	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK, short form.
		{"gcppubsub://myproject/mysub", false},
		// OK, long form.
		{"gcppubsub://projects/myproject/subscriptions/mysub", false},
		// Invalid parameter.
		{"gcppubsub://myproject/mysub?param=value", true},
	}

	ctx := context.Background()
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
