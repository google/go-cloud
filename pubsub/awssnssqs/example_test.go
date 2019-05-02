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

func ExampleOpenTopic() {
	// This example is used in https://gocloud.dev/howto/pubsub/publish/#sns-ctor

	// Variables set up elsewhere:
	ctx := context.Background()

	// Establish an AWS session.
	// See https://docs.aws.amazon.com/sdk-for-go/api/aws/session/ for more info.
	// The region must match the region for "MyTopic".
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-east-2"),
	})
	if err != nil {
		log.Fatal(err)
	}

	// Create a *pubsub.Topic.
	const topicARN = "arn:aws:sns:us-east-2:123456789012:MyTopic"
	topic := awssnssqs.OpenTopic(ctx, sess, topicARN, nil)
	defer topic.Shutdown(ctx)
}

func Example_openTopic() {
	// This example is used in https://gocloud.dev/howto/pubsub/publish/#sns

	// import _ "gocloud.dev/pubsub/awssnssqs"

	// Variables set up elsewhere:
	ctx := context.Background()

	const topicARN = "arn:aws:sns:us-east-2:123456789012:MyTopic"
	topic, err := pubsub.OpenTopic(ctx, "awssns://"+topicARN+"?region=us-east-2")
	if err != nil {
		log.Fatal(err)
	}
	defer topic.Shutdown(ctx)
}

func ExampleOpenSubscription() {
	// This example is used in https://gocloud.dev/howto/pubsub/subscribe/#sns-ctor

	// Variables set up elsewhere:
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

func Example_openSubscription() {
	// This example is used in https://gocloud.dev/howto/pubsub/subscribe/#sns

	// import _ "gocloud.dev/pubsub/awssnssqs"

	// Variables set up elsewhere:
	ctx := context.Background()

	// OpenSubscription creates a *pubsub.Subscription from a URL.
	// This URL will open the subscription with the URL
	// "https://sqs.us-east-2.amazonaws.com/123456789012/MyQueue".
	subscription, err := pubsub.OpenSubscription(ctx,
		"awssqs://sqs.us-east-2.amazonaws.com/123456789012/"+
			"MyQueue?region=us-east-2")
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)
}
