---
title: "Subscribe to a Topic's Messages"
date: 2019-03-26T09:44:33-07:00
weight: 2
---

Subscribing to messages on a topic with the Go CDK takes three steps:

1. Open the subscription with the Pub/Sub provider of your choice (once per subscription).
2. Receive messages from the topic.
3. For each message, acknowledge its receipt using the `Ack` method
   after completing any work related to the message. This will prevent the
   message from being redelivered.

<!--more-->

The last two steps are the same across all providers because the first step
creates a value of the portable [`*pubsub.Subscription`][] type. A simple
subscriber that operates on messages serially looks like this:

{{< goexample src="gocloud.dev/pubsub.ExampleSubscription_Receive" imports="0" >}}

If you want your subscriber to operate on the incoming messages concurrently,
you can start multiple goroutines:

{{< goexample src="gocloud.dev/pubsub.ExampleSubscription_Receive_concurrent" imports="0" >}}

The rest of this guide will discuss how to accomplish the first step: opening
a subscription for your chosen Pub/Sub provider.

[`*pubsub.Subscription`]: https://godoc.org/gocloud.dev/pubsub#Subscription

## Constructors versus URL openers

If you know that your program is always going to use a particular Pub/Sub
provider or you need fine-grained control over the connection settings, you
should call the constructor function in the driver package directly (like
`gcppubsub.OpenSubscription`). However, if you want to change providers based
on configuration, you can use `pubsub.OpenSubscription`, making sure you
["blank import"][] the driver package to link it in. See the
[documentation on URLs][] for more details. This guide will show how to use
both forms for each pub/sub provider.

["blank import"]: https://golang.org/doc/effective_go.html#blank_import
[documentation on URLs]: {{< ref "/concepts/urls.md" >}}

## Amazon Simple Queueing Service {#sqs}

The Go CDK can subscribe to an Amazon [Simple Queueing Service][SQS] (SQS)
topic. SQS URLs closely resemble the the queue URL, except the leading
`https://` is replaced with `awssqs://`. You can specify the `region`
query parameter to ensure your application connects to the correct region, but
otherwise `pubsub.OpenSubscription` will use the region found in the environment
variables or your AWS CLI configuration.

{{< goexample "gocloud.dev/pubsub/awssnssqs.Example_openSubscriptionFromURL" >}}

Messages with a `base64encoded` message attribute will be automatically
[Base64][] decoded before being returned. See the [SNS publishing guide][]
or the [SQS publshing guide][] for more details.

[Base64]: https://en.wikipedia.org/wiki/Base64
[SNS publishing guide]: {{< ref "./publish.md#sns" >}}
[SQS publishing guide]: {{< ref "./publish.md#sqs" >}}
[SQS]: https://aws.amazon.com/sqs/

### Amazon Simple Queueing Service Constructor {#sqs-ctor}

The [`awssnssqs.OpenSubscription`][] constructor opens an SQS queue. You must
first create an [AWS session][] with the same region as your topic:

{{< goexample "gocloud.dev/pubsub/awssnssqs.ExampleOpenSubscription" >}}

[`awssnssqs.OpenSubscription`]: https://godoc.org/gocloud.dev/pubsub/awssnssqs#OpenSubscription
[AWS session]: https://docs.aws.amazon.com/sdk-for-go/api/aws/session/

## Google Cloud Pub/Sub {#gcp}

The Go CDK can receive messages from a Google [Cloud Pub/Sub][] subscription.
The URLs use the project ID and the subscription ID.
`pubsub.OpenSubscription` will use [Application Default Credentials][GCP creds].

{{< goexample "gocloud.dev/pubsub/gcppubsub.Example_openSubscription" >}}

[Cloud Pub/Sub]: https://cloud.google.com/pubsub/docs/
[GCP creds]: https://cloud.google.com/docs/authentication/production

### Google Cloud Pub/Sub Constructor {#gcp-ctor}

The [`gcppubsub.OpenSubscription`][] constructor opens a Cloud Pub/Sub
subscription. You must first obtain [GCP credentials][GCP creds] and then
create a gRPC connection to Cloud Pub/Sub. (This gRPC connection can be
reused among subscriptions.)

{{< goexample "gocloud.dev/pubsub/gcppubsub.ExampleOpenSubscription" >}}

[`gcppubsub.OpenSubscription`]: https://godoc.org/gocloud.dev/pubsub/gcppubsub#OpenSubscription

## Azure Service Bus {#azure}

The Go CDK can recieve messages from an [Azure Service Bus][] subscription
over [AMQP 1.0][]. The URL for subscribing is the topic name with the
subscription name in the `subscription` query parameter.
`pubsub.OpenSubscription` will use the environment variable
`SERVICEBUS_CONNECTION_STRING` to obtain the Service Bus Connection String
you need to copy [from the Azure portal][Azure connection string].

{{< goexample "gocloud.dev/pubsub/azuresb.Example_openSubscription" >}}

[AMQP 1.0]: https://www.amqp.org/
[Azure connection string]: https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-dotnet-how-to-use-topics-subscriptions#get-the-connection-string
[Azure Service Bus]: https://azure.microsoft.com/en-us/services/service-bus/

### Azure Service Bus Constructor {#azure-ctor}

The [`azuresb.OpenSubscription`][] constructor opens an Azure Service Bus
subscription. You must first connect to the topic and subscription using the
[Azure Service Bus library][] and then pass the subscription to
`azuresb.OpenSubscription`. There are also helper functions in the `azuresb`
package to make this easier.

{{< goexample "gocloud.dev/pubsub/azuresb.ExampleOpenSubscription" >}}

[`azuresb.OpenSubscription`]: https://godoc.org/gocloud.dev/pubsub/azuresb#OpenSubscription
[Azure Service Bus library]: https://github.com/Azure/azure-service-bus-go

## RabbitMQ {#rabbitmq}

The Go CDK can receive messages from an [AMQP 0.9.1][] queue, the dialect of
AMQP spoken by [RabbitMQ][]. A RabbitMQ URL only includes the queue name.
The RabbitMQ's server is discovered from the `RABBIT_SERVER_URL` environment
variable (which is something like `amqp://guest:guest@localhost:5672/`).

{{< goexample "gocloud.dev/pubsub/rabbitpubsub.Example_openSubscription" >}}

[AMQP 0.9.1]: https://www.rabbitmq.com/protocol.html
[RabbitMQ]: https://www.rabbitmq.com

### RabbitMQ Constructor {#rabbitmq-ctor}

The [`rabbitpubsub.OpenSubscription`][] constructor opens a RabbitMQ queue.
You must first create an [`*amqp.Connection`][] to your RabbitMQ instance.

{{< goexample "gocloud.dev/pubsub/rabbitpubsub.ExampleOpenSubscription" >}}

[`*amqp.Connection`]: https://godoc.org/github.com/streadway/amqp#Connection
[`rabbitpubsub.OpenSubscription`]: https://godoc.org/gocloud.dev/pubsub/rabbitpubsub#OpenSubscription

## NATS {#nats}

The Go CDK can publish to a [NATS][] subject. A NATS URL only includes the
subject name. The NATS server is discovered from the `NATS_SERVER_URL`
environment variable (which is something like `nats://nats.example.com`).

NATS guarantees at-most-once delivery, so there is no equivalent of `Ack`.
If your application only uses NATS and no other implementations (including
in-memory), you can skip calling `Ack` and add `?ackfunc=panic` to the end of
the URL you use to open a subscription. Otherwise, you should add
`?ackfunc=noop` to the end of your URL.

{{< goexample "gocloud.dev/pubsub/natspubsub.Example_openSubscription" >}}

To parse messages [published via the Go CDK][publish#nats], the NATS driver
will first attempt to decode the payload using [gob][]. Failing that, it will
return the message payload as the `Data` with no metadata to accomodate
subscribing to messages coming from a source not using the Go CDK.

[gob]: https://golang.org/pkg/encoding/gob/
[NATS]: https://nats.io/
[publish#nats]: {{< ref "./publish.md#nats" >}}

### NATS Constructor {#nats-ctor}

The [`natspubsub.OpenSubscription`][] constructor opens a NATS subject as a
topic. You must first create an [`*nats.Conn`][] to your NATS instance.

{{< goexample "gocloud.dev/pubsub/natspubsub.ExampleOpenSubscription" >}}

[`*nats.Conn`]: https://godoc.org/github.com/nats-io/go-nats#Conn
[`natspubsub.OpenSubscription`]: https://godoc.org/gocloud.dev/pubsub/natspubsub#OpenSubscription

## Kafka {#kafka}

The Go CDK can receive messages from a [Kafka][] cluster.
A Kafka URL includes the consumer group name, plus at least one instance
of a query parameter specifying the topic to subscribe to.
The brokers in the Kafka cluster are discovered from the
`KAFKA_BROKERS` environment variable (which is a comma-delimited list of
hosts, something like `1.2.3.4:9092,5.6.7.8:9092`).

{{< goexample "gocloud.dev/pubsub/kafkapubsub.Example_openSubscription" >}}

[Kafka]: https://kafka.apache.org/

### Kafka Constructor {#kafka-ctor}

The [`kafkapubsub.OpenSubscription`][] constructor creates a consumer in a
consumer group, subscribed to one or more topics.

In addition to the list of brokers, you'll need a [`*sarama.Config`][], which
exposes many knobs that can affect performance and semantics; review and set
them carefully. [`kafkapubsub.MinimalConfig`][] provides a minimal config to get
you started.

{{< goexample "gocloud.dev/pubsub/kafkapubsub.ExampleOpenSubscription" >}}

[`*sarama.Config`]: https://godoc.org/github.com/Shopify/sarama#Config
[`kafkapubsub.OpenSubscription`]: https://godoc.org/gocloud.dev/pubsub/kafkapubsub#OpenSubscription
[`kafkapubsub.MinimalConfig`]: https://godoc.org/gocloud.dev/pubsub/kafkapubsub#MinimalConfig

## In-Memory {#mem}

The Go CDK includes an in-memory Pub/Sub provider useful for local testing.
The names in `mem://` URLs are a process-wide namespace, so subscriptions to
the same name will receive messages posted to that topic. For instance, if
you open a topic `mem://topicA` and open two subscriptions with
`mem://topicA`, you will have two subscriptions to the same topic.

{{< goexample "gocloud.dev/pubsub/mempubsub.Example_openSubscription" >}}

### In-Memory Constructor {#mem-ctor}

To create a subscription to an in-memory Pub/Sub topic, pass the [topic you
created][publish-mem-ctor] into the [`mempubsub.NewSubscription` function][].
You will also need to pass an acknowledgement deadline: once a message is
received, if it is not acknowledged after the deadline elapses, then it will be
redelivered.

{{< goexample "gocloud.dev/pubsub/mempubsub.ExampleNewSubscription" >}}

[`mempubsub.NewSubscription` function]: https://godoc.org/gocloud.dev/pubsub/mempubsub#NewSubscription
[publish-mem-ctor]: {{< ref "./publish.md#mem-ctor" >}}

