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

package awssnssqs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/aws/smithy-go"
	"github.com/google/go-cmp/cmp"
	"gocloud.dev/internal/testing/setup"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
	"gocloud.dev/pubsub/drivertest"
)

const (
	region        = "us-east-2"
	accountNumber = "456752665576"
)

// We run conformance tests against multiple kinds of topics; this enum
// represents which one we're doing.
type topicKind string

const (
	topicKindSNS    = topicKind("SNS")    // send through an SNS topic
	topicKindSNSRaw = topicKind("SNSRaw") // send through an SNS topic using RawMessageDelivery=true
	topicKindSQS    = topicKind("SQS")    // send directly to an SQS queue
)

type harness struct {
	snsClient       *sns.Client
	sqsClient       *sqs.Client
	topicKind       topicKind
	rt              http.RoundTripper
	closer          func()
	numTopics       uint32
	numSubs         uint32
	useFIFO         bool
	topicAttributes map[string]string
}

type harnessOption func(h *harness)

func newHarness(ctx context.Context, t *testing.T, topicKind topicKind) (drivertest.Harness, error) {
	t.Helper()

	cfg, rt, closer, _ := setup.NewAWSv2Config(context.Background(), t, region)
	return &harness{snsClient: sns.NewFromConfig(cfg), sqsClient: sqs.NewFromConfig(cfg), rt: rt, topicKind: topicKind, closer: closer}, nil
}

func (h *harness) CreateTopic(ctx context.Context, testName string) (dt driver.Topic, cleanup func(), err error) {
	topicName := sanitize(fmt.Sprintf("%s-top-%d", testName, atomic.AddUint32(&h.numTopics, 1)))
	if h.useFIFO {
		topicName += ".fifo"
	}
	return createTopic(ctx, topicName, h.snsClient, h.sqsClient, h.topicKind, h.topicAttributes)
}

func convertStringToPtrMap(m map[string]string) map[string]*string {
	if m == nil {
		return nil
	}
	out := make(map[string]*string, len(m))
	for k, v := range m {
		out[k] = aws.String(v)
	}
	return out
}

func createTopic(ctx context.Context, topicName string, snsClient *sns.Client, sqsClient *sqs.Client, topicKind topicKind, attributes map[string]string) (dt driver.Topic, cleanup func(), err error) {
	switch topicKind {
	case topicKindSNS, topicKindSNSRaw:
		// Create an SNS topic.
		input := &sns.CreateTopicInput{Name: aws.String(topicName), Attributes: attributes}
		out, err := snsClient.CreateTopic(ctx, input)
		if err != nil {
			return nil, nil, fmt.Errorf("creating SNS topic %q: %v", topicName, err)
		}
		dt = openSNSTopic(ctx, snsClient, *out.TopicArn, &TopicOptions{})
		cleanup = func() {
			snsClient.DeleteTopic(ctx, &sns.DeleteTopicInput{TopicArn: out.TopicArn})
		}
		return dt, cleanup, nil
	case topicKindSQS:
		// Create an SQS queue.
		qURL, _, err := createSQSQueue(ctx, sqsClient, topicName, attributes)
		if err != nil {
			return nil, nil, fmt.Errorf("creating SQS queue %q: %v", topicName, err)
		}
		dt = openSQSTopic(ctx, sqsClient, qURL, &TopicOptions{})
		cleanup = func() {
			sqsClient.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(qURL)})
		}
		return dt, cleanup, nil
	default:
		panic("unreachable")
	}
}

func (h *harness) MakeNonexistentTopic(ctx context.Context) (driver.Topic, error) {
	switch h.topicKind {
	case topicKindSNS, topicKindSNSRaw:
		const fakeTopicARN = "arn:aws:sns:" + region + ":" + accountNumber + ":nonexistenttopic"
		return openSNSTopic(ctx, h.snsClient, fakeTopicARN, &TopicOptions{}), nil
	case topicKindSQS:
		const fakeQueueURL = "https://" + region + ".amazonaws.com/" + accountNumber + "/nonexistent-queue"
		return openSQSTopic(ctx, h.sqsClient, fakeQueueURL, &TopicOptions{}), nil
	default:
		panic("unreachable")
	}
}

func (h *harness) CreateSubscription(ctx context.Context, dt driver.Topic, testName string) (ds driver.Subscription, cleanup func(), err error) {
	subName := sanitize(fmt.Sprintf("%s-sub-%d", testName, atomic.AddUint32(&h.numSubs, 1)))
	return createSubscription(ctx, dt, subName, h.snsClient, h.sqsClient, h.topicKind)
}

func createSubscription(ctx context.Context, dt driver.Topic, subName string, snsClient *sns.Client, sqsClient *sqs.Client, topicKind topicKind) (ds driver.Subscription, cleanup func(), err error) {
	switch topicKind {
	case topicKindSNS, topicKindSNSRaw:
		// Create an SQS queue, and subscribe it to the SNS topic.
		qURL, qARN, err := createSQSQueue(ctx, sqsClient, subName, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("creating SQS queue %q: %v", subName, err)
		}
		ds = openSubscription(ctx, sqsClient, qURL, &SubscriptionOptions{})

		snsTopicARN := dt.(*snsTopic).arn
		var cleanup func()
		req := &sns.SubscribeInput{
			TopicArn: aws.String(snsTopicARN),
			Endpoint: aws.String(qARN),
			Protocol: aws.String("sqs"),
		}
		// Enable RawMessageDelivery on the subscription if needed.
		if topicKind == topicKindSNSRaw {
			req.Attributes = map[string]string{"RawMessageDelivery": "true"}
		}
		out, err := snsClient.Subscribe(ctx, req)
		if err != nil {
			return nil, nil, fmt.Errorf("subscribing: %v", err)
		}
		cleanup = func() {
			snsClient.Unsubscribe(ctx, &sns.UnsubscribeInput{SubscriptionArn: out.SubscriptionArn})
			sqsClient.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(qURL)})
		}
		return ds, cleanup, nil
	case topicKindSQS:
		// The SQS queue already exists; we created it for the topic. Re-use it
		// for the subscription.
		qURL := dt.(*sqsTopic).qURL
		return openSubscription(ctx, sqsClient, qURL, &SubscriptionOptions{}), func() {}, nil
	default:
		panic("unreachable")
	}
}

func createSQSQueue(ctx context.Context, sqsClient *sqs.Client, topicName string, attributes map[string]string) (string, string, error) {
	out, err := sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String(topicName), Attributes: attributes})
	if err != nil {
		return "", "", fmt.Errorf("creating SQS queue %q: %v", topicName, err)
	}
	qURL := aws.ToString(out.QueueUrl)

	// Get the ARN.
	out2, err := sqsClient.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(qURL),
		AttributeNames: []sqstypes.QueueAttributeName{"QueueArn"},
	})
	if err != nil {
		return "", "", fmt.Errorf("getting queue ARN for %s: %v", qURL, err)
	}
	qARN := out2.Attributes["QueueArn"]

	queuePolicy := `{
		"Version": "2012-10-17",
		"Id": "AllowQueue",
		"Statement": [
		{
		"Sid": "MySQSPolicy001",
		"Effect": "Allow",
		"Principal": {
		"AWS": "*"
		},
		"Action": "sqs:SendMessage",
		"Resource": "` + qARN + `"
		}
		]
		}`
	if _, err := sqsClient.SetQueueAttributes(ctx, &sqs.SetQueueAttributesInput{
		Attributes: map[string]string{"Policy": queuePolicy},
		QueueUrl:   aws.String(qURL),
	}); err != nil {
		return "", "", fmt.Errorf("setting policy: %v", err)
	}
	return qURL, qARN, nil
}

func (h *harness) MakeNonexistentSubscription(ctx context.Context) (driver.Subscription, func(), error) {
	const fakeSubscriptionQueueURL = "https://" + region + ".amazonaws.com/" + accountNumber + "/nonexistent-subscription"
	return openSubscription(ctx, h.sqsClient, fakeSubscriptionQueueURL, &SubscriptionOptions{}), func() {}, nil
}

func (h *harness) Close() {
	h.closer()
}

func (h *harness) MaxBatchSizes() (int, int) {
	if h.topicKind == topicKindSQS {
		return sendBatcherOptsSQS.MaxBatchSize, ackBatcherOpts.MaxBatchSize
	}
	return sendBatcherOptsSNS.MaxBatchSize, ackBatcherOpts.MaxBatchSize
}

func (h *harness) SupportsMultipleSubscriptions() bool {
	// If we're publishing to an SQS topic, we're reading from the same topic,
	// so there's no way to get multiple subscriptions.
	return h.topicKind != topicKindSQS
}

func TestConformanceSNSTopic(t *testing.T) {
	asTests := []drivertest.AsTest{awsAsTest{topicKind: topicKindSNS}}
	newSNSHarness := func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
		t.Helper()

		return newHarness(ctx, t, topicKindSNS)
	}
	drivertest.RunConformanceTests(t, newSNSHarness, asTests)
}

func TestConformanceSNSTopicRaw(t *testing.T) {
	asTests := []drivertest.AsTest{awsAsTest{topicKind: topicKindSNSRaw}}
	newSNSHarness := func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
		t.Helper()

		return newHarness(ctx, t, topicKindSNSRaw)
	}
	drivertest.RunConformanceTests(t, newSNSHarness, asTests)
}

func TestConformanceSQSTopic(t *testing.T) {
	asTests := []drivertest.AsTest{awsAsTest{topicKind: topicKindSQS}}
	newSQSHarness := func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
		t.Helper()

		return newHarness(ctx, t, topicKindSQS)
	}
	drivertest.RunConformanceTests(t, newSQSHarness, asTests)
}

type awsAsTest struct {
	topicKind topicKind
}

func (awsAsTest) Name() string {
	return "aws test"
}

func (t awsAsTest) TopicCheck(topic *pubsub.Topic) error {
	switch t.topicKind {
	case topicKindSNS, topicKindSNSRaw:
		var s *sns.Client
		if !topic.As(&s) {
			return fmt.Errorf("cast failed for %T", s)
		}
	case topicKindSQS:
		var s *sqs.Client
		if !topic.As(&s) {
			return fmt.Errorf("cast failed for %T", s)
		}
	default:
		panic("unreachable")
	}
	return nil
}

func (t awsAsTest) SubscriptionCheck(sub *pubsub.Subscription) error {
	var s *sqs.Client
	if !sub.As(&s) {
		return fmt.Errorf("cast failed for %T", s)
	}
	return nil
}

func (t awsAsTest) TopicErrorCheck(topic *pubsub.Topic, err error) error {
	var e smithy.APIError
	if !topic.ErrorAs(err, &e) {
		return errors.New("Topic.ErrorAs failed")
	}
	switch t.topicKind {
	case topicKindSNS, topicKindSNSRaw:
		if got, want := e.ErrorCode(), (&snstypes.NotFoundException{}).ErrorCode(); want != got {
			return fmt.Errorf("got %q, want %q", got, want)
		}
	case topicKindSQS:
		if got, want := e.ErrorCode(), "AWS.SimpleQueueService.NonExistentQueue"; got != want {
			return fmt.Errorf("got %q, want %q", got, want)
		}
	default:
		panic("unreachable")
	}
	return nil
}

func (t awsAsTest) SubscriptionErrorCheck(s *pubsub.Subscription, err error) error {
	var e smithy.APIError
	if !s.ErrorAs(err, &e) {
		return errors.New("Subscription.ErrorAs failed")
	}
	if got, want := e.ErrorCode(), "AWS.SimpleQueueService.NonExistentQueue"; got != want {
		return fmt.Errorf("got %q, want %q", got, want)
	}
	return nil
}

func (t awsAsTest) MessageCheck(m *pubsub.Message) error {
	var sm sqstypes.Message
	if !m.As(&sm) {
		return fmt.Errorf("cast failed for %T", &sm)
	}
	return nil
}

func (t awsAsTest) BeforeSend(as func(any) bool) error {
	switch t.topicKind {
	case topicKindSNS, topicKindSNSRaw:
		var pub *sns.PublishInput
		if !as(&pub) {
			return fmt.Errorf("cast failed for %T", &pub)
		}
		var entry *snstypes.PublishBatchRequestEntry
		if !as(&entry) {
			return fmt.Errorf("cast failed for %T", &entry)
		}
	case topicKindSQS:
		var entry *sqstypes.SendMessageBatchRequestEntry
		if !as(&entry) {
			return fmt.Errorf("cast failed for %T", &entry)
		}
	default:
		panic("unreachable")
	}
	return nil
}

func (t awsAsTest) AfterSend(as func(any) bool) error {
	switch t.topicKind {
	case topicKindSNS, topicKindSNSRaw:
		var pub *sns.PublishOutput
		if !as(&pub) {
			return fmt.Errorf("cast failed for %T", &pub)
		}
		var entry snstypes.PublishBatchResultEntry
		if !as(&entry) {
			return fmt.Errorf("cast failed for %T", &entry)
		}
	case topicKindSQS:
		var entry sqstypes.SendMessageBatchResultEntry
		if !as(&entry) {
			return fmt.Errorf("cast failed for %T", &entry)
		}
	default:
		panic("unreachable")
	}
	return nil
}

func sanitize(s string) string {
	// AWS doesn't like names that are too long; trim some not-so-useful stuff.
	const maxNameLen = 80
	s = strings.Replace(s, "TestConformance", "", 1)
	s = strings.Replace(s, "/Test", "", 1)
	s = strings.Replace(s, "/", "_", -1)
	if len(s) > maxNameLen {
		// Drop prefix, not suffix, because suffix includes something to make
		// entities unique within a test.
		s = s[len(s)-maxNameLen:]
	}
	return s
}

func BenchmarkSNSSQS(b *testing.B) {
	benchmark(b, topicKindSNS)
}

func BenchmarkSQS(b *testing.B) {
	benchmark(b, topicKindSQS)
}

func benchmark(b *testing.B, topicKind topicKind) {
	b.Helper()

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		b.Fatal(err)
	}
	topicName := fmt.Sprintf("%s-topic", b.Name())
	snsClient := sns.NewFromConfig(cfg)
	sqsClient := sqs.NewFromConfig(cfg)
	dt, cleanup1, err := createTopic(ctx, topicName, snsClient, sqsClient, topicKind, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer cleanup1()
	sendBatcherOpts := sendBatcherOptsSNS
	if topicKind == topicKindSQS {
		sendBatcherOpts = sendBatcherOptsSQS
	}
	topic := pubsub.NewTopic(dt, sendBatcherOpts)
	defer topic.Shutdown(ctx)
	subName := fmt.Sprintf("%s-subscription", b.Name())
	ds, cleanup2, err := createSubscription(ctx, dt, subName, snsClient, sqsClient, topicKind)
	if err != nil {
		b.Fatal(err)
	}
	defer cleanup2()
	sub := pubsub.NewSubscription(ds, recvBatcherOpts, ackBatcherOpts)
	defer sub.Shutdown(ctx)
	drivertest.RunBenchmarks(b, topic, sub)
}

func TestOpenTopicFromURL(t *testing.T) {
	tests := []struct {
		URL     string
		WantErr bool
	}{
		// SNS...

		// OK.
		{"awssns:///arn:aws:service:region:accountid:resourceType/resourcePath", false},
		// OK, setting region.
		{"awssns:///arn:aws:service:region:accountid:resourceType/resourcePath?region=us-east-2", false},
		// Invalid parameter.
		{"awssns:///arn:aws:service:region:accountid:resourceType/resourcePath?param=value", true},

		// SQS...
		// OK.
		{"awssqs://sqs.us-east-2.amazonaws.com/99999/my-queue", false},
		// OK, setting region.
		{"awssqs://sqs.us-east-2.amazonaws.com/99999/my-queue?region=us-east-2", false},
		// Invalid parameter.
		{"awssqs://sqs.us-east-2.amazonaws.com/99999/my-queue?param=value", true},
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
	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"awssqs://sqs.us-east-2.amazonaws.com/99999/my-queue", false},
		// OK, setting region.
		{"awssqs://sqs.us-east-2.amazonaws.com/99999/my-queue?region=us-east-2", false},
		// OK, setting raw.
		{"awssqs://sqs.us-east-2.amazonaws.com/99999/my-queue?raw=true", false},
		// OK, setting raw.
		{"awssqs://sqs.us-east-2.amazonaws.com/99999/my-queue?raw=1", false},
		// Invalid raw.
		{"awssqs://sqs.us-east-2.amazonaws.com/99999/my-queue?raw=foo", true},
		// OK, setting nacklazy.
		{"awssqs://sqs.us-east-2.amazonaws.com/99999/my-queue?nacklazy=1", false},
		// Invalid nacklazy.
		{"awssqs://sqs.us-east-2.amazonaws.com/99999/my-queue?nacklazy=foo", true},
		// OK, setting waittime.
		{"awssqs://sqs.us-east-2.amazonaws.com/99999/my-queue?waittime=5s", false},
		// Invalid waittime.
		{"awssqs://sqs.us-east-2.amazonaws.com/99999/my-queue?waittime=foo", true},
		// Invalid parameter.
		{"awssqs://sqs.us-east-2.amazonaws.com/99999/my-queue?param=value", true},
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

func TestFIFO(t *testing.T) {
	for _, tt := range []struct {
		name string
		kind topicKind
	}{
		{
			name: "TestSNSTopic",
			kind: topicKindSNS,
		},
		// This test is flaky because it sets 2 attributes for CreateTopic,
		// and the HTTP record/replay randomly re-sorts them. I'm not sure how
		// to fix that, so disabling the test. It's also unclear why only
		// this test appears affected, maybe the AWS code iterates over the
		// map here and doesn't in other cases?
		/*
			{
				name:  "TestSQSTopic",
				kind:  topicKindSQS,
			},
		*/
	} {
		t.Run(tt.name, func(t *testing.T) {
			testFIFOTopic(t, tt.kind)
		})
	}
}

// testFIFOTopic tests FIFO topics.
//
// FIFO topics require a message group ID to be set on the message.
//
// The content-based deduplication attribute must be set to true on the topic.
//   - If set to true, the message deduplication ID is generated using the message body (sha256 hash).
//   - If not set, then the DeduplicationID must be set on the message.
//
// For more information see:
//   - https://pkg.go.dev/github.com/aws/aws-sdk-go/service/sns#CreateTopicInput.Attributes
//   - https://pkg.go.dev/github.com/aws/aws-sdk-go/service/sqs#CreateQueueInput.Attributes
func testFIFOTopic(t *testing.T, kind topicKind) {
	t.Helper()
	type harnessArgs struct {
		attributes map[string]string
	}

	const (
		attributeKeyContentBasedDeduplication = "ContentBasedDeduplication"
		attributeKeyFifoTopic                 = "FifoTopic"
		attributeKeyFifoQueue                 = "FifoQueue"
	)

	tests := []struct {
		name    string
		harness harnessArgs
		message *pubsub.Message
		wantErr bool
	}{
		{
			name: "TestSendReceiveValid",
			harness: harnessArgs{
				attributes: map[string]string{
					attributeKeyContentBasedDeduplication: "true",
				},
			},
			message: &pubsub.Message{
				Body: []byte("hello world"),
				Metadata: map[string]string{
					MetadataKeyMessageGroupID: "1",
				},
			},
			wantErr: false,
		},
		{
			name: "TestSendReceiveInvalidNoMessageGroupID",
			harness: harnessArgs{
				attributes: map[string]string{
					attributeKeyContentBasedDeduplication: "true",
				},
			},
			message: &pubsub.Message{
				Body: []byte("hello world"),
				Metadata: map[string]string{
					MetadataKeyDeduplicationID: "1",
				},
			},
			wantErr: true,
		},
		{
			name: "TestSendReceiveInvalidNoDeduplicationID",
			// We dont set the ContentBasedDeduplication attribute to trigger the error.
			harness: harnessArgs{},
			message: &pubsub.Message{
				Body:     []byte("hello world"),
				Metadata: map[string]string{},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create the harness.
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			h, err := newHarness(ctx, t, kind)
			if err != nil {
				t.Fatal(err)
			}
			defer h.Close()

			// Set the FIFO attributes.
			attributes := make(map[string]string)
			for k, v := range tt.harness.attributes {
				attributes[k] = v
			}
			switch kind {
			case topicKindSNS:
				attributes[attributeKeyFifoTopic] = "true"
			case topicKindSQS:
				attributes[attributeKeyFifoQueue] = "true"
			}
			h.(*harness).topicAttributes = attributes
			h.(*harness).useFIFO = true

			// Create the topic and subscription.
			dt, cleanup, err := h.CreateTopic(ctx, t.Name())
			if err != nil {
				t.Errorf("harness.CreateTopic() error = %v", err)
				return
			}
			defer cleanup()
			topic := pubsub.NewTopic(dt, sendBatcherOptsSNS)
			defer topic.Shutdown(ctx)
			ds, cleanup, err := h.CreateSubscription(ctx, dt, t.Name())
			if err != nil {
				t.Errorf("harness.CreateSubscription() error = %v", err)
				return
			}
			defer cleanup()
			sub := pubsub.NewSubscription(ds, recvBatcherOpts, ackBatcherOpts)
			defer sub.Shutdown(ctx)

			// Send and receive the message.
			err = topic.Send(ctx, tt.message)
			if (err != nil) != tt.wantErr {
				t.Errorf("Topic.Send() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			m, err := sub.Receive(ctx)
			if err != nil {
				t.Errorf("Subscription.Receive() error = %v", err)
				return
			}
			if diff := cmp.Diff(tt.message.Body, m.Body); diff != "" {
				t.Errorf("Received message body: -got, +want: %s", diff)
			}
			m.Ack()
		})
	}
}
