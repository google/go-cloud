// Copyright 2018 The Go Cloud Authors
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
	"github.com/googleapis/gax-go"
	"gocloud.dev/internal/batcher"
	"gocloud.dev/internal/retry"
	"net/http"
	"reflect"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sqs"
	"gocloud.dev/internal/testing/setup"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
	"gocloud.dev/pubsub/drivertest"
)

const (
	// These constants capture values that were used during the last -record.
	//
	// If you want to use --record mode,
	// 1a. Create an SNS topic in your AWS project by browsing to
	//    https://console.aws.amazon.com/sns/v2/home
	//    and clicking "Topics", "Create new topic".
	// 1b. Create a subscription queue by browsing to
	//    https://console.aws.amazon.com/sqs/home
	//    and clicking "Create New Queue", typing the queue name, then
	//    "Quick-Create Queue".
	// 2. Update the topicARN constant to your topic ARN, and the
	//    qURL to your queue URL.
	topicARN = "arn:aws:sns:us-east-2:221420415498:test-topic"
	region   = "us-east-2"
)

var qURLs = []string{
	"https://sqs.us-east-2.amazonaws.com/221420415498/test-q-0",
	"https://sqs.us-east-2.amazonaws.com/221420415498/test-q-1",
}

type harness struct {
	sess   *session.Session
	cfg    *aws.Config
	rt     http.RoundTripper
	closer func()
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	sess, rt, done := setup.NewAWSSession(t, region)
	return &harness{sess: sess, cfg: &aws.Config{}, rt: rt, closer: done}, nil
}

func (h *harness) MakeTopic(ctx context.Context) (driver.Topic, error) {
	client := sns.New(h.sess, h.cfg)
	dt := openTopic(ctx, client, topicARN)
	return dt, nil
}

func (h *harness) MakeNonexistentTopic(ctx context.Context) (driver.Topic, error) {
	client := sns.New(h.sess, h.cfg)
	dt := openTopic(ctx, client, "nonexistent-topic")
	return dt, nil
}

func (h *harness) MakeSubscription(ctx context.Context, dt driver.Topic, n int) (driver.Subscription, error) {
	client := sqs.New(h.sess, h.cfg)
	u := qURLs[n]
	ds := openSubscription(ctx, client, u)
	return ds, nil
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
	mu      sync.Mutex
	done    bool

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
	client := sqs.New(h.sess, h.cfg)
	ds := openSubscription(ctx, client, "nonexistent-subscription")
	return ds, nil
}

func (h *harness) Close() {
	h.closer()
}

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
