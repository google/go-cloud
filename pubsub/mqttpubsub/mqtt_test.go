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
	publicTestHost = "test.mosquitto.org"
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
		fmt.Println("ERRRRRRRRRR", err)
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

func (*harness) SupportsMultipleSubscriptions() bool { return true }

type mqttAsTest struct{}

func (mqttAsTest) Name() string {
	return "mqtt test"
}

func (mqttAsTest) TopicCheck(topic *pubsub.Topic) error {
	var pub Publisher
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
	var sub1 Subscriber
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
	var pm mqtt.Message
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


//func TestErrorCode(t *testing.T) {
//	ctx := context.Background()
//	dh, err := newHarness(ctx, t)
//	if err != nil {
//		t.Fatal(err)
//	}
//	defer dh.Close()
//	h := dh.(*harness)
//
//	Topics
	//dt, err := openTopic(h.Publisher, "bar")
	//if err != nil {
	//	t.Fatal(err)
	//}
	//
	//if gce := dt.ErrorCode(nil); gce != gcerrors.OK {
	//	t.Fatalf("Expected %v, got %v", gcerrors.OK, gce)
	//}
	//if gce := dt.ErrorCode(context.Canceled); gce != gcerrors.Canceled {
	//	t.Fatalf("Expected %v, got %v", gcerrors.Canceled, gce)
	//}
	//if gce := dt.ErrorCode(nats.ErrBadSubject); gce != gcerrors.FailedPrecondition {
	//	t.Fatalf("Expected %v, got %v", gcerrors.FailedPrecondition, gce)
	//}
	//if gce := dt.ErrorCode(nats.ErrAuthorization); gce != gcerrors.PermissionDenied {
	//	t.Fatalf("Expected %v, got %v", gcerrors.PermissionDenied, gce)
	//}
	//if gce := dt.ErrorCode(nats.ErrMaxPayload); gce != gcerrors.ResourceExhausted {
	//	t.Fatalf("Expected %v, got %v", gcerrors.ResourceExhausted, gce)
	//}
	//if gce := dt.ErrorCode(nats.ErrReconnectBufExceeded); gce != gcerrors.ResourceExhausted {
	//	t.Fatalf("Expected %v, got %v", gcerrors.ResourceExhausted, gce)
	//}
	//
	//Subscriptions
	//ds, err := openSubscription(h.Subscriber, "bar")
	//if err != nil {
	//	t.Fatal(err)
	//}
	//if gce := ds.ErrorCode(nil); gce != gcerrors.OK {
	//	t.Fatalf("Expected %v, got %v", gcerrors.OK, gce)
	//}
	//if gce := ds.ErrorCode(context.Canceled); gce != gcerrors.Canceled {
	//	t.Fatalf("Expected %v, got %v", gcerrors.Canceled, gce)
	//}
	//if gce := ds.ErrorCode(nats.ErrBadSubject); gce != gcerrors.FailedPrecondition {
	//	t.Fatalf("Expected %v, got %v", gcerrors.FailedPrecondition, gce)
	//}
	//if gce := ds.ErrorCode(nats.ErrBadSubscription); gce != gcerrors.NotFound {
	//	t.Fatalf("Expected %v, got %v", gcerrors.NotFound, gce)
	//}
	//if gce := ds.ErrorCode(nats.ErrTypeSubscription); gce != gcerrors.FailedPrecondition {
	//	t.Fatalf("Expected %v, got %v", gcerrors.FailedPrecondition, gce)
	//}
	//if gce := ds.ErrorCode(nats.ErrAuthorization); gce != gcerrors.PermissionDenied {
	//	t.Fatalf("Expected %v, got %v", gcerrors.PermissionDenied, gce)
	//}
	//if gce := ds.ErrorCode(nats.ErrMaxMessages); gce != gcerrors.ResourceExhausted {
	//	t.Fatalf("Expected %v, got %v", gcerrors.ResourceExhausted, gce)
	//}
	//if gce := ds.ErrorCode(nats.ErrSlowConsumer); gce != gcerrors.ResourceExhausted {
	//	t.Fatalf("Expected %v, got %v", gcerrors.ResourceExhausted, gce)
	//}
	//if gce := ds.ErrorCode(nats.ErrTimeout); gce != gcerrors.DeadlineExceeded {
	//	t.Fatalf("Expected %v, got %v", gcerrors.DeadlineExceeded, gce)
	//}
//}


//func fakeConnectionStringInEnv() func() {
//	oldEnvVal := os.Getenv("MQTT_SERVER_URL")
//	os.Setenv("MQTT_SERVER_URL", fmt.Sprintf("mqtt://localhost:%d", testPort))
//	return func() {
//		os.Setenv("MQTT_SERVER_URL", oldEnvVal)
//	}
//}
//
//func TestOpenTopicFromURL(t *testing.T) {
//	ctx := context.Background()
//	dh, err := newHarness(ctx, t)
//	if err != nil {
//		t.Fatal(err)
//	}
//	defer dh.Close()
//	fmt.Println("??????????/")
//	cleanup := fakeConnectionStringInEnv()
//	defer cleanup()
//
//	tests := []struct {
//		URL     string
//		WantErr bool
//	}{
//		OK.
		//{"mqtt://mytopic", false},
		//Invalid parameter.
		//{"mqtt://mytopic?param=value", true},
	//}
	//
	//for _, test := range tests {
	//	topic, err := pubsub.OpenTopic(ctx, test.URL)
	//	if (err != nil) != test.WantErr {
	//		t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
	//	}
	//	if topic != nil {
	//		topic.Shutdown(ctx)
	//	}
	//}
//}
//
//func TestOpenSubscriptionFromURL(t *testing.T) {
//	ctx := context.Background()
//	dh, err := newHarness(ctx, t)
//	if err != nil {
//		t.Fatal(err)
//	}
//	defer dh.Close()
//
//	cleanup := fakeConnectionStringInEnv()
//	defer cleanup()
//
//	tests := []struct {
//		URL     string
//		WantErr bool
//	}{
//		OK.
		//{"mqtt://mytopic", false},
		//Invalid parameter.
		//{"mqtt://mytopic?param=value", true},
	//}
	//
	//for _, test := range tests {
	//	sub, err := pubsub.OpenSubscription(ctx, test.URL)
	//	if (err != nil) != test.WantErr {
	//		t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
	//	}
	//	if sub != nil {
	//		sub.Shutdown(ctx)
	//	}
	//}
//}
//
//func TestCodec(t *testing.T) {
//	for _, dm := range []*driver.Message{
//		{Metadata: nil, Body: nil},
//		{Metadata: map[string]string{"a": "1"}, Body: nil},
//		{Metadata: nil, Body: []byte("hello")},
//		{Metadata: map[string]string{"a": "1"}, Body: []byte("hello")},
//		{Metadata: map[string]string{"a": "1"}, Body: []byte("hello"),
//			AckID: "foo", AsFunc: func(interface{}) bool { return true }},
//	} {
//		bytes, err := encodeMessage(dm)
//		if err != nil {
//			t.Fatal(err)
//		}
//		var got driver.Message
//		if err := decodeMessage(bytes, &got); err != nil {
//			t.Fatal(err)
//		}
//		want := *dm
//		want.AckID = nil
//		want.AsFunc = nil
//		if diff := cmp.Diff(got, want); diff != "" {
//			t.Errorf("%+v:\n%s", want, diff)
//		}
//	}
//}
//