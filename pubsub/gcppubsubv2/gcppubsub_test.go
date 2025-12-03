// Copyright 2025 The Go Cloud Development Kit Authors
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

package gcppubsubv2

import (
	"context"
	"fmt"
	"path"
	"strings"
	"sync/atomic"
	"testing"

	raw "cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"gocloud.dev/gcp"
	"gocloud.dev/internal/testing/setup"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
	"gocloud.dev/pubsub/drivertest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// projectID is the project ID that was used during the last test run using
// --record.
const projectID = "go-cloud-test-216917"

type harness struct {
	closer    func()
	client    *raw.Client
	numTopics uint32 // atomic
	numSubs   uint32 // atomic
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	t.Helper()

	client, err := Client(ctx, projectID, nil)
	if err != nil {
		return nil, fmt.Errorf("making client: %v", err)
	}
	return &harness{closer: func() {}, client: client, numTopics: 0, numSubs: 0}, nil
}

func (h *harness) CreateTopic(ctx context.Context, testName string) (dt driver.Topic, cleanup func(), err error) {
	topicName := fmt.Sprintf("%s-topic-%d", sanitize(testName), atomic.AddUint32(&h.numTopics, 1))
	return createTopic(ctx, h.client, topicName)
}

func topicPath(topicName string) string {
	return fmt.Sprintf("projects/%s/topics/%s", projectID, topicName)
}
func subscriptionPath(subName string) string {
	return fmt.Sprintf("projects/%s/subscriptions/%s", projectID, subName)
}

func createTopic(ctx context.Context, client *raw.Client, topicName string) (dt driver.Topic, cleanup func(), err error) {
	_, err = client.TopicAdminClient.CreateTopic(ctx, &pubsubpb.Topic{Name: topicPath(topicName)})
	// We may encounter topics that were created by a previous test run and were
	// not properly cleaned up. In such a case delete the existing topic and create
	// a new topic with a higher topic number (to avoid cool-off issues between
	// deletion and re-creation).
	if err != nil && status.Code(err) == codes.AlreadyExists {
		deleteTopic(ctx, client, topicName)
		return createTopic(ctx, client, topicName)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create topic: %w", err)
	}
	dt = openTopic(client.Publisher(topicName))
	cleanup = func() {
		deleteTopic(ctx, client, topicName)
	}
	return dt, cleanup, nil
}

func deleteTopic(ctx context.Context, client *raw.Client, topicName string) {
	_ = client.TopicAdminClient.DeleteTopic(ctx, &pubsubpb.DeleteTopicRequest{
		Topic: topicPath(topicName),
	})
}

func (h *harness) MakeNonexistentTopic(ctx context.Context) (driver.Topic, error) {
	return openTopic(h.client.Publisher("nonexistent-topic")), nil
}

func (h *harness) CreateSubscription(ctx context.Context, dt driver.Topic, testName string) (ds driver.Subscription, cleanup func(), err error) {
	subName := fmt.Sprintf("%s-subscription-%d", sanitize(testName), atomic.AddUint32(&h.numSubs, 1))
	return createSubscription(ctx, h.client, dt, subName)
}

func createSubscription(ctx context.Context, client *raw.Client, dt driver.Topic, subName string) (ds driver.Subscription, cleanup func(), err error) {
	t := dt.(*topic)
	_, err = client.SubscriptionAdminClient.CreateSubscription(ctx, &pubsubpb.Subscription{
		Name:  subscriptionPath(subName),
		Topic: t.publisher.String(),
	})
	// We may encounter subscriptions that were created by a previous test run
	// and were not properly cleaned up. In such a case delete the existing
	// subscription and create a new subscription with a higher subscription
	// number (to avoid cool-off issues between deletion and re-creation).
	if err != nil && status.Code(err) == codes.AlreadyExists {
		deleteSubscription(ctx, client, subName)
		return createSubscription(ctx, client, dt, subName)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create subscription: %w", err)
	}
	ds = openSubscription(client.Subscriber(subName), nil)
	cleanup = func() {
		deleteSubscription(ctx, client, subName)
	}
	return ds, cleanup, nil
}

func deleteSubscription(ctx context.Context, client *raw.Client, subName string) {
	_ = client.SubscriptionAdminClient.DeleteSubscription(ctx, &pubsubpb.DeleteSubscriptionRequest{
		Subscription: subscriptionPath(subName)},
	)
}

func (h *harness) MakeNonexistentSubscription(ctx context.Context) (driver.Subscription, func(), error) {
	return openSubscription(h.client.Subscriber("nonexistent-subscription"), nil), func() {}, nil
}

func (h *harness) Close() {
	h.client.Close()
	h.closer()
}

func (h *harness) MaxBatchSizes() (int, int) {
	return sendBatcherOpts.MaxBatchSize, ackBatcherOpts.MaxBatchSize
}

func (*harness) SupportsMultipleSubscriptions() bool { return true }

func TestConformance(t *testing.T) {
	if !*setup.Record {
		t.Skip("replaying is not supported for gcppubsubv2 because it uses a streaming RPC internally")
	}
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
	client, err := Client(ctx, projectID, conn)
	if err != nil {
		b.Fatal(err)
	}
	topicName := fmt.Sprintf("%s-topic", b.Name())
	dt, cleanup1, err := createTopic(ctx, client, topicName)
	if err != nil {
		b.Fatal(err)
	}
	defer cleanup1()
	topic := pubsub.NewTopic(dt, nil)
	defer topic.Shutdown(ctx)

	// Make subscription.
	subName := fmt.Sprintf("%s-subscription", b.Name())
	ds, cleanup2, err := createSubscription(ctx, client, dt, subName)
	if err != nil {
		b.Fatal(err)
	}
	defer cleanup2()
	sub := pubsub.NewSubscription(ds, defaultRecvBatcherOpts, ackBatcherOpts)
	defer sub.Shutdown(ctx)

	drivertest.RunBenchmarks(b, topic, sub)
}

type gcpAsTest struct{}

func (gcpAsTest) Name() string {
	return "gcp test"
}

func (gcpAsTest) TopicCheck(topic *pubsub.Topic) error {
	var c2 raw.Publisher
	if topic.As(&c2) {
		return fmt.Errorf("cast succeeded for %T, want failure", &c2)
	}
	var c3 *raw.Publisher
	if !topic.As(&c3) {
		return fmt.Errorf("cast failed for %T", &c3)
	}
	return nil
}

func (gcpAsTest) SubscriptionCheck(sub *pubsub.Subscription) error {
	var c2 raw.Subscriber
	if sub.As(&c2) {
		return fmt.Errorf("cast succeeded for %T, want failure", &c2)
	}
	var c3 *raw.Subscriber
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
	var rm raw.Message
	if m.As(&rm) {
		return fmt.Errorf("cast succeeded for %T, want failure", &rm)
	}
	var prm *raw.Message
	if !m.As(&prm) {
		return fmt.Errorf("cast failed for %T", &prm)
	}
	return nil
}

func (gcpAsTest) BeforeSend(as func(any) bool) error {
	var prm *raw.Message
	if !as(&prm) {
		return fmt.Errorf("cast failed for %T", &prm)
	}
	return nil
}

func (gcpAsTest) AfterSend(as func(any) bool) error {
	var msgId string
	if !as(&msgId) {
		return fmt.Errorf("cast failed for %T", &msgId)
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
	client, err := Client(ctx, projID, conn)
	if err != nil {
		t.Fatal(err)
	}
	topic := OpenTopic(client, "my-topic", nil)
	defer topic.Shutdown(ctx)
	err = topic.Send(ctx, &pubsub.Message{Body: []byte("hello world")})
	if err == nil {
		t.Error("got nil, want error")
	}

	// Repeat with OpenTopicByPath.
	topic, err = OpenTopicByPath(client, path.Join("projects", string(projID), "topics", "my-topic"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer topic.Shutdown(ctx)
	err = topic.Send(ctx, &pubsub.Message{Body: []byte("hello world")})
	if err == nil {
		t.Error("got nil, want error")
	}

	// Try an invalid path.
	_, err = OpenTopicByPath(client, "my-topic", nil)
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
	client, err := Client(ctx, projID, conn)
	if err != nil {
		t.Fatal(err)
	}
	sub := OpenSubscription(client, "my-subscription", nil)
	defer sub.Shutdown(ctx)
	_, err = sub.Receive(ctx)
	if err == nil {
		t.Error("got nil, want error")
	}

	// Repeat with OpenSubscriptionByPath.
	sub, err = OpenSubscriptionByPath(client, path.Join("projects", string(projID), "subscriptions", "my-subscription"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Shutdown(ctx)
	_, err = sub.Receive(ctx)
	if err == nil {
		t.Error("got nil, want error")
	}

	// Try an invalid path.
	_, err = OpenSubscriptionByPath(client, "my-subscription", nil)
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
		{"gcppubsubv2://myproject/mytopic", false},
		// OK, long form.
		{"gcppubsubv2://projects/myproject/topic/mytopic", false},
		// Invalid parameter.
		{"gcppubsubv2://myproject/mytopic?param=value", true},
		// Valid max_send_batch_size
		{"gcppubsubv2://projects/mytopic?max_send_batch_size=1", false},
		// Invalid max_send_batch_size
		{"gcppubsubv2://projects/mytopic?max_send_batch_size=0", true},
		// Invalid max_send_batch_size
		{"gcppubsubv2://projects/mytopic?max_send_batch_size=1001", true},
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
		{"gcppubsubv2://myproject/mysub", false},
		// OK, long form.
		{"gcppubsubv2://projects/myproject/subscriptions/mysub", false},
		// Invalid parameter.
		{"gcppubsubv2://myproject/mysub?param=value", true},
		// Valid max_recv_batch_size
		{"gcppubsubv2://projects/myproject/subscriptions/mysub?max_recv_batch_size=1", false},
		// Invalid max_recv_batch_size
		{"gcppubsubv2://projects/myproject/subscriptions/mysub?max_recv_batch_size=0", true},
		// Invalid max_recv_batch_size
		{"gcppubsubv2://projects/myproject/subscriptions/mysub?max_recv_batch_size=1001", true},
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
