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

	snsv2 "github.com/aws/aws-sdk-go-v2/service/sns"
	snstypesv2 "github.com/aws/aws-sdk-go-v2/service/sns/types"
	sqsv2 "github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypesv2 "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/smithy-go"
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
	useV2       bool
	sess        *session.Session
	snsClientV2 *snsv2.Client
	sqsClientV2 *sqsv2.Client
	topicKind   topicKind
	rt          http.RoundTripper
	closer      func()
	numTopics   uint32
	numSubs     uint32
}

func newHarness(ctx context.Context, t *testing.T, topicKind topicKind) (drivertest.Harness, error) {
	sess, rt, closer, _ := setup.NewAWSSession(ctx, t, region)
	return &harness{useV2: false, sess: sess, rt: rt, topicKind: topicKind, closer: closer}, nil
}

func newHarnessV2(ctx context.Context, t *testing.T, topicKind topicKind) (drivertest.Harness, error) {
	cfg, rt, closer, _ := setup.NewAWSv2Config(context.Background(), t, region)
	return &harness{useV2: true, snsClientV2: snsv2.NewFromConfig(cfg), sqsClientV2: sqsv2.NewFromConfig(cfg), rt: rt, topicKind: topicKind, closer: closer}, nil
}

func (h *harness) CreateTopic(ctx context.Context, testName string) (dt driver.Topic, cleanup func(), err error) {
	topicName := sanitize(fmt.Sprintf("%s-top-%d", testName, atomic.AddUint32(&h.numTopics, 1)))
	return createTopic(ctx, topicName, h.useV2, h.sess, h.snsClientV2, h.sqsClientV2, h.topicKind)
}

func createTopic(ctx context.Context, topicName string, useV2 bool, sess *session.Session, snsClientV2 *snsv2.Client, sqsClientV2 *sqsv2.Client, topicKind topicKind) (dt driver.Topic, cleanup func(), err error) {
	switch topicKind {
	case topicKindSNS, topicKindSNSRaw:
		// Create an SNS topic.
		if useV2 {
			out, err := snsClientV2.CreateTopic(ctx, &snsv2.CreateTopicInput{Name: aws.String(topicName)})
			if err != nil {
				return nil, nil, fmt.Errorf("creating SNS topic %q: %v", topicName, err)
			}
			dt = openSNSTopicV2(ctx, snsClientV2, *out.TopicArn, &TopicOptions{})
			cleanup = func() {
				snsClientV2.DeleteTopic(ctx, &snsv2.DeleteTopicInput{TopicArn: out.TopicArn})
			}
		} else {
			client := sns.New(sess)
			out, err := client.CreateTopicWithContext(ctx, &sns.CreateTopicInput{Name: aws.String(topicName)})
			if err != nil {
				return nil, nil, fmt.Errorf("creating SNS topic %q: %v", topicName, err)
			}
			dt = openSNSTopic(ctx, client, *out.TopicArn, &TopicOptions{})
			cleanup = func() {
				client.DeleteTopicWithContext(ctx, &sns.DeleteTopicInput{TopicArn: out.TopicArn})
			}
		}
		return dt, cleanup, nil
	case topicKindSQS:
		// Create an SQS queue.
		if useV2 {
			qURL, _, err := createSQSQueue(ctx, true, nil, sqsClientV2, topicName)
			if err != nil {
				return nil, nil, fmt.Errorf("creating SQS queue %q: %v", topicName, err)
			}
			dt = openSQSTopicV2(ctx, sqsClientV2, qURL, &TopicOptions{})
			cleanup = func() {
				sqsClientV2.DeleteQueue(ctx, &sqsv2.DeleteQueueInput{QueueUrl: aws.String(qURL)})
			}
		} else {
			sqsClient := sqs.New(sess)
			qURL, _, err := createSQSQueue(ctx, false, sqsClient, nil, topicName)
			if err != nil {
				return nil, nil, fmt.Errorf("creating SQS queue %q: %v", topicName, err)
			}
			dt = openSQSTopic(ctx, sqsClient, qURL, &TopicOptions{})
			cleanup = func() {
				sqsClient.DeleteQueueWithContext(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(qURL)})
			}
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
		if h.useV2 {
			return openSNSTopicV2(ctx, h.snsClientV2, fakeTopicARN, &TopicOptions{}), nil
		} else {
		}
		return openSNSTopic(ctx, sns.New(h.sess), fakeTopicARN, &TopicOptions{}), nil
	case topicKindSQS:
		const fakeQueueURL = "https://" + region + ".amazonaws.com/" + accountNumber + "/nonexistent-queue"
		if h.useV2 {
			return openSQSTopicV2(ctx, h.sqsClientV2, fakeQueueURL, &TopicOptions{}), nil
		} else {
		}
		return openSQSTopic(ctx, sqs.New(h.sess), fakeQueueURL, &TopicOptions{}), nil
	default:
		panic("unreachable")
	}
}

func (h *harness) CreateSubscription(ctx context.Context, dt driver.Topic, testName string) (ds driver.Subscription, cleanup func(), err error) {
	subName := sanitize(fmt.Sprintf("%s-sub-%d", testName, atomic.AddUint32(&h.numSubs, 1)))
	return createSubscription(ctx, dt, subName, h.useV2, h.sess, h.snsClientV2, h.sqsClientV2, h.topicKind)
}

func createSubscription(ctx context.Context, dt driver.Topic, subName string, useV2 bool, sess *session.Session, snsClientV2 *snsv2.Client, sqsClientV2 *sqsv2.Client, topicKind topicKind) (ds driver.Subscription, cleanup func(), err error) {
	switch topicKind {
	case topicKindSNS, topicKindSNSRaw:
		// Create an SQS queue, and subscribe it to the SNS topic.
		var qURL, qARN string
		var err error
		if useV2 {
			qURL, qARN, err = createSQSQueue(ctx, true, nil, sqsClientV2, subName)
			if err != nil {
				return nil, nil, fmt.Errorf("creating SQS queue %q: %v", subName, err)
			}
			ds = openSubscriptionV2(ctx, sqsClientV2, qURL, &SubscriptionOptions{})
		} else {
			sqsClient := sqs.New(sess)
			qURL, qARN, err = createSQSQueue(ctx, false, sqsClient, nil, subName)
			if err != nil {
				return nil, nil, fmt.Errorf("creating SQS queue %q: %v", subName, err)
			}
			ds = openSubscription(ctx, sqsClient, qURL, &SubscriptionOptions{})
		}

		snsTopicARN := dt.(*snsTopic).arn
		var cleanup func()
		if useV2 {
			req := &snsv2.SubscribeInput{
				TopicArn: aws.String(snsTopicARN),
				Endpoint: aws.String(qARN),
				Protocol: aws.String("sqs"),
			}
			// Enable RawMessageDelivery on the subscription if needed.
			if topicKind == topicKindSNSRaw {
				req.Attributes = map[string]string{"RawMessageDelivery": "true"}
			}
			out, err := snsClientV2.Subscribe(ctx, req)
			if err != nil {
				return nil, nil, fmt.Errorf("subscribing: %v", err)
			}
			cleanup = func() {
				snsClientV2.Unsubscribe(ctx, &snsv2.UnsubscribeInput{SubscriptionArn: out.SubscriptionArn})
				sqsClientV2.DeleteQueue(ctx, &sqsv2.DeleteQueueInput{QueueUrl: aws.String(qURL)})
			}
		} else {
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
			cleanup = func() {
				sqsClient := sqs.New(sess)
				snsClient.UnsubscribeWithContext(ctx, &sns.UnsubscribeInput{SubscriptionArn: out.SubscriptionArn})
				sqsClient.DeleteQueueWithContext(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(qURL)})
			}
		}
		return ds, cleanup, nil
	case topicKindSQS:
		// The SQS queue already exists; we created it for the topic. Re-use it
		// for the subscription.
		qURL := dt.(*sqsTopic).qURL
		if useV2 {
			return openSubscriptionV2(ctx, sqsClientV2, qURL, &SubscriptionOptions{}), func() {}, nil
		} else {
			return openSubscription(ctx, sqs.New(sess), qURL, &SubscriptionOptions{}), func() {}, nil
		}
	default:
		panic("unreachable")
	}
}

func createSQSQueue(ctx context.Context, useV2 bool, sqsClient *sqs.SQS, sqsClientV2 *sqsv2.Client, topicName string) (string, string, error) {
	var qURL string
	if useV2 {
		out, err := sqsClientV2.CreateQueue(ctx, &sqsv2.CreateQueueInput{QueueName: aws.String(topicName)})
		if err != nil {
			return "", "", fmt.Errorf("creating SQS queue %q: %v", topicName, err)
		}
		qURL = aws.StringValue(out.QueueUrl)
	} else {
		out, err := sqsClient.CreateQueueWithContext(ctx, &sqs.CreateQueueInput{QueueName: aws.String(topicName)})
		if err != nil {
			return "", "", fmt.Errorf("creating SQS queue %q: %v", topicName, err)
		}
		qURL = aws.StringValue(out.QueueUrl)
	}

	// Get the ARN.
	var qARN string
	if useV2 {
		out2, err := sqsClientV2.GetQueueAttributes(ctx, &sqsv2.GetQueueAttributesInput{
			QueueUrl:       aws.String(qURL),
			AttributeNames: []sqstypesv2.QueueAttributeName{"QueueArn"},
		})
		if err != nil {
			return "", "", fmt.Errorf("getting queue ARN for %s: %v", qURL, err)
		}
		qARN = out2.Attributes["QueueArn"]
	} else {
		out2, err := sqsClient.GetQueueAttributesWithContext(ctx, &sqs.GetQueueAttributesInput{
			QueueUrl:       aws.String(qURL),
			AttributeNames: []*string{aws.String("QueueArn")},
		})
		if err != nil {
			return "", "", fmt.Errorf("getting queue ARN for %s: %v", qURL, err)
		}
		qARN = aws.StringValue(out2.Attributes["QueueArn"])
	}

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
	var err error
	if useV2 {
		_, err = sqsClientV2.SetQueueAttributes(ctx, &sqsv2.SetQueueAttributesInput{
			Attributes: map[string]string{"Policy": queuePolicy},
			QueueUrl:   aws.String(qURL),
		})
	} else {
		_, err = sqsClient.SetQueueAttributesWithContext(ctx, &sqs.SetQueueAttributesInput{
			Attributes: map[string]*string{"Policy": &queuePolicy},
			QueueUrl:   aws.String(qURL),
		})
	}
	if err != nil {
		return "", "", fmt.Errorf("setting policy: %v", err)
	}
	return qURL, qARN, nil
}

func (h *harness) MakeNonexistentSubscription(ctx context.Context) (driver.Subscription, func(), error) {
	const fakeSubscriptionQueueURL = "https://" + region + ".amazonaws.com/" + accountNumber + "/nonexistent-subscription"
	if h.useV2 {
		return openSubscriptionV2(ctx, h.sqsClientV2, fakeSubscriptionQueueURL, &SubscriptionOptions{}), func() {}, nil
	} else {
		return openSubscription(ctx, sqs.New(h.sess), fakeSubscriptionQueueURL, &SubscriptionOptions{}), func() {}, nil
	}
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
	asTests := []drivertest.AsTest{awsAsTest{useV2: false, topicKind: topicKindSNS}}
	newSNSHarness := func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
		return newHarness(ctx, t, topicKindSNS)
	}
	drivertest.RunConformanceTests(t, newSNSHarness, asTests)
}

func TestConformanceSNSTopicV2(t *testing.T) {
	asTests := []drivertest.AsTest{awsAsTest{useV2: true, topicKind: topicKindSNS}}
	newSNSHarness := func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
		return newHarnessV2(ctx, t, topicKindSNS)
	}
	drivertest.RunConformanceTests(t, newSNSHarness, asTests)
}

func TestConformanceSNSTopicRaw(t *testing.T) {
	asTests := []drivertest.AsTest{awsAsTest{useV2: false, topicKind: topicKindSNSRaw}}
	newSNSHarness := func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
		return newHarness(ctx, t, topicKindSNSRaw)
	}
	drivertest.RunConformanceTests(t, newSNSHarness, asTests)
}

func TestConformanceSNSTopicRawV2(t *testing.T) {
	asTests := []drivertest.AsTest{awsAsTest{useV2: true, topicKind: topicKindSNSRaw}}
	newSNSHarness := func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
		return newHarnessV2(ctx, t, topicKindSNSRaw)
	}
	drivertest.RunConformanceTests(t, newSNSHarness, asTests)
}

func TestConformanceSQSTopic(t *testing.T) {
	asTests := []drivertest.AsTest{awsAsTest{useV2: false, topicKind: topicKindSQS}}
	newSQSHarness := func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
		return newHarness(ctx, t, topicKindSQS)
	}
	drivertest.RunConformanceTests(t, newSQSHarness, asTests)
}

func TestConformanceSQSTopicV2(t *testing.T) {
	asTests := []drivertest.AsTest{awsAsTest{useV2: true, topicKind: topicKindSQS}}
	newSQSHarness := func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
		return newHarnessV2(ctx, t, topicKindSQS)
	}
	drivertest.RunConformanceTests(t, newSQSHarness, asTests)
}

type awsAsTest struct {
	useV2     bool
	topicKind topicKind
}

func (awsAsTest) Name() string {
	return "aws test"
}

func (t awsAsTest) TopicCheck(topic *pubsub.Topic) error {
	switch t.topicKind {
	case topicKindSNS, topicKindSNSRaw:
		if t.useV2 {
			var s *snsv2.Client
			if !topic.As(&s) {
				return fmt.Errorf("cast failed for %T", s)
			}
		} else {
			var s *sns.SNS
			if !topic.As(&s) {
				return fmt.Errorf("cast failed for %T", s)
			}
		}
	case topicKindSQS:
		if t.useV2 {
			var s *sqsv2.Client
			if !topic.As(&s) {
				return fmt.Errorf("cast failed for %T", s)
			}
		} else {
			var s *sqs.SQS
			if !topic.As(&s) {
				return fmt.Errorf("cast failed for %T", s)
			}
		}
	default:
		panic("unreachable")
	}
	return nil
}

func (t awsAsTest) SubscriptionCheck(sub *pubsub.Subscription) error {
	if t.useV2 {
		var s *sqsv2.Client
		if !sub.As(&s) {
			return fmt.Errorf("cast failed for %T", s)
		}
	} else {
		var s *sqs.SQS
		if !sub.As(&s) {
			return fmt.Errorf("cast failed for %T", s)
		}
	}
	return nil
}

func (t awsAsTest) TopicErrorCheck(topic *pubsub.Topic, err error) error {
	if t.useV2 {
		var e smithy.APIError
		if !topic.ErrorAs(err, &e) {
			return errors.New("Topic.ErrorAs failed")
		}
		switch t.topicKind {
		case topicKindSNS, topicKindSNSRaw:
			if got, want := e.ErrorCode(), sns.ErrCodeNotFoundException; got != want {
				return fmt.Errorf("got %q, want %q", got, want)
			}
		case topicKindSQS:
			if got, want := e.ErrorCode(), sqs.ErrCodeQueueDoesNotExist; got != want {
				return fmt.Errorf("got %q, want %q", got, want)
			}
		default:
			panic("unreachable")
		}
		return nil
	}
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

func (t awsAsTest) SubscriptionErrorCheck(s *pubsub.Subscription, err error) error {
	if t.useV2 {
		var e smithy.APIError
		if !s.ErrorAs(err, &e) {
			return errors.New("Subscription.ErrorAs failed")
		}
		if got, want := e.ErrorCode(), sqs.ErrCodeQueueDoesNotExist; got != want {
			return fmt.Errorf("got %q, want %q", got, want)
		}
		return nil
	}
	var ae awserr.Error
	if !s.ErrorAs(err, &ae) {
		return fmt.Errorf("failed to convert %v (%T) to an awserr.Error", err, err)
	}
	if got, want := ae.Code(), sqs.ErrCodeQueueDoesNotExist; got != want {
		return fmt.Errorf("got %q, want %q", got, want)
	}
	return nil
}

func (t awsAsTest) MessageCheck(m *pubsub.Message) error {
	if t.useV2 {
		var sm sqstypesv2.Message
		if !m.As(&sm) {
			return fmt.Errorf("cast failed for %T", &sm)
		}
	} else {
		var sm sqs.Message
		if m.As(&sm) {
			return fmt.Errorf("cast succeeded for %T, want failure", &sm)
		}
		var psm *sqs.Message
		if !m.As(&psm) {
			return fmt.Errorf("cast failed for %T", &psm)
		}
	}
	return nil
}

func (t awsAsTest) BeforeSend(as func(interface{}) bool) error {
	switch t.topicKind {
	case topicKindSNS, topicKindSNSRaw:
		if t.useV2 {
			var pub *snsv2.PublishInput
			if !as(&pub) {
				return fmt.Errorf("cast failed for %T", &pub)
			}
			var entry *snstypesv2.PublishBatchRequestEntry
			if !as(&entry) {
				return fmt.Errorf("cast failed for %T", &entry)
			}
		} else {
			var pub *sns.PublishInput
			if !as(&pub) {
				return fmt.Errorf("cast failed for %T", &pub)
			}
			var entry *sns.PublishBatchRequestEntry
			if !as(&entry) {
				return fmt.Errorf("cast failed for %T", &entry)
			}
		}
	case topicKindSQS:
		if t.useV2 {
			var entry *sqstypesv2.SendMessageBatchRequestEntry
			if !as(&entry) {
				return fmt.Errorf("cast failed for %T", &entry)
			}
		} else {
			var smi *sqs.SendMessageInput
			if !as(&smi) {
				return fmt.Errorf("cast failed for %T", &smi)
			}
			var entry *sqs.SendMessageBatchRequestEntry
			if !as(&entry) {
				return fmt.Errorf("cast failed for %T", &entry)
			}
		}
	default:
		panic("unreachable")
	}
	return nil
}

func (t awsAsTest) AfterSend(as func(interface{}) bool) error {
	switch t.topicKind {
	case topicKindSNS, topicKindSNSRaw:
		if t.useV2 {
			var pub *snsv2.PublishOutput
			if !as(&pub) {
				return fmt.Errorf("cast failed for %T", &pub)
			}
			var entry snstypesv2.PublishBatchResultEntry
			if !as(&entry) {
				return fmt.Errorf("cast failed for %T", &entry)
			}
		} else {
			var pub *sns.PublishOutput
			if !as(&pub) {
				return fmt.Errorf("cast failed for %T", &pub)
			}
			var entry sns.PublishBatchResultEntry
			if !as(&entry) {
				return fmt.Errorf("cast failed for %T", &entry)
			}
		}
	case topicKindSQS:
		if t.useV2 {
			var entry sqstypesv2.SendMessageBatchResultEntry
			if !as(&entry) {
				return fmt.Errorf("cast failed for %T", &entry)
			}
		} else {
			var entry *sqs.SendMessageBatchResultEntry
			if !as(&entry) {
				return fmt.Errorf("cast failed for %T", &entry)
			}
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
	dt, cleanup1, err := createTopic(ctx, topicName, false, sess, nil, nil, topicKind)
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
	ds, cleanup2, err := createSubscription(ctx, dt, subName, false, sess, nil, nil, topicKind)
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
		// OK, setting usev2.
		{"awssns:///arn:aws:service:region:accountid:resourceType/resourcePath?awssdk=v2", false},
		// Invalid parameter.
		{"awssns:///arn:aws:service:region:accountid:resourceType/resourcePath?param=value", true},

		// SQS...
		// OK.
		{"awssqs://sqs.us-east-2.amazonaws.com/99999/my-queue", false},
		// OK, setting region.
		{"awssqs://sqs.us-east-2.amazonaws.com/99999/my-queue?region=us-east-2", false},
		// OK, setting usev2.
		{"awssqs://sqs.us-east-2.amazonaws.com/99999/my-queue?awssdk=v2", false},
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
		// OK, setting usev2.
		{"awssqs://sqs.us-east-2.amazonaws.com/99999/my-queue?awssdk=v2", false},
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
