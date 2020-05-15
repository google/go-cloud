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

package awssnssqs_test

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/awssnssqs"
)

func ExampleOpenSNSTopic() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// Establish an AWS session.
	// See https://docs.aws.amazon.com/sdk-for-go/api/aws/session/ for more info.
	// The region must match the region for the SNS topic "mytopic".
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-east-2"),
	})
	if err != nil {
		log.Fatal(err)
	}

	// Create a *pubsub.Topic.
	const topicARN = "arn:aws:sns:us-east-2:123456789012:mytopic"
	topic := awssnssqs.OpenSNSTopic(ctx, sess, topicARN, nil)
	defer topic.Shutdown(ctx)
}

func ExampleOpenSQSTopic() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// Establish an AWS session.
	// See https://docs.aws.amazon.com/sdk-for-go/api/aws/session/ for more info.
	// The region must match the region for the SQS queue "myqueue".
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-east-2"),
	})
	if err != nil {
		log.Fatal(err)
	}

	// Create a *pubsub.Topic.
	const queueURL = "https://sqs.us-east-2.amazonaws.com/123456789012/myqueue"
	topic := awssnssqs.OpenSQSTopic(ctx, sess, queueURL, nil)
	defer topic.Shutdown(ctx)
}

func Example_openSNSTopicFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/pubsub/awssnssqs"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	const topicARN = "arn:aws:sns:us-east-2:123456789012:mytopic"
	// Note the 3 slashes; ARNs have multiple colons and therefore aren't valid
	// as hostnames in the URL.
	topic, err := pubsub.OpenTopic(ctx, "awssns:///"+topicARN+"?region=us-east-2")
	if err != nil {
		log.Fatal(err)
	}
	defer topic.Shutdown(ctx)
}

func Example_openSQSTopicFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/pubsub/awssnssqs"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// https://docs.aws.amazon.com/sdk-for-net/v2/developer-guide/QueueURL.html
	const queueURL = "sqs.us-east-2.amazonaws.com/123456789012/myqueue"
	topic, err := pubsub.OpenTopic(ctx, "awssqs://"+queueURL+"?region=us-east-2")
	if err != nil {
		log.Fatal(err)
	}
	defer topic.Shutdown(ctx)
}

func ExampleOpenSubscription() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// Establish an AWS session.
	// See https://docs.aws.amazon.com/sdk-for-go/api/aws/session/ for more info.
	// The region must match the region for "MyQueue".
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-east-2"),
	})
	if err != nil {
		log.Fatal(err)
	}

	// Construct a *pubsub.Subscription.
	// https://docs.aws.amazon.com/sdk-for-net/v2/developer-guide/QueueURL.html
	const queueURL = "https://sqs.us-east-2.amazonaws.com/123456789012/MyQueue"
	subscription := awssnssqs.OpenSubscription(ctx, sess, queueURL, nil)
	defer subscription.Shutdown(ctx)
}

func Example_openSubscriptionFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/pubsub/awssnssqs"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// pubsub.OpenSubscription creates a *pubsub.Subscription from a URL.
	// This URL will open the subscription with the URL
	// "https://sqs.us-east-2.amazonaws.com/123456789012/myqueue".
	subscription, err := pubsub.OpenSubscription(ctx,
		"awssqs://sqs.us-east-2.amazonaws.com/123456789012/"+
			"myqueue?region=us-east-2")
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)
}
