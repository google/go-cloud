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
	"fmt"
	"testing"

	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sqs"
	"gocloud.dev/internal/pubsub"
	"gocloud.dev/internal/pubsub/driver"
	"gocloud.dev/internal/pubsub/drivertest"
	"gocloud.dev/internal/testing/setup"
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
	"https://sqs.us-east-2.amazonaws.com/221420415498/test-subscription-1",
	"https://sqs.us-east-2.amazonaws.com/221420415498/test-subscription-2",
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

func (h *harness) MakeSubscription(ctx context.Context, dt driver.Topic, i int) (driver.Subscription, error) {
	client := sqs.New(h.sess, h.cfg)
	u := qURLs[i+1]
	ds := openSubscription(ctx, client, u)
	return ds, nil
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
