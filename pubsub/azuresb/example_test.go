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

package azuresb_test

import (
	"context"
	"log"
	"os"

	servicebus "github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/azuresb"
)

func ExampleOpenTopic() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// Change these as needed for your application.
	connString := os.Getenv("SERVICEBUS_CONNECTION_STRING")
	topicName := "test-topic"

	if connString == "" {
		log.Fatal("Service Bus ConnectionString is not set")
	}

	// Connect to Azure Service Bus for the given topic.
	sbClient, err := azuresb.NewClientFromConnectionString(connString, nil)
	if err != nil {
		log.Fatal(err)
	}
	sbSender, err := azuresb.NewSender(sbClient, topicName, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer sbSender.Close(ctx)

	// Construct a *pubsub.Topic.
	topic, err := azuresb.OpenTopic(ctx, sbSender, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer topic.Shutdown(ctx)
}

func Example_openTopicFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/pubsub/azuresb"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// pubsub.OpenTopic creates a *pubsub.Topic from a URL.
	// This URL will open the topic "mytopic" using a connection string
	// from the environment variable SERVICEBUS_CONNECTION_STRING.
	topic, err := pubsub.OpenTopic(ctx, "azuresb://mytopic")
	if err != nil {
		log.Fatal(err)
	}
	defer topic.Shutdown(ctx)
}

func ExampleOpenSubscription() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// Change these as needed for your application.
	serviceBusConnString := os.Getenv("SERVICEBUS_CONNECTION_STRING")
	const topicName = "test-topic"
	const subscriptionName = "test-subscription"

	// Connect to Azure Service Bus for the given subscription.
	sbClient, err := azuresb.NewClientFromConnectionString(serviceBusConnString, nil)
	if err != nil {
		log.Fatal(err)
	}
	sbReceiver, err := azuresb.NewReceiver(sbClient, topicName, subscriptionName, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer sbReceiver.Close(ctx)

	// Construct a *pubsub.Subscription.
	subscription, err := azuresb.OpenSubscription(ctx, sbClient, sbReceiver, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)
}

func Example_openSubscriptionFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/pubsub/azuresb"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// pubsub.OpenSubscription creates a *pubsub.Subscription from a URL.
	// This URL will open the subscription "mysubscription" for the topic
	// "mytopic" using a connection string from the environment variable
	// SERVICEBUS_CONNECTION_STRING.
	subscription, err := pubsub.OpenSubscription(ctx,
		"azuresb://mytopic?subscription=mysubscription")
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)
}

func Example_OpenSubscription_inReceiveAndDeleteMode() {

	ctx := context.Background()

	// Change these as needed for your application.
	serviceBusConnString := os.Getenv("SERVICEBUS_CONNECTION_STRING")
	const topicName = "test-topic"
	const subscriptionName = "test-subscription"

	// Connect to Azure Service Bus for the given subscription.
	sbClient, err := azuresb.NewClientFromConnectionString(serviceBusConnString, nil)
	if err != nil {
		log.Fatal(err)
	}
	// Create the azuresb.Subscription, configuring it with the
	// ReceiveAndDelete option.
	// See https://godoc.org/github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus#SubscriptionWithReceiveAndDelete.
	sbReceiverOptions := &servicebus.ReceiverOptions{
		ReceiveMode: servicebus.ReceiveModeReceiveAndDelete,
	}
	sbReceiver, err := azuresb.NewReceiver(sbClient, topicName, subscriptionName, sbReceiverOptions)
	if err != nil {
		log.Fatal(err)
	}
	defer sbReceiver.Close(ctx)

	// Construct a *pubsub.Subscription. Since we configured
	// the azuresb.Subscription with ReceiveAndDelete mode, we need
	// to set SubscriptionOptions.ReceiveAndDelete = true.
	subscription, err := azuresb.OpenSubscription(ctx,
		sbClient, sbReceiver, &azuresb.SubscriptionOptions{ReceiveAndDelete: true})
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)
}
