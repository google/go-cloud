// Copyright 2019 The Go Cloud Development Kit Authors
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

package kafkapubsub // import "gocloud.dev/pubsub/kafkapubsub"

// To run these tests against a real Kafka server, run localkafka.sh.
// See https://github.com/spotify/docker-kafka for more on the docker container
// that the script runs.

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gocloud.dev/internal/testing/setup"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
	"gocloud.dev/pubsub/drivertest"
)

var (
	localBrokerAddrs = []string{"localhost:9092"}
	// Makes OpenSubscription wait ~forever until the subscriber has joined the
	// ConsumerGroup. Messages sent to the topic before the subscriber has joined
	// won't be received.
	subscriptionOptions = &SubscriptionOptions{WaitForJoin: 24 * time.Hour}
)

type harness struct {
	uniqueID  int
	numSubs   uint32
	numTopics uint32
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	if !setup.HasDockerTestEnvironment() {
		t.Skip("Skipping Kafka tests since the Kafka server is not available")
	}
	return &harness{uniqueID: rand.Int()}, nil
}

func createKafkaTopic(topicName string, partitions int32) (func(), error) {
	// Create the topic.
	config := MinimalConfig()
	admin, err := sarama.NewClusterAdmin(localBrokerAddrs, config)
	if err != nil {
		return func() {}, err
	}
	close1 := func() { admin.Close() }

	topicDetail := &sarama.TopicDetail{
		NumPartitions:     partitions,
		ReplicationFactor: 1,
	}
	if err := admin.CreateTopic(topicName, topicDetail, false); err != nil {
		return close1, err
	}
	close2 := func() {
		admin.DeleteTopic(topicName)
		close1()
	}
	return close2, nil
}

func (h *harness) CreateTopic(ctx context.Context, testName string) (driver.Topic, func(), error) {
	topicName := fmt.Sprintf("%s-topic-%d-%d", sanitize(testName), h.uniqueID, atomic.AddUint32(&h.numTopics, 1))
	cleanup, err := createKafkaTopic(topicName, 1)
	if err != nil {
		return nil, cleanup, err
	}

	// Open it.
	dt, err := openTopic(localBrokerAddrs, MinimalConfig(), topicName, nil)
	if err != nil {
		return nil, cleanup, err
	}
	return dt, cleanup, nil
}

func (h *harness) MakeNonexistentTopic(ctx context.Context) (driver.Topic, error) {
	return openTopic(localBrokerAddrs, MinimalConfig(), "nonexistent-topic", nil)
}

func (h *harness) CreateSubscription(ctx context.Context, dt driver.Topic, testName string) (driver.Subscription, func(), error) {
	groupID := fmt.Sprintf("%s-sub-%d-%d", sanitize(testName), h.uniqueID, atomic.AddUint32(&h.numSubs, 1))
	ds, err := openSubscription(localBrokerAddrs, MinimalConfig(), groupID, []string{dt.(*topic).topicName}, subscriptionOptions)
	return ds, func() {}, err
}

func (h *harness) MakeNonexistentSubscription(ctx context.Context) (driver.Subscription, func(), error) {
	ds, err := openSubscription(localBrokerAddrs, MinimalConfig(), "unused-group", []string{"nonexistent-topic"}, subscriptionOptions)
	return ds, func() {}, err
}

func (h *harness) Close() {}

func (h *harness) MaxBatchSizes() (int, int) { return sendBatcherOpts.MaxBatchSize, 0 }

func (*harness) SupportsMultipleSubscriptions() bool { return true }

func TestConformance(t *testing.T) {
	asTests := []drivertest.AsTest{asTest{}}
	drivertest.RunConformanceTests(t, newHarness, asTests)
}

type asTest struct{}

func (asTest) Name() string {
	return "kafka"
}

func (asTest) TopicCheck(topic *pubsub.Topic) error {
	var sp sarama.SyncProducer
	if !topic.As(&sp) {
		return fmt.Errorf("cast failed for %T", sp)
	}
	return nil
}

func (asTest) SubscriptionCheck(sub *pubsub.Subscription) error {
	var cg sarama.ConsumerGroup
	if !sub.As(&cg) {
		return fmt.Errorf("cast failed for %T", cg)
	}
	var cgs sarama.ConsumerGroupSession
	if !sub.As(&cgs) {
		return fmt.Errorf("cast failed for %T", cgs)
	}
	return nil
}

func (asTest) TopicErrorCheck(t *pubsub.Topic, err error) error {
	var pe sarama.ProducerErrors
	if !t.ErrorAs(err, &pe) {
		return fmt.Errorf("failed to convert %v (%T)", err, err)
	}
	return nil
}

func (asTest) SubscriptionErrorCheck(s *pubsub.Subscription, err error) error {
	var ke sarama.KError
	if !s.ErrorAs(err, &ke) {
		return fmt.Errorf("failed to convert %v (%T)", err, err)
	}
	return nil
}

func (asTest) MessageCheck(m *pubsub.Message) error {
	var cm *sarama.ConsumerMessage
	if !m.As(&cm) {
		return fmt.Errorf("cast failed for %T", cm)
	}
	return nil
}

func (asTest) BeforeSend(as func(any) bool) error {
	var pm *sarama.ProducerMessage
	if !as(&pm) {
		return fmt.Errorf("cast failed for %T", &pm)
	}
	return nil
}

func (asTest) AfterSend(as func(any) bool) error {
	return nil
}

// TestKafkaKey tests sending/receiving a message with the Kafka message key set.
func TestKafkaKey(t *testing.T) {
	if !setup.HasDockerTestEnvironment() {
		t.Skip("Skipping Kafka tests since the Kafka server is not available")
	}
	const (
		keyName  = "kafkakey"
		keyValue = "kafkakeyvalue"
	)
	uniqueID := rand.Int()
	ctx := context.Background()

	topicName := fmt.Sprintf("%s-topic-%d", sanitize(t.Name()), uniqueID)
	topicCleanup, err := createKafkaTopic(topicName, 1)
	defer topicCleanup()
	if err != nil {
		t.Fatal(err)
	}
	topic, err := OpenTopic(localBrokerAddrs, MinimalConfig(), topicName, &TopicOptions{KeyName: keyName})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := topic.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	}()

	groupID := fmt.Sprintf("%s-sub-%d", sanitize(t.Name()), uniqueID)
	subOpts := *subscriptionOptions
	subOpts.KeyName = keyName
	sub, err := OpenSubscription(localBrokerAddrs, MinimalConfig(), groupID, []string{topicName}, &subOpts)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := sub.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	}()

	m := &pubsub.Message{
		Metadata: map[string]string{
			"foo":   "bar",
			keyName: keyValue,
		},
		Body: []byte("hello world"),
		BeforeSend: func(as func(any) bool) error {
			// Verify that the Key field was set correctly on the outgoing Kafka
			// message.
			var pm *sarama.ProducerMessage
			if !as(&pm) {
				return errors.New("failed to convert to ProducerMessage")
			}
			gotKeyBytes, err := pm.Key.Encode()
			if err != nil {
				return fmt.Errorf("failed to Encode Kafka Key: %v", err)
			}
			if gotKey := string(gotKeyBytes); gotKey != keyValue {
				return errors.New("Kafka key wasn't set appropriately")
			}
			return nil
		},
	}
	err = topic.Send(ctx, m)
	if err != nil {
		t.Fatal(err)
	}

	// The test will hang here if the message isn't available, so use a shorter timeout.
	ctx2, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	got, err := sub.Receive(ctx2)
	if err != nil {
		t.Fatal(err)
	}
	got.Ack()

	m.BeforeSend = nil // don't expect this in the received message
	m.LoggableID = keyValue
	if diff := cmp.Diff(got, m, cmpopts.IgnoreUnexported(pubsub.Message{})); diff != "" {
		t.Errorf("got\n%v\nwant\n%v\ndiff\n%v", got, m, diff)
	}

	// Verify that Key was set in the received Kafka message via As.
	var cm *sarama.ConsumerMessage
	if !got.As(&cm) {
		t.Fatal("failed to get message As ConsumerMessage")
	}
	if gotKey := string(cm.Key); gotKey != keyValue {
		t.Errorf("got key %q want %q", gotKey, keyValue)
	}
}

// TestMultiplePartionsWithRebalancing tests use of a topic with multiple
// partitions, including the rebalancing that happens when a new consumer
// appears in the group.
func TestMultiplePartionsWithRebalancing(t *testing.T) {
	if !setup.HasDockerTestEnvironment() {
		t.Skip("Skipping Kafka tests since the Kafka server is not available")
	}
	const (
		keyName   = "kafkakey"
		nMessages = 10
	)
	uniqueID := rand.Int()
	ctx := context.Background()

	// Create a topic with 10 partitions. Using 10 instead of just 2 because
	// that also tests having multiple claims.
	topicName := fmt.Sprintf("%s-topic-%d", sanitize(t.Name()), uniqueID)
	topicCleanup, err := createKafkaTopic(topicName, 10)
	defer topicCleanup()
	if err != nil {
		t.Fatal(err)
	}
	topic, err := OpenTopic(localBrokerAddrs, MinimalConfig(), topicName, &TopicOptions{KeyName: keyName})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := topic.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	}()

	// Open a subscription.
	groupID := fmt.Sprintf("%s-sub-%d", sanitize(t.Name()), uniqueID)
	subOpts := *subscriptionOptions
	subOpts.KeyName = keyName
	sub, err := OpenSubscription(localBrokerAddrs, MinimalConfig(), groupID, []string{topicName}, &subOpts)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := sub.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	}()

	// Send some messages.
	send := func() {
		for i := 0; i < nMessages; i++ {
			m := &pubsub.Message{
				Metadata: map[string]string{
					keyName: fmt.Sprintf("key%d", i),
				},
				Body: []byte("hello world"),
			}
			if err := topic.Send(ctx, m); err != nil {
				t.Fatal(err)
			}
		}
	}
	send()

	// Receive the messages via the subscription.
	got := make(chan int)
	done := make(chan error)
	read := func(ctx context.Context, subNum int, sub *pubsub.Subscription) {
		for {
			m, err := sub.Receive(ctx)
			if err != nil {
				if err == context.Canceled {
					// Expected after all messages are received, no error.
					done <- nil
				} else {
					done <- err
				}
				return
			}
			m.Ack()
			got <- subNum
		}
	}
	// The test will hang here if the messages aren't available, so use a shorter
	// timeout.
	ctx2, cancel := context.WithTimeout(ctx, 30*time.Second)
	go read(ctx2, 0, sub)
	for i := 0; i < nMessages; i++ {
		select {
		case <-got:
		case err := <-done:
			// Premature error.
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	// Add another subscription to the same group. Kafka will rebalance the
	// consumer group, causing the Cleanup/Setup/ConsumeClaim loop. Each of the
	// two subscriptions should get claims for 50% of the partitions.
	sub2, err := OpenSubscription(localBrokerAddrs, MinimalConfig(), groupID, []string{topicName}, &subOpts)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := sub2.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	}()

	// Send and receive some messages.
	// Now both subscriptions should get some messages.
	send()

	// The test will hang here if the message isn't available, so use a shorter timeout.
	ctx3, cancel := context.WithTimeout(ctx, 30*time.Second)
	go read(ctx3, 0, sub)
	go read(ctx3, 1, sub2)
	counts := []int{0, 0}
	for i := 0; i < nMessages; i++ {
		select {
		case sub := <-got:
			counts[sub]++
		case err := <-done:
			// Premature error.
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	cancel()
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if counts[0] == 0 || counts[1] == 0 {
		t.Errorf("one of the partitioned subscriptions didn't get any messages: %v", counts)
	}
}

func sanitize(testName string) string {
	return strings.Replace(testName, "/", "_", -1)
}

func BenchmarkKafka(b *testing.B) {
	ctx := context.Background()
	uniqueID := rand.Int()

	// Create the topic.
	topicName := fmt.Sprintf("%s-topic-%d", b.Name(), uniqueID)
	cleanup, err := createKafkaTopic(topicName, 1)
	defer cleanup()
	if err != nil {
		b.Fatal(err)
	}

	topic, err := OpenTopic(localBrokerAddrs, MinimalConfig(), topicName, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer topic.Shutdown(ctx)

	groupID := fmt.Sprintf("%s-subscription-%d", b.Name(), uniqueID)
	sub, err := OpenSubscription(localBrokerAddrs, MinimalConfig(), groupID, []string{topicName}, subscriptionOptions)
	if err != nil {
		b.Fatal(err)
	}
	defer sub.Shutdown(ctx)

	drivertest.RunBenchmarks(b, topic, sub)
}

func fakeConnectionStringInEnv() func() {
	oldEnvVal := os.Getenv("KAFKA_BROKERS")
	os.Setenv("KAFKA_BROKERS", "localhost:10000")
	return func() {
		os.Setenv("KAFKA_BROKERS", oldEnvVal)
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
		{"kafka://mytopic", false},
		// OK, specifying key_name.
		{"kafka://mytopic?key_name=x-partition-key", false},
		// Invalid key_name value.
		{"kafka://mytopic?key_name=", true},
		// Invalid parameter.
		{"kafka://mytopic?param=value", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		topic, err := pubsub.OpenTopic(ctx, test.URL)
		if err != nil && errors.Is(err, sarama.ErrOutOfBrokers) {
			// Since we don't have a real kafka broker to talk to, we will always get an error when
			// opening a topic. This test is checking specifically for query parameter usage, so
			// we treat the "no brokers" error message as a nil error.
			err = nil
		}

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
		{"kafka://mygroup?topic=mytopic", false},
		// OK, specifying initial offset.
		{"kafka://mygroup?topic=mytopic&offset=oldest", false},
		{"kafka://mygroup?topic=mytopic&offset=newest", false},
		// Invalid offset specified.
		{"kafka://mygroup?topic=mytopic&offset=value", true},
		// Invalid parameter.
		{"kafka://mygroup?topic=mytopic&param=value", true},
	}

	ctx := context.Background()

	for _, test := range tests {
		sub, err := pubsub.OpenSubscription(ctx, test.URL)
		if err != nil && errors.Is(err, sarama.ErrOutOfBrokers) {
			// Since we don't have a real kafka broker to talk to, we will always get an error when
			// opening a subscription. This test is checking specifically for query parameter usage, so
			// we treat the "no brokers" error message as a nil error.
			err = nil
		}

		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
		if sub != nil {
			sub.Shutdown(ctx)
		}
	}
}
