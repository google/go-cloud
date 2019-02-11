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
)

var (
	// See docs below on how to provision an Azure Service Bus Namespace and obtaining the connection string.
	// https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-dotnet-get-started-with-queues
	connString = os.Getenv("SERVICEBUS_CONNECTION_STRING")
)

const (
	topicName        = "test-topic"
	subscriptionName = "test-sub"
)

func ExampleOpenTopic() {

	ctx := context.Background()

	if connString == "" {
		log.Fatal("Service Bus ConnectionString is not set")
	}

	// Construct a Service Bus Namespace from a SAS Token.
	// See https://godoc.org/github.com/Azure/azure-service-bus-go#Namespace.
	ns, err := azurepubsub.NewNamespaceFromConnectionString(connString)
	logFatal(err)

	// Construct a Service Bus Topic for a topicName associated with a NameSpace.
	// See https://godoc.org/github.com/Azure/azure-service-bus-go#Topic.
	sbTopic, err := azurepubsub.NewTopic(ns, topicName, nil)
	logFatal(err)
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
	logFatal(err)
}

func ExampleOpenSubscription() {
	ctx := context.Background()

	if connString == "" {
		log.Fatal("Service Bus ConnectionString is not set")
	}

	// Construct a Service Bus Namespace from a SAS Token.
	// See https://godoc.org/github.com/Azure/azure-service-bus-go#Namespace.
	ns, err := azurepubsub.NewNamespaceFromConnectionString(connString)
	logFatal(err)

	// Construct a Service Bus Topic for a topicName associated with a NameSpace.
	// See https://godoc.org/github.com/Azure/azure-service-bus-go#Topic.
	sbTopic, err := azurepubsub.NewTopic(ns, topicName, nil)
	logFatal(err)
	defer sbTopic.Close(ctx)

	// Construct a Service Bus Subscription which is a child to a Service Bus Topic.
	// See https://godoc.org/github.com/Azure/azure-service-bus-go#Topic.NewSubscription.
	sbSub, err := azurepubsub.NewSubscription(sbTopic, subscriptionName, nil)
	logFatal(err)
	defer sbSub.Close(ctx)

	// Construct a *pubsub.Subscription for a given Service Bus NameSpace and Topic.
	s := azurepubsub.OpenSubscription(ctx, ns, sbTopic, sbSub, nil)

	// Receive a message from the *pubsub.Subscription backed by Service Bus.
	msg, err := s.Receive(ctx)
	logFatal(err)

	// Acknowledge the message, this operation issues a 'Complete' disposition on the Service Bus message.
	// See https://godoc.org/github.com/Azure/azure-service-bus-go#Message.Complete.
	msg.Ack()
}

func logFatal(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
