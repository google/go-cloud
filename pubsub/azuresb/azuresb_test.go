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
package azuresb

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"gocloud.dev/internal/testing/setup"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
	"gocloud.dev/pubsub/drivertest"

	"github.com/Azure/azure-amqp-common-go"
	"github.com/Azure/azure-service-bus-go"
)

var (
	// See docs below on how to provision an Azure Service Bus Namespace and obtaining the connection string.
	// https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-dotnet-get-started-with-queues
	connString = os.Getenv("SERVICEBUS_CONNECTION_STRING")
)

const (
	nonexistentTopicName = "nonexistent-topic"
)

type harness struct {
	ns         *servicebus.Namespace
	numTopics  uint32 // atomic
	numSubs    uint32 // atomic
	closer     func()
	autodelete bool
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	if connString == "" {
		return nil, fmt.Errorf("azuresb: test harness requires environment variable SERVICEBUS_CONNECTION_STRING to run")
	}
	ns, err := NewNamespaceFromConnectionString(connString)
	if err != nil {
		return nil, err
	}
	noop := func() {}
	return &harness{
		ns:     ns,
		closer: noop,
	}, nil
}

func newHarnessUsingAutodelete(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	h, err := newHarness(ctx, t)
	if err == nil {
		h.(*harness).autodelete = true
	}
	return h, err
}

func (h *harness) CreateTopic(ctx context.Context, testName string) (dt driver.Topic, cleanup func(), err error) {
	// Keep the topic entity name under 50 characters as per Azure limits.
	// See https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-quotas
	topicName := fmt.Sprintf("%s-topic-%d", sanitize(testName), atomic.AddUint32(&h.numTopics, 1))
	if len(topicName) > 50 {
		topicName = topicName[:50]
	}

	createTopic(ctx, topicName, h.ns, nil)

	sbTopic, err := NewTopic(h.ns, topicName, nil)
	dt, err = openTopic(ctx, sbTopic, nil)
	if err != nil {
		return nil, nil, err
	}

	cleanup = func() {
		sbTopic.Close(ctx)
		deleteTopic(ctx, topicName, h.ns)
	}
	return dt, cleanup, nil
}

func (h *harness) MakeNonexistentTopic(ctx context.Context) (driver.Topic, error) {
	sbTopic, err := NewTopic(h.ns, nonexistentTopicName, nil)
	if err != nil {
		return nil, err
	}
	return openTopic(ctx, sbTopic, nil)
}

func (h *harness) CreateSubscription(ctx context.Context, dt driver.Topic, testName string) (ds driver.Subscription, cleanup func(), err error) {
	// Keep the subscription entity name under 50 characters as per Azure limits.
	// See https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-quotas
	// There also appears to be some overhead to that when using AMQP; topic
	// names of length 50 with a subscription of length 50 fail; 50+40 works.
	subName := fmt.Sprintf("%s-sub-%d", sanitize(testName), atomic.AddUint32(&h.numSubs, 1))
	if len(subName) > 40 {
		subName = subName[:40]
	}

	t := dt.(*topic)

	err = createSubscription(ctx, t.sbTopic.Name, subName, h.ns, nil)
	if err != nil {
		return nil, nil, err
	}

	var opts []servicebus.SubscriptionOption
	if h.autodelete {
		opts = append(opts, servicebus.SubscriptionWithReceiveAndDelete())
	}
	sbSub, err := NewSubscription(t.sbTopic, subName, opts)
	if err != nil {
		return nil, nil, err
	}

	sopts := SubscriptionOptions{}
	if h.autodelete {
		sopts.AckFuncForReceiveAndDelete = func() {}
	}
	ds, err = openSubscription(ctx, h.ns, t.sbTopic, sbSub, &sopts)
	if err != nil {
		return nil, nil, err
	}

	cleanup = func() {
		sbSub.Close(ctx)
		deleteSubscription(ctx, t.sbTopic.Name, subName, h.ns)
	}
	return ds, cleanup, nil
}

func (h *harness) MakeNonexistentSubscription(ctx context.Context) (driver.Subscription, error) {
	sbTopic, _ := NewTopic(h.ns, nonexistentTopicName, nil)
	sbSub, _ := NewSubscription(sbTopic, "nonexistent-subscription", nil)
	return openSubscription(ctx, h.ns, sbTopic, sbSub, nil)
}

func (h *harness) Close() {
	h.closer()
}

func (h *harness) MaxBatchSizes() (int, int) { return sendBatcherOpts.MaxBatchSize, 0 }

// Please run the TestConformance with an extended timeout since each test needs to perform CRUD for ServiceBus Topics and Subscriptions.
// Example: C:\Go\bin\go.exe test -timeout 60s gocloud.dev/pubsub/azuresb -run ^TestConformance$
func TestConformance(t *testing.T) {
	if !*setup.Record {
		t.Skip("replaying is not yet supported for Azure pubsub")
	}
	asTests := []drivertest.AsTest{sbAsTest{}}
	drivertest.RunConformanceTests(t, newHarness, asTests)
}

/* Disabled for now, as the tests do not pass.
func TestConformanceWithAutodelete(t *testing.T) {
	if !*setup.Record {
		t.Skip("replaying is not yet supported for Azure pubsub")
	}
	asTests := []drivertest.AsTest{sbAsTest{}}
	drivertest.RunConformanceTests(t, newHarnessUsingAutodelete, asTests)
}
*/

type sbAsTest struct{}

func (sbAsTest) Name() string {
	return "azure"
}

func (sbAsTest) TopicCheck(top *pubsub.Topic) error {
	var t2 servicebus.Topic
	if top.As(&t2) {
		return fmt.Errorf("cast succeeded for %T, want failure", &t2)
	}
	var t3 *servicebus.Topic
	if !top.As(&t3) {
		return fmt.Errorf("cast failed for %T", &t3)
	}
	return nil
}

func (sbAsTest) SubscriptionCheck(sub *pubsub.Subscription) error {
	var s2 servicebus.Subscription
	if sub.As(&s2) {
		return fmt.Errorf("cast succeeded for %T, want failure", &s2)
	}
	var s3 *servicebus.Subscription
	if !sub.As(&s3) {
		return fmt.Errorf("cast failed for %T", &s3)
	}
	return nil
}

func (sbAsTest) TopicErrorCheck(t *pubsub.Topic, err error) error {
	var sbError common.Retryable
	if !t.ErrorAs(err, &sbError) {
		return fmt.Errorf("failed to convert %v (%T) to a common.Retryable", err, err)
	}
	return nil
}

func (sbAsTest) SubscriptionErrorCheck(s *pubsub.Subscription, err error) error {
	// We generate our own error for non-existent subscription, so there's no
	// underlying Azure error type.
	return nil
}

func (sbAsTest) MessageCheck(m *pubsub.Message) error {
	var m2 servicebus.Message
	if m.As(&m2) {
		return fmt.Errorf("cast succeeded for %T, want failure", &m2)
	}
	var m3 *servicebus.Message
	if !m.As(&m3) {
		return fmt.Errorf("cast failed for %T", &m3)
	}
	return nil
}

func sanitize(testName string) string {
	return strings.Replace(testName, "/", "_", -1)
}

// createTopic ensures the existence of a Service Bus Topic on a given Namespace.
func createTopic(ctx context.Context, topicName string, ns *servicebus.Namespace, opts []servicebus.TopicManagementOption) error {
	tm := ns.NewTopicManager()
	_, err := tm.Get(ctx, topicName)
	if err == nil {
		_ = tm.Delete(ctx, topicName)
	}
	_, err = tm.Put(ctx, topicName, opts...)
	return err
}

// deleteTopic removes a Service Bus Topic on a given Namespace.
func deleteTopic(ctx context.Context, topicName string, ns *servicebus.Namespace) error {
	tm := ns.NewTopicManager()
	te, _ := tm.Get(ctx, topicName)
	if te != nil {
		return tm.Delete(ctx, topicName)
	}
	return nil
}

// createSubscription ensures the existence of a Service Bus Subscription on a given Namespace and Topic.
func createSubscription(ctx context.Context, topicName string, subscriptionName string, ns *servicebus.Namespace, opts []servicebus.SubscriptionManagementOption) error {
	sm, err := ns.NewSubscriptionManager(topicName)
	if err != nil {
		return err
	}
	_, err = sm.Get(ctx, subscriptionName)
	if err == nil {
		_ = sm.Delete(ctx, subscriptionName)
	}
	_, err = sm.Put(ctx, subscriptionName, opts...)
	return err
}

// deleteSubscription removes a Service Bus Subscription on a given Namespace and Topic.
func deleteSubscription(ctx context.Context, topicName string, subscriptionName string, ns *servicebus.Namespace) error {
	sm, err := ns.NewSubscriptionManager(topicName)
	if err != nil {
		return nil
	}
	se, _ := sm.Get(ctx, subscriptionName)
	if se != nil {
		_ = sm.Delete(ctx, subscriptionName)
	}
	return nil
}

func BenchmarkAzureServiceBusPubSub(b *testing.B) {
	const (
		benchmarkTopicName        = "benchmark-topic"
		benchmarkSubscriptionName = "benchmark-subscription"
	)
	ctx := context.Background()

	if connString == "" {
		b.Fatal("azuresb: benchmark requires environment variable SERVICEBUS_CONNECTION_STRING to run")
	}
	ns, err := NewNamespaceFromConnectionString(connString)
	if err != nil {
		b.Fatal(err)
	}

	// Make topic.
	if err := createTopic(ctx, benchmarkTopicName, ns, nil); err != nil {
		b.Fatal(err)
	}
	defer deleteTopic(ctx, benchmarkTopicName, ns)

	sbTopic, err := NewTopic(ns, benchmarkTopicName, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer sbTopic.Close(ctx)
	topic, err := OpenTopic(ctx, sbTopic, nil)
	if err != nil {
		b.Fatal(err)
	}

	// Make subscription.
	if err := createSubscription(ctx, benchmarkTopicName, benchmarkSubscriptionName, ns, nil); err != nil {
		b.Fatal(err)
	}
	sbSub, err := NewSubscription(sbTopic, benchmarkSubscriptionName, nil)
	if err != nil {
		b.Fatal(err)
	}
	sub, err := OpenSubscription(ctx, ns, sbTopic, sbSub, nil)
	if err != nil {
		b.Fatal(err)
	}

	drivertest.RunBenchmarks(b, topic, sub)
}

func fakeConnectionStringInEnv() func() {
	oldEnvVal := os.Getenv("SERVICEBUS_CONNECTION_STRING")
	os.Setenv("SERVICEBUS_CONNECTION_STRING", "Endpoint=sb://foo.servicebus.windows.net/;SharedAccessKeyName=RootManageSharedAccessKey;SharedAccessKey=mykey")
	return func() {
		os.Setenv("SERVICEBUS_CONNECTION_STRING", oldEnvVal)
	}
}

func TestOpenTopicFromURL(t *testing.T) {
	cleanup := fakeConnectionStringInEnv()
	defer cleanup()

	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"azuresb://mytopic", false},
		// Invalid parameter.
		{"azuresb://mytopic?param=value", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		_, err := pubsub.OpenTopic(ctx, test.URL)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
	}
}

func TestOpenSubscriptionFromURL(t *testing.T) {
	cleanup := fakeConnectionStringInEnv()
	defer cleanup()

	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"azuresb://mytopic?subscription=mysub", false},
		// Missing subscription.
		{"azuresb://mytopic", true},
		// Invalid parameter.
		{"azuresb://mytopic?subscription=mysub&param=value", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		_, err := pubsub.OpenSubscription(ctx, test.URL)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
	}
}
