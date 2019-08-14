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
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sqs"
	"gocloud.dev/internal/testing/setup"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
	"gocloud.dev/pubsub/drivertest"
)

const (
	region        = "us-east-2"
	accountNumber = "462380225722"
)

// We run conformance tests against multiple kinds of topics; this enum
// represents which one we're doing.
type topicKind string

const (
	topicKindSNS    = topicKind("SNS")    // send through an SNS topic
	topicKindSNSRaw = topicKind("SNSRaw") // send through an SNS topic using RawMessageDelivery=true
	topicKindSQS    = topicKind("SQS")    // send directly to an SQS queue
)

func newSession() (*session.Session, error) {
	return session.NewSession(&aws.Config{
		HTTPClient: &http.Client{},
		Region:     aws.String(region),
		MaxRetries: aws.Int(0),
	})
}

type harness struct {
	sess      *session.Session
	topicKind topicKind
	rt        http.RoundTripper
	closer    func()
	numTopics uint32
	numSubs   uint32
}

func newHarness(ctx context.Context, t *testing.T, topicKind topicKind) (drivertest.Harness, error) {
	sess, rt, closer, _ := setup.NewAWSSession(ctx, t, region)
	return &harness{sess: sess, rt: rt, topicKind: topicKind, closer: closer}, nil
}

func (h *harness) CreateTopic(ctx context.Context, testName string) (dt driver.Topic, cleanup func(), err error) {
	topicName := sanitize(fmt.Sprintf("%s-top-%d", testName, atomic.AddUint32(&h.numTopics, 1)))
	return createTopic(ctx, topicName, h.sess, h.topicKind)
}

func createTopic(ctx context.Context, topicName string, sess *session.Session, topicKind topicKind) (dt driver.Topic, cleanup func(), err error) {
	switch topicKind {
	case topicKindSNS, topicKindSNSRaw:
		// Create an SNS topic.
		client := sns.New(sess)
		out, err := client.CreateTopicWithContext(ctx, &sns.CreateTopicInput{Name: aws.String(topicName)})
		if err != nil {
			return nil, nil, fmt.Errorf("creating SNS topic %q: %v", topicName, err)
		}
		dt = openSNSTopic(ctx, sess, *out.TopicArn, nil)
		cleanup = func() {
			client.DeleteTopicWithContext(ctx, &sns.DeleteTopicInput{TopicArn: out.TopicArn})
		}
		return dt, cleanup, nil
	case topicKindSQS:
		// Create an SQS queue.
		sqsClient := sqs.New(sess)
		qURL, _, err := createSQSQueue(ctx, sqsClient, topicName)
		if err != nil {
			return nil, nil, fmt.Errorf("creating SQS queue %q: %v", topicName, err)
		}
		dt = openSQSTopic(ctx, sess, qURL, nil)
		cleanup = func() {
			sqsClient.DeleteQueueWithContext(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(qURL)})
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
		return openSNSTopic(ctx, h.sess, fakeTopicARN, nil), nil
	case topicKindSQS:
		const fakeQueueURL = "https://" + region + ".amazonaws.com/" + accountNumber + "/nonexistent-queue"
		return openSQSTopic(ctx, h.sess, fakeQueueURL, nil), nil
	default:
		panic("unreachable")
	}
}

func (h *harness) CreateSubscription(ctx context.Context, dt driver.Topic, testName string) (ds driver.Subscription, cleanup func(), err error) {
	subName := sanitize(fmt.Sprintf("%s-sub-%d", testName, atomic.AddUint32(&h.numSubs, 1)))
	return createSubscription(ctx, dt, subName, h.sess, h.topicKind)
}

func createSubscription(ctx context.Context, dt driver.Topic, subName string, sess *session.Session, topicKind topicKind) (ds driver.Subscription, cleanup func(), err error) {
	switch topicKind {
	case topicKindSNS, topicKindSNSRaw:
		// Create an SQS queue, and subscribe it to the SNS topic.
		sqsClient := sqs.New(sess)
		qURL, qARN, err := createSQSQueue(ctx, sqsClient, subName)
		if err != nil {
			return nil, nil, fmt.Errorf("creating SQS queue %q: %v", subName, err)
		}
		ds = openSubscription(ctx, sess, qURL, nil)

		snsTopicARN := dt.(*snsTopic).arn
		snsClient := sns.New(sess)
		req := &sns.SubscribeInput{
			TopicArn: aws.String(snsTopicARN),
			Endpoint: aws.String(qARN),
			Protocol: aws.String("sqs"),
		}
		// Enable RawMessageDelivery on the subscription if needed.
		if topicKind == topicKindSNSRaw {
			req.Attributes = map[string]*string{"RawMessageDelivery": aws.String("true")}
		}
		out, err := snsClient.SubscribeWithContext(ctx, req)
		if err != nil {
			return nil, nil, fmt.Errorf("subscribing: %v", err)
		}
		cleanup := func() {
			snsClient.UnsubscribeWithContext(ctx, &sns.UnsubscribeInput{SubscriptionArn: out.SubscriptionArn})
			sqsClient.DeleteQueueWithContext(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(qURL)})
		}
		return ds, cleanup, nil
	case topicKindSQS:
		// The SQS queue already exists; we created it for the topic. Re-use it
		// for the subscription.
		qURL := dt.(*sqsTopic).qURL
		return openSubscription(ctx, sess, qURL, nil), func() {}, nil
	default:
		panic("unreachable")
	}
}

func createSQSQueue(ctx context.Context, sqsClient *sqs.SQS, topicName string) (string, string, error) {
	out, err := sqsClient.CreateQueueWithContext(ctx, &sqs.CreateQueueInput{QueueName: aws.String(topicName)})
	if err != nil {
		return "", "", fmt.Errorf("creating SQS queue %q: %v", topicName, err)
	}
	qURL := aws.StringValue(out.QueueUrl)

	// Get the ARN.
	out2, err := sqsClient.GetQueueAttributesWithContext(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(qURL),
		AttributeNames: []*string{aws.String("QueueArn")},
	})
	if err != nil {
		return "", "", fmt.Errorf("getting queue ARN for %s: %v", qURL, err)
	}
	qARN := aws.StringValue(out2.Attributes["QueueArn"])

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
	_, err = sqsClient.SetQueueAttributesWithContext(ctx, &sqs.SetQueueAttributesInput{
		Attributes: map[string]*string{"Policy": &queuePolicy},
		QueueUrl:   aws.String(qURL),
	})
	if err != nil {
		return "", "", fmt.Errorf("setting policy: %v", err)
	}
	return qURL, qARN, nil
}

func (h *harness) MakeNonexistentSubscription(ctx context.Context) (driver.Subscription, error) {
	const fakeSubscriptionQueueURL = "https://" + region + ".amazonaws.com/" + accountNumber + "/nonexistent-subscription"
	return openSubscription(ctx, h.sess, fakeSubscriptionQueueURL, nil), nil
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
		return newHarness(ctx, t, topicKindSNS)
	}
	drivertest.RunConformanceTests(t, newSNSHarness, asTests)
}

func TestConformanceSNSTopicRaw(t *testing.T) {
	asTests := []drivertest.AsTest{awsAsTest{topicKind: topicKindSNSRaw}}
	newSNSHarness := func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
		return newHarness(ctx, t, topicKindSNSRaw)
	}
	drivertest.RunConformanceTests(t, newSNSHarness, asTests)
}

func TestConformanceSQSTopic(t *testing.T) {
	asTests := []drivertest.AsTest{awsAsTest{topicKind: topicKindSQS}}
	newSQSHarness := func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
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
		var s *sns.SNS
		if !topic.As(&s) {
			return fmt.Errorf("cast failed for %T", s)
		}
	case topicKindSQS:
		var s *sqs.SQS
		if !topic.As(&s) {
			return fmt.Errorf("cast failed for %T", s)
		}
	default:
		panic("unreachable")
	}
	return nil
}

func (awsAsTest) SubscriptionCheck(sub *pubsub.Subscription) error {
	var s *sqs.SQS
	if !sub.As(&s) {
		return fmt.Errorf("cast failed for %T", s)
	}
	return nil
}

func (t awsAsTest) TopicErrorCheck(topic *pubsub.Topic, err error) error {
	var ae awserr.Error
	if !topic.ErrorAs(err, &ae) {
		return fmt.Errorf("failed to convert %v (%T) to an awserr.Error", err, err)
	}
	switch t.topicKind {
	case topicKindSNS, topicKindSNSRaw:
		if got, want := ae.Code(), sns.ErrCodeNotFoundException; got != want {
			return fmt.Errorf("got %q, want %q", got, want)
		}
	case topicKindSQS:
		if got, want := ae.Code(), sqs.ErrCodeQueueDoesNotExist; got != want {
			return fmt.Errorf("got %q, want %q", got, want)
		}
	default:
		panic("unreachable")
	}
	return nil
}

func (awsAsTest) SubscriptionErrorCheck(s *pubsub.Subscription, err error) error {
	var ae awserr.Error
	if !s.ErrorAs(err, &ae) {
		return fmt.Errorf("failed to convert %v (%T) to an awserr.Error", err, err)
	}
	if got, want := ae.Code(), sqs.ErrCodeQueueDoesNotExist; got != want {
		return fmt.Errorf("got %q, want %q", got, want)
	}
	return nil
}

func (awsAsTest) MessageCheck(m *pubsub.Message) error {
	var sm sqs.Message
	if m.As(&sm) {
		return fmt.Errorf("cast succeeded for %T, want failure", &sm)
	}
	var psm *sqs.Message
	if !m.As(&psm) {
		return fmt.Errorf("cast failed for %T", &psm)
	}
	return nil
}

func (t awsAsTest) BeforeSend(as func(interface{}) bool) error {
	switch t.topicKind {
	case topicKindSNS, topicKindSNSRaw:
		var pub *sns.PublishInput
		if !as(&pub) {
			return fmt.Errorf("cast failed for %T", &pub)
		}
	case topicKindSQS:
		var smi *sqs.SendMessageInput
		if !as(&smi) {
			return fmt.Errorf("cast failed for %T", &smi)
		}
		var entry *sqs.SendMessageBatchRequestEntry
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
	ctx := context.Background()
	sess, err := session.NewSession(&aws.Config{
		HTTPClient: &http.Client{},
		Region:     aws.String(region),
		MaxRetries: aws.Int(0),
	})
	if err != nil {
		b.Fatal(err)
	}
	topicName := fmt.Sprintf("%s-topic", b.Name())
	dt, cleanup1, err := createTopic(ctx, topicName, sess, topicKind)
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
	ds, cleanup2, err := createSubscription(ctx, dt, subName, sess, topicKind)
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
