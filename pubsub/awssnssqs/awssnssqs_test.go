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

const region = "us-east-2"

func TestOpenTopic(t *testing.T) {
	ctx := context.Background()
	sess, err := newSession()
	if err != nil {
		t.Fatal(err)
	}
	client := sns.New(sess, &aws.Config{})
	fakeTopicARN := ""
	topic := OpenTopic(ctx, client, fakeTopicARN, nil)
	if err := topic.Send(ctx, &pubsub.Message{Body: []byte("")}); err == nil {
		t.Error("got nil, want error from send to nonexistent topic")
	}
}

func TestOpenSubscription(t *testing.T) {
	ctx := context.Background()
	sess, err := newSession()
	if err != nil {
		t.Fatal(err)
	}
	client := sqs.New(sess, &aws.Config{})
	fakeQURL := ""
	sub := OpenSubscription(ctx, client, fakeQURL, nil)
	if _, err := sub.Receive(ctx); err == nil {
		t.Error("got nil, want error from receive from nonexistent subscription")
	}
}

func newSession() (*session.Session, error) {
	return session.NewSession(&aws.Config{
		HTTPClient: &http.Client{},
		Region:     aws.String(region),
		MaxRetries: aws.Int(0),
	})
}

type harness struct {
	sess      *session.Session
	rt        http.RoundTripper
	closer    func()
	numTopics uint32
	numSubs   uint32
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	sess, rt, done := setup.NewAWSSession2(ctx, t, region)
	return &harness{sess: sess, rt: rt, closer: done, numTopics: 0, numSubs: 0}, nil
}

func (h *harness) CreateTopic(ctx context.Context, testName string) (dt driver.Topic, cleanup func(), err error) {
	topicName := fmt.Sprintf("%s-topic-%d", sanitize(testName), atomic.AddUint32(&h.numTopics, 1))
	return createTopic(ctx, topicName, h.sess)
}

func createTopic(ctx context.Context, topicName string, sess *session.Session) (dt driver.Topic, cleanup func(), err error) {
	client := sns.New(sess)
	out, err := client.CreateTopic(&sns.CreateTopicInput{Name: aws.String(topicName)})
	if err != nil {
		return nil, nil, fmt.Errorf(`creating topic "%s": %v`, topicName, err)
	}
	dt = openTopic(ctx, client, *out.TopicArn, nil)
	cleanup = func() {
		client.DeleteTopic(&sns.DeleteTopicInput{TopicArn: out.TopicArn})
	}
	return dt, cleanup, nil
}

func (h *harness) MakeNonexistentTopic(ctx context.Context) (driver.Topic, error) {
	client := sns.New(h.sess)
	dt := openTopic(ctx, client, "nonexistent-topic", nil)
	return dt, nil
}

func (h *harness) CreateSubscription(ctx context.Context, dt driver.Topic, testName string) (ds driver.Subscription, cleanup func(), err error) {
	subName := fmt.Sprintf("%s-subscription-%d", sanitize(testName), atomic.AddUint32(&h.numSubs, 1))
	return createSubscription(ctx, dt, subName, h.sess)
}

func createSubscription(ctx context.Context, dt driver.Topic, subName string, sess *session.Session) (ds driver.Subscription, cleanup func(), err error) {
	sqsClient := sqs.New(sess)
	out, err := sqsClient.CreateQueue(&sqs.CreateQueueInput{QueueName: aws.String(subName)})
	if err != nil {
		return nil, nil, fmt.Errorf(`creating subscription queue "%s": %v`, subName, err)
	}
	ds = openSubscription(ctx, sqsClient, *out.QueueUrl)

	snsClient := sns.New(sess, &aws.Config{})
	cleanupSub, err := subscribeQueueToTopic(ctx, sqsClient, snsClient, out.QueueUrl, dt)
	if err != nil {
		return nil, nil, fmt.Errorf("subscribing: %v", err)
	}
	cleanup = func() {
		sqsClient.DeleteQueue(&sqs.DeleteQueueInput{QueueUrl: out.QueueUrl})
		cleanupSub()
	}
	return ds, cleanup, nil
}

func subscribeQueueToTopic(ctx context.Context, sqsClient *sqs.SQS, snsClient *sns.SNS, qURL *string, dt driver.Topic) (func(), error) {
	out2, err := sqsClient.GetQueueAttributes(&sqs.GetQueueAttributesInput{
		QueueUrl:       qURL,
		AttributeNames: []*string{aws.String("QueueArn")},
	})
	if err != nil {
		return nil, fmt.Errorf("getting queue ARN for %s: %v", *qURL, err)
	}
	qARN := out2.Attributes["QueueArn"]

	t := dt.(*topic)
	subOut, err := snsClient.Subscribe(&sns.SubscribeInput{
		TopicArn: aws.String(t.arn),
		Endpoint: qARN,
		Protocol: aws.String("sqs"),
	})
	if err != nil {
		return nil, fmt.Errorf("subscribing: %v", err)
	}
	cleanup := func() {
		_, _ = snsClient.Unsubscribe(&sns.UnsubscribeInput{
			SubscriptionArn: subOut.SubscriptionArn,
		})
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
"Resource": "` + *qARN + `",
"Condition": {
"ArnEquals": {
"aws:SourceArn": "` + t.arn + `"
}
}
}
]
}`
	_, err = sqsClient.SetQueueAttributes(&sqs.SetQueueAttributesInput{
		Attributes: map[string]*string{"Policy": &queuePolicy},
		QueueUrl:   qURL,
	})
	if err != nil {
		return nil, fmt.Errorf("setting policy: %v", err)
	}

	return cleanup, nil
}

func (h *harness) MakeNonexistentSubscription(ctx context.Context) (driver.Subscription, error) {
	client := sqs.New(h.sess)
	ds := openSubscription(ctx, client, "nonexistent-subscription")
	return ds, nil
}

func (h *harness) Close() {
	h.closer()
}

// Tips on dealing with failures when in -record mode:
// - There may be leftover messages in queues. Using the AWS CLI tool,
//   purge the queues before running the test.
//   E.g.
//	     aws sqs purge-queue --queue-url URL
//   You can get the queue URLs with
//       aws sqs list-queues

func TestConformance(t *testing.T) {
	asTests := []drivertest.AsTest{awsAsTest{}}
	drivertest.RunConformanceTests(t, newHarness, asTests)
}

type awsAsTest struct{}

func (awsAsTest) Name() string {
	return "aws test"
}

func (awsAsTest) TopicCheck(top *pubsub.Topic) error {
	var s *sns.SNS
	if !top.As(&s) {
		return fmt.Errorf("cast failed for %T", s)
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

func (awsAsTest) TopicErrorCheck(t *pubsub.Topic, err error) error {
	var ae awserr.Error
	if !t.ErrorAs(err, &ae) {
		return fmt.Errorf("failed to convert %v (%T) to an awserr.Error", err, err)
	}
	// It seems like it should be ErrCodeNotFoundException but that's not what AWS gives back.
	if got, want := ae.Code(), sns.ErrCodeInvalidParameterException; got != want {
		return fmt.Errorf("got %q, want %q", got, want)
	}
	return nil
}

func (awsAsTest) SubscriptionErrorCheck(s *pubsub.Subscription, err error) error {
	var ae awserr.Error
	if !s.ErrorAs(err, &ae) {
		return fmt.Errorf("failed to convert %v (%T) to an awserr.Error", err, err)
	}
	// It seems like it should be ErrCodeNotFoundException but that's not what AWS gives back.
	if got, want := ae.Code(), "InvalidAddress"; got != want {
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

func sanitize(testName string) string {
	return strings.Replace(testName, "/", "_", -1)
}

// The first run will hang because the SQS queue is not yet subscribed to the
// SNS topic. Go to console.aws.amazon.com and manually subscribe the queue
// to the topic and then rerun this benchmark to get results.
func BenchmarkAwsPubSub(b *testing.B) {
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
	dt, cleanup1, err := createTopic(ctx, topicName, sess)
	if err != nil {
		b.Fatal(err)
	}
	defer cleanup1()
	topic := pubsub.NewTopic(dt, nil)
	defer topic.Shutdown(ctx)
	subName := fmt.Sprintf("%s-subscription", b.Name())
	ds, cleanup2, err := createSubscription(ctx, dt, subName, sess)
	if err != nil {
		b.Fatal(err)
	}
	defer cleanup2()
	sub := pubsub.NewSubscription(ds, nil)
	defer sub.Shutdown(ctx)
	drivertest.RunBenchmarks(b, topic, sub)
}

func TestOpenTopicFromURL(t *testing.T) {
	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"awssnssqs://arn:aws:service:region:accountid:resourceType/resourcePath", false},
		// OK, setting region.
		{"awssnssqs://arn:aws:service:region:accountid:resourceType/resourcePath?region=us-east-2", false},
		// Invalid parameter.
		{"awssnssqs://arn:aws:service:region:accountid:resourceType/resourcePath?param=value", true},
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
	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"awssnssqs://sqs.us-east-2.amazonaws.com/99999/my-subscription", false},
		// OK, setting region.
		{"awssnssqs://sqs.us-east-2.amazonaws.com/99999/my-subscription?region=us-east-2", false},
		// Invalid parameter.
		{"awssnssqs://sqs.us-east-2.amazonaws.com/99999/my-subscription?param=value", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		_, err := pubsub.OpenSubscription(ctx, test.URL)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
	}
}
