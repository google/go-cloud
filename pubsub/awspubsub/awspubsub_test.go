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

package awspubsub

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/googleapis/gax-go"
	"gocloud.dev/internal/batcher"
	"gocloud.dev/internal/retry"

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
	subscribeQueueToTopic(ctx, sqsClient, snsClient, out.QueueUrl, dt)
	cleanup = func() {
		sqsClient.DeleteQueue(&sqs.DeleteQueueInput{QueueUrl: out.QueueUrl})
	}
	return ds, cleanup, nil
}

// ackBatcher is a trivial batcher that sends off items as singleton batches.
type ackBatcher struct {
	handler func(items interface{}) error
}

func (ab *ackBatcher) Add(ctx context.Context, item interface{}) error {
	item2 := item.(driver.AckID)
	items := []driver.AckID{item2}
	return ab.handler(items)
}

func (ab *ackBatcher) AddNoWait(item interface{}) <-chan error {
	item2 := item.(driver.AckID)
	items := []driver.AckID{item2}
	c := make(chan error)
	go func() {
		c <- ab.handler(items)
	}()
	return c
}

func (ab *ackBatcher) Shutdown() {
}

func subscribeQueueToTopic(ctx context.Context, sqsClient *sqs.SQS, snsClient *sns.SNS, qURL *string, dt driver.Topic) error {
	out2, err := sqsClient.GetQueueAttributes(&sqs.GetQueueAttributesInput{
		QueueUrl:       qURL,
		AttributeNames: []*string{aws.String("QueueArn")},
	})
	if err != nil {
		return fmt.Errorf("getting queue ARN for %s: %v", *qURL, err)
	}
	qARN := out2.Attributes["QueueArn"]

	t := dt.(*topic)
	_, err = snsClient.Subscribe(&sns.SubscribeInput{
		TopicArn: aws.String(t.arn),
		Endpoint: qARN,
		Protocol: aws.String("sqs"),
	})
	if err != nil {
		return fmt.Errorf("subscribing: %v", err)
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
		return fmt.Errorf("setting policy: %v", err)
	}

	return nil
}

func makeAckBatcher(ctx context.Context, ds driver.Subscription, setPermanentError func(error)) driver.Batcher {
	const maxHandlers = 1
	h := func(items interface{}) error {
		ids := items.([]driver.AckID)
		err := retry.Call(ctx, gax.Backoff{}, ds.IsRetryable, func() error {
			return ds.SendAcks(ctx, ids)
		})
		if err != nil {
			setPermanentError(err)
		}
		return err
	}
	b := batcher.New(reflect.TypeOf([]driver.AckID{}).Elem(), maxHandlers, h)
	return &wrappedBatcher{b}
	// return &simpleBatcher{handler: h, batch: nil}
}

type wrappedBatcher struct {
	b *batcher.Batcher
}

// Add adds an item to the batcher.
func (wb *wrappedBatcher) Add(ctx context.Context, item interface{}) error {
	return wb.b.Add(ctx, item)
}

// AddNoWait adds an item to the batcher. Unlike the method with the
// same name on the production batcher (internal/batcher), this method
// blocks in order to make acking and receiving happen in a deterministic
// order, to support record/replay.
func (wb *wrappedBatcher) AddNoWait(item interface{}) <-chan error {
	c := make(chan error, 1)
	defer close(c)
	c <- wb.b.Add(context.Background(), item)
	return c
}

// Shutdown waits for all active calls to Add to finish, then returns. After
// Shutdown is called, all calls to Add fail.
func (wb *wrappedBatcher) Shutdown() {
	wb.b.Shutdown()
}

type simpleBatcher struct {
	mu   sync.Mutex
	done bool

	handler func(items interface{}) error
}

// Add adds an item to the batcher.
func (sb *simpleBatcher) Add(ctx context.Context, item interface{}) error {
	c := sb.AddNoWait(item)
	// Wait until either our result is ready or the context is done.
	select {
	case err := <-c:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// AddNoWait adds an item to the batcher. Unlike the method with the
// same name on the production batcher (internal/batcher), this method
// blocks in order to make acking and receiving happen in a deterministic
// order, to support record/replay.
func (sb *simpleBatcher) AddNoWait(item interface{}) <-chan error {
	c := make(chan error, 1)
	defer close(c)
	if sb.done {
		c <- errors.New("tried to add an item to a simpleBatcher after shutdown")
		return c
	}
	m := item.(*pubsub.Message)
	batch := []*pubsub.Message{m}
	err := sb.handler(batch)
	if err != nil {
		c <- err
	}
	return c
}

// Shutdown waits for all active calls to Add to finish, then returns. After
// Shutdown is called, all calls to Add fail.
func (sb *simpleBatcher) Shutdown() {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.done = true
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
