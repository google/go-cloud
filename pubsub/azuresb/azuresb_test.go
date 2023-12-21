// Copyright 2018 The Go Cloud Development Kit Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	https://www.apache.org/licenses/LICENSE-2.0
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

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	servicebus "github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus/admin"
)

var (
	// See docs below on how to provision an Azure Service Bus Namespace and obtaining the connection string.
	// https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-dotnet-get-started-with-queues
	connString = os.Getenv("SERVICEBUS_CONNECTION_STRING")
	sbHostname = os.Getenv("AZURE_SERVICEBUS_HOSTNAME")
)

const (
	nonexistentTopicName = "nonexistent-topic"

	// Try to keep the entity name under Azure limits.
	// https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-quotas
	// says 50, but there appears to be some additional overhead. 40 works.
	maxNameLen = 40
)

type harness struct {
	adminClient *admin.Client
	sbClient    *servicebus.Client
	numTopics   uint32 // atomic
	numSubs     uint32 // atomic
	closer      func()
	autodelete  bool
	topics      map[driver.Topic]string
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	if connString == "" && sbHostname == "" {
		return nil, fmt.Errorf("azuresb: test harness requires environment variable SERVICEBUS_CONNECTION_STRING or AZURE_SERVICEBUS_HOSTNAME to run")
	}
	adminClient, err := admin.NewClientFromConnectionString(connString, nil)
	if err != nil {
		return nil, err
	}
	sbClient, err := NewClientFromConnectionString(connString, nil)
	if err != nil {
		return nil, err
	}
	noop := func() {}
	return &harness{
		adminClient: adminClient,
		sbClient:    sbClient,
		closer:      noop,
		topics:      map[driver.Topic]string{},
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
	topicName := sanitize(fmt.Sprintf("%s-top-%d", testName, atomic.AddUint32(&h.numTopics, 1)))
	if err := createTopic(ctx, topicName, h.adminClient, nil); err != nil {
		return nil, nil, err
	}

	sbSender, err := NewSender(h.sbClient, topicName, nil)
	dt, err = openTopic(ctx, sbSender, nil)
	if err != nil {
		return nil, nil, err
	}
	h.topics[dt] = topicName
	cleanup = func() {
		sbSender.Close(ctx)
		deleteTopic(ctx, topicName, h.adminClient)
	}
	return dt, cleanup, nil
}

func (h *harness) MakeNonexistentTopic(ctx context.Context) (driver.Topic, error) {
	sbSender, err := NewSender(h.sbClient, nonexistentTopicName, nil)
	if err != nil {
		return nil, err
	}
	dt, err := openTopic(ctx, sbSender, nil)
	if err != nil {
		return nil, err
	}
	h.topics[dt] = nonexistentTopicName
	return dt, nil
}

func (h *harness) CreateSubscription(ctx context.Context, dt driver.Topic, testName string) (ds driver.Subscription, cleanup func(), err error) {
	subName := sanitize(fmt.Sprintf("%s-sub-%d", testName, atomic.AddUint32(&h.numSubs, 1)))
	topicName := h.topics[dt]
	err = createSubscription(ctx, topicName, subName, h.adminClient, nil)
	if err != nil {
		return nil, nil, err
	}

	var opts servicebus.ReceiverOptions
	if h.autodelete {
		opts.ReceiveMode = servicebus.ReceiveModeReceiveAndDelete
	}
	sbReceiver, err := NewReceiver(h.sbClient, topicName, subName, &opts)
	if err != nil {
		return nil, nil, err
	}

	sopts := SubscriptionOptions{}
	if h.autodelete {
		sopts.ReceiveAndDelete = true
	}
	ds, err = openSubscription(ctx, h.sbClient, sbReceiver, &sopts)
	if err != nil {
		return nil, nil, err
	}

	cleanup = func() {
		sbReceiver.Close(ctx)
		deleteSubscription(ctx, topicName, subName, h.adminClient)
	}
	return ds, cleanup, nil
}

func (h *harness) MakeNonexistentSubscription(ctx context.Context) (driver.Subscription, func(), error) {
	const topicName = "topic-for-nonexistent-sub"
	_, cleanup, err := h.CreateTopic(ctx, topicName)
	if err != nil {
		return nil, nil, err
	}
	sbReceiver, err := NewReceiver(h.sbClient, topicName, "nonexistent-subscription", nil)
	if err != nil {
		return nil, cleanup, err
	}
	sub, err := openSubscription(ctx, h.sbClient, sbReceiver, nil)
	return sub, cleanup, err
}

func (h *harness) Close() {
	h.closer()
}

func (h *harness) MaxBatchSizes() (int, int) { return sendBatcherOpts.MaxBatchSize, 0 }

func (h *harness) SupportsMultipleSubscriptions() bool { return true }

// Please run the TestConformance with an extended timeout since each test needs to perform CRUD for ServiceBus Topics and Subscriptions.
// Example: C:\Go\bin\go.exe test -timeout 60s gocloud.dev/pubsub/azuresb -run ^TestConformance$
func TestConformance(t *testing.T) {
	if !*setup.Record {
		t.Skip("replaying is not yet supported for Azure pubsub")
	}
	asTests := []drivertest.AsTest{sbAsTest{}}
	drivertest.RunConformanceTests(t, newHarness, asTests)
}

func TestConformanceWithAutodelete(t *testing.T) {
	if !*setup.Record {
		t.Skip("replaying is not yet supported for Azure pubsub")
	}
	asTests := []drivertest.AsTest{sbAsTest{}}
	drivertest.RunConformanceTests(t, newHarnessUsingAutodelete, asTests)
}

type sbAsTest struct{}

func (sbAsTest) Name() string {
	return "azure"
}

func (sbAsTest) TopicCheck(topic *pubsub.Topic) error {
	var t2 servicebus.Sender
	if topic.As(&t2) {
		return fmt.Errorf("cast succeeded for %T, want failure", &t2)
	}
	var t3 *servicebus.Sender
	if !topic.As(&t3) {
		return fmt.Errorf("cast failed for %T", &t3)
	}
	return nil
}

func (sbAsTest) SubscriptionCheck(sub *pubsub.Subscription) error {
	var s2 servicebus.Receiver
	if sub.As(&s2) {
		return fmt.Errorf("cast succeeded for %T, want failure", &s2)
	}
	var s3 *servicebus.Receiver
	if !sub.As(&s3) {
		return fmt.Errorf("cast failed for %T", &s3)
	}
	return nil
}

func (sbAsTest) TopicErrorCheck(t *pubsub.Topic, err error) error {
	return nil
}

func (sbAsTest) SubscriptionErrorCheck(s *pubsub.Subscription, err error) error {
	return nil
}

func (sbAsTest) MessageCheck(m *pubsub.Message) error {
	var m2 servicebus.ReceivedMessage
	if m.As(&m2) {
		return fmt.Errorf("cast succeeded for %T, want failure", &m2)
	}
	var m3 *servicebus.ReceivedMessage
	if !m.As(&m3) {
		return fmt.Errorf("cast failed for %T", &m3)
	}
	return nil
}

func (sbAsTest) BeforeSend(as func(interface{}) bool) error {
	var m *servicebus.Message
	if !as(&m) {
		return fmt.Errorf("cast failed for %T", &m)
	}
	return nil
}

func (sbAsTest) AfterSend(as func(interface{}) bool) error {
	return nil
}

func sanitize(s string) string {
	// First trim some not-so-useful strings that are part of all test names.
	s = strings.Replace(s, "TestConformance/Test", "", 1)
	s = strings.Replace(s, "TestConformanceWithAutodelete/Test", "", 1)
	s = strings.Replace(s, "/", "_", -1)
	if len(s) > maxNameLen {
		// Drop prefix, not suffix, because suffix includes something to make
		// entities unique within a test.
		s = s[len(s)-maxNameLen:]
	}
	return s
}

// createTopic ensures the existence of a Service Bus Topic on a given Namespace.
func createTopic(ctx context.Context, topicName string, adminClient *admin.Client, properties *admin.TopicProperties) error {
	t, _ := adminClient.GetTopic(ctx, topicName, nil)
	if t != nil {
		_, _ = adminClient.DeleteTopic(ctx, topicName, nil)
	}
	opts := admin.CreateTopicOptions{
		Properties: properties,
	}
	_, err := adminClient.CreateTopic(ctx, topicName, &opts)
	return err
}

// deleteTopic removes a Service Bus Topic on a given Namespace.
func deleteTopic(ctx context.Context, topicName string, adminClient *admin.Client) error {
	t, _ := adminClient.GetTopic(ctx, topicName, nil)
	if t != nil {
		_, err := adminClient.DeleteTopic(ctx, topicName, nil)
		return err
	}
	return nil
}

// createSubscription ensures the existence of a Service Bus Subscription on a given Namespace and Topic.
func createSubscription(ctx context.Context, topicName string, subscriptionName string, adminClient *admin.Client, properties *admin.SubscriptionProperties) error {
	s, _ := adminClient.GetSubscription(ctx, topicName, subscriptionName, nil)
	if s != nil {
		_, _ = adminClient.DeleteSubscription(ctx, topicName, subscriptionName, nil)
	}
	opts := admin.CreateSubscriptionOptions{
		Properties: properties,
	}
	_, err := adminClient.CreateSubscription(ctx, topicName, subscriptionName, &opts)
	return err
}

// deleteSubscription removes a Service Bus Subscription on a given Namespace and Topic.
func deleteSubscription(ctx context.Context, topicName string, subscriptionName string, adminClient *admin.Client) error {
	se, _ := adminClient.GetSubscription(ctx, topicName, subscriptionName, nil)
	if se != nil {
		_, err := adminClient.DeleteSubscription(ctx, topicName, subscriptionName, nil)
		return err
	}
	return nil
}

// to run test using Azure Entra credentials:
// 1. grant access to ${AZURE_CLIENT_ID} to Service Bus namespace
// 2. run test:
// AZURE_CLIENT_SECRET='secret' \
// AZURE_CLIENT_ID=client_id_uud \
// AZURE_TENANT_ID=tenant_id_uuid \
// AZURE_SERVICEBUS_HOSTNAME=hostname go test -benchmem -run=^$ -bench ^BenchmarkAzureServiceBusPubSub$ gocloud.dev/pubsub/azuresb
func BenchmarkAzureServiceBusPubSub(b *testing.B) {
	const (
		benchmarkTopicName        = "benchmark-topic"
		benchmarkSubscriptionName = "benchmark-subscription"
	)
	ctx := context.Background()

	var adminClient *admin.Client
	var sbClient *servicebus.Client
	var err error

	if connString == "" && sbHostname == "" {
		b.Fatal("azuresb: benchmark requires environment variable SERVICEBUS_CONNECTION_STRING or AZURE_SERVICEBUS_HOSTNAME to run")
	}

	if connString != "" {
		adminClient, err = admin.NewClientFromConnectionString(connString, nil)
		if err != nil {
			b.Fatal(err)
		}
		sbClient, err = NewClientFromConnectionString(connString, nil)
		if err != nil {
			b.Fatal(err)
		}
	} else if sbHostname != "" {
		cred, err := azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			b.Fatal(err)
		}
		adminClient, err = admin.NewClient(sbHostname, cred, nil)
		if err != nil {
			b.Fatal(err)
		}
		sbClient, err = NewClientFromServiceBusHostname(sbHostname, nil)
		if err != nil {
			b.Fatal(err)
		}
	}

	// Make topic.
	if err := createTopic(ctx, benchmarkTopicName, adminClient, nil); err != nil {
		b.Fatal(err)
	}
	defer deleteTopic(ctx, benchmarkTopicName, adminClient)

	sbSender, err := NewSender(sbClient, benchmarkTopicName, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer sbSender.Close(ctx)
	topic, err := OpenTopic(ctx, sbSender, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer topic.Shutdown(ctx)

	// Make subscription.
	if err := createSubscription(ctx, benchmarkTopicName, benchmarkSubscriptionName, adminClient, nil); err != nil {
		b.Fatal(err)
	}
	sbReceiver, err := NewReceiver(sbClient, benchmarkTopicName, benchmarkSubscriptionName, nil)
	if err != nil {
		b.Fatal(err)
	}
	sub, err := OpenSubscription(ctx, sbClient, sbReceiver, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer sub.Shutdown(ctx)

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
		// Setting listener_timeout.
		{"azuresb://mytopic?subscription=mysub&listener_timeout=10s", false},
		// Invalid listener_timeout.
		{"azuresb://mytopic?subscription=mysub&listener_timeout=xxx", true},
		// Invalid parameter.
		{"azuresb://mytopic?subscription=mysub&param=value", true},
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
