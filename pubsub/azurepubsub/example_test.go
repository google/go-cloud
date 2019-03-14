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

package azurepubsub_test

import (
	"context"
	"log"
	"os"

	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/azurepubsub"

	servicebus "github.com/Azure/azure-service-bus-go"
)

func ExampleOpenTopic() {

	ctx := context.Background()
	// See docs below on how to provision an Azure Service Bus Namespace and obtaining the connection string.
	// https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-dotnet-get-started-with-queues
	connString := os.Getenv("SERVICEBUS_CONNECTION_STRING")
	topicName := "test-topic"

	if connString == "" {
		log.Fatal("Service Bus ConnectionString is not set")
	}

	// Construct a Service Bus Namespace from a SAS Token.
	// See https://godoc.org/github.com/Azure/azure-service-bus-go#Namespace.
	ns, err := azurepubsub.NewNamespaceFromConnectionString(connString)
	if err != nil {
		log.Fatal(err)
	}

	// Construct a Service Bus Topic for a topicName associated with a NameSpace.
	// See https://godoc.org/github.com/Azure/azure-service-bus-go#Topic.
	sbTopic, err := azurepubsub.NewTopic(ns, topicName, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer sbTopic.Close(ctx)

	// Construct a *pubsub.Topic.
	t := azurepubsub.OpenTopic(ctx, sbTopic, nil)
	defer t.Shutdown(ctx)

	// Construct a *pubsub.Message.
	msg := &pubsub.Message{
		Body: []byte("example message"),
		Metadata: map[string]string{
			"Priority": "1",
		},
	}

	// Send *pubsub.Message from *pubsub.Topic backed by Azure Service Bus.
	err = t.Send(ctx, msg)
	if err != nil {
		log.Fatal(err)
	}
}

func ExampleOpenSubscription() {
	ctx := context.Background()
	// See docs below on how to provision an Azure Service Bus Namespace and obtaining the connection string.
	// https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-dotnet-get-started-with-queues
	connString := os.Getenv("SERVICEBUS_CONNECTION_STRING")
	topicName := "test-topic"
	subscriptionName := "test-sub"

	if connString == "" {
		log.Fatal("Service Bus ConnectionString is not set")
	}

	// Construct a Service Bus Namespace from a SAS Token.
	// See https://godoc.org/github.com/Azure/azure-service-bus-go#Namespace.
	ns, err := azurepubsub.NewNamespaceFromConnectionString(connString)
	if err != nil {
		log.Fatal(err)
	}

	// Construct a Service Bus Topic for a topicName associated with a NameSpace.
	// See https://godoc.org/github.com/Azure/azure-service-bus-go#Topic.
	sbTopic, err := azurepubsub.NewTopic(ns, topicName, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer sbTopic.Close(ctx)

	// Construct a Service Bus Subscription which is a child to a Service Bus Topic.
	// See https://godoc.org/github.com/Azure/azure-service-bus-go#Topic.NewSubscription.
	sbSub, err := azurepubsub.NewSubscription(sbTopic, subscriptionName, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer sbSub.Close(ctx)

	// Construct a *pubsub.Subscription for a given Service Bus NameSpace and Topic.
	s := azurepubsub.OpenSubscription(ctx, ns, sbTopic, sbSub, nil)

	// Receive a message from the *pubsub.Subscription backed by Service Bus.
	msg, err := s.Receive(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Acknowledge the message, this operation issues a 'Complete' disposition on the Service Bus message.
	// See https://godoc.org/github.com/Azure/azure-service-bus-go#Message.Complete.
	msg.Ack()
}
func Example_openfromURL() {
	ctx := context.Background()

	// OpenTopic creates a *pubsub.Topic from a URL.
	// This URL will open the topic "mytopic" using a connection string
	// from the environment variable SERVICEBUS_CONNECTION_STRING.
	t, err := pubsub.OpenTopic(ctx, "azuresb://mytopic")

	// Similarly, OpenSubscription creates a *pubsub.Subscription from a URL.
	// This URL will open the subscription "mysub" for the topic "mytopic".
	s, err := pubsub.OpenSubscription(ctx, "azuresb://mytopic?subscription=mysub")
	_, _, _ = t, s, err
}

func ExampleAckFuncForReceiveAndDelete() {

	ctx := context.Background()
	// See docs below on how to provision an Azure Service Bus Namespace and obtaining the connection string.
	// https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-dotnet-get-started-with-queues
	connString := os.Getenv("SERVICEBUS_CONNECTION_STRING")
	topicName := "test-topic"
	subscriptionName := "test-sub"

	if connString == "" {
		log.Fatal("Service Bus ConnectionString is not set")
	}

	// Construct a Service Bus Namespace from a SAS Token.
	// See https://godoc.org/github.com/Azure/azure-service-bus-go#Namespace.
	ns, err := azurepubsub.NewNamespaceFromConnectionString(connString)
	if err != nil {
		log.Fatal(err)
	}

	// Construct a Service Bus Topic for a topicName associated with a NameSpace.
	// See https://godoc.org/github.com/Azure/azure-service-bus-go#Topic.
	sbTopic, err := azurepubsub.NewTopic(ns, topicName, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer sbTopic.Close(ctx)

	// Construct Receiver to AutoDelete messages.
	// See https://godoc.org/github.com/Azure/azure-service-bus-go#SubscriptionWithReceiveAndDelete.
	var opts []servicebus.SubscriptionOption
	opts = append(opts, servicebus.SubscriptionWithReceiveAndDelete())

	// Construct a Service Bus Subscription which is a child to a Service Bus Topic.
	// See https://godoc.org/github.com/Azure/azure-service-bus-go#Topic.NewSubscription.
	sbSub, err := azurepubsub.NewSubscription(sbTopic, subscriptionName, opts)
	if err != nil {
		log.Fatal(err)
	}
	defer sbSub.Close(ctx)

	// This package accommodates both kinds of systems. If your application uses
	// at-least-once providers, it should always call Message.Ack. If your application
	// only uses at-most-once providers (ReceiveAndDeleteMode), it may call Message.Ack, but does not need to.
	//
	// For more information on Service Bus ReceiveMode, see https://godoc.org/github.com/Azure/azure-service-bus-go#SubscriptionWithReceiveAndDelete.
	//
	// Use the options.AckOverride to supply your own logic for acking (optional regardless of modes). The custom AckOverride function can perform logging, no-ops,
	// or your own bulk disposition operation. See https://godoc.org/github.com/Azure/azure-service-bus-go#Subscription.SendBatchDisposition.
	//
	// Important:
	// 1. AckOverride is optional for Service Bus Subscription in ReceiveAndDeleteMode (SubscriptionWithReceiveAndDelete option enabled).
	//    Calling the default driver Ack (opts.AckOverride is nil) in at-most-once mode will be disregarded by Azure Service Bus.
	// 2. AckOverride can be used for custom batch dispositions (SubscriptionWithReceiveAndDelete option not set) but not recommended.
	noop := func() {
	}
	subOpts := &azurepubsub.SubscriptionOptions{
		AckFuncForReceiveAndDelete: noop,
	}
	// Construct a *pubsub.Subscription for a given Service Bus NameSpace and Topic.
	s := azurepubsub.OpenSubscription(ctx, ns, sbTopic, sbSub, subOpts)

	// Construct a *pubsub.Topic.
	t := azurepubsub.OpenTopic(ctx, sbTopic, nil)
	defer t.Shutdown(ctx)

	// Send *pubsub.Message from *pubsub.Topic backed by Azure Service Bus.
	err = t.Send(ctx, &pubsub.Message{
		Body: []byte("example message"),
		Metadata: map[string]string{
			"Priority": "1",
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	// Receive a message from the *pubsub.Subscription backed by Service Bus.
	msg, err := s.Receive(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Ack will redirect to the AckOverride option (if provided), otherwise the driver Ack will be called.
	msg.Ack()
}
