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

package mqttpubsub

import (
	"context"
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"gocloud.dev/internal/testing/setup"
	"testing"

	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
	"gocloud.dev/pubsub/drivertest"
)

const (
	publicTestHost = "test.mosquitto.org" // officially for testing
	localTestHost  = "localhost"
	testPort       = 1883
)

type harness struct {
	Subscriber
	Publisher
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	url := fmt.Sprintf("%s:%d", localTestHost, testPort)
	if !setup.HasDockerTestEnvironment() {
		t.Log("using the public server because the local MQTT server is not available")
		url = fmt.Sprintf("%s:%d", publicTestHost, testPort)

	}

	sub, err := defaultSubClient(url)
	if err != nil {
		return nil, err
	}
	pub, err := defaultPubClient(url)
	if err != nil {
		return nil, err
	}

	return &harness{sub, pub}, nil
}

func (h *harness) CreateTopic(ctx context.Context, testName string) (driver.Topic, func(), error) {
	cleanup := func() {}
	dt, err := openTopic(h.Publisher, testName)
	if err != nil {
		return nil, nil, err
	}

	return dt, cleanup, nil
}

func (h *harness) MakeNonexistentTopic(ctx context.Context) (driver.Topic, error) {
	// A nil *topic behaves like a nonexistent topic.
	return (*topic)(nil), nil
}

func (h *harness) CreateSubscription(ctx context.Context, dt driver.Topic, testName string) (driver.Subscription, func(), error) {
	ds, err := openSubscription(h.Subscriber, testName)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		var sub Subscriber
		if ds.As(&sub) {
			h.Subscriber.UnSubscribe(testName)
		}
	}
	return ds, cleanup, nil
}

func (h *harness) MakeNonexistentSubscription(ctx context.Context) (driver.Subscription, error) {
	return (*subscription)(nil), nil
}

func (h *harness) Close() {
	h.Publisher.Stop()
	h.Subscriber.Close()
}

func (*harness) MaxBatchSizes() (int, int) { return 0, 0 }

// supported from MQTT v5.0. Not supported by "github.com/eclipse/paho.mqtt.golang" driver
func (*harness) SupportsMultipleSubscriptions() bool { return false }

type mqttAsTest struct{}

func (mqttAsTest) Name() string {
	return "mqtt test"
}

func (mqttAsTest) TopicCheck(topic *pubsub.Topic) error {
	var pub *Publisher
	if topic.As(&pub) {
		return fmt.Errorf("cast succeeded for %T, want failure", &pub)
	}
	var pub2 Publisher
	if !topic.As(&pub2) {
		return fmt.Errorf("cast failed for %T", &pub2)
	}
	return nil
}

func (mqttAsTest) SubscriptionCheck(sub *pubsub.Subscription) error {
	var sub1 *Subscriber
	if sub.As(&sub1) {
		return fmt.Errorf("cast succeeded for %T, want failure", &sub1)
	}
	var sub2 Subscriber
	if !sub.As(&sub2) {
		return fmt.Errorf("cast failed for %T", &sub2)
	}
	return nil
}

func (mqttAsTest) TopicErrorCheck(t *pubsub.Topic, err error) error {
	var dummy string
	if t.ErrorAs(err, &dummy) {
		return fmt.Errorf("cast succeeded for %T, want failure", &dummy)
	}
	return nil
}

func (mqttAsTest) SubscriptionErrorCheck(s *pubsub.Subscription, err error) error {
	var dummy string
	if s.ErrorAs(err, &dummy) {
		return fmt.Errorf("cast succeeded for %T, want failure", &dummy)
	}
	return nil
}

func (mqttAsTest) MessageCheck(m *pubsub.Message) error {
	var pm *mqtt.Message
	if m.As(&pm) {
		return fmt.Errorf("cast succeeded for %T, want failure", &pm)
	}
	var ppm mqtt.Message
	if !m.As(&ppm) {
		return fmt.Errorf("cast failed for %T", &ppm)
	}
	return nil
}

func (mqttAsTest) BeforeSend(as func(interface{}) bool) error {
	return nil
}

func TestConformance(t *testing.T) {
	asTests := []drivertest.AsTest{mqttAsTest{}}
	drivertest.RunConformanceTests(t, newHarness, asTests)
}
