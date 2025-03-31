---
title: "Subscribe to Messages from a Topic"
date: 2019-03-26T09:44:33-07:00
lastmod: 2019-07-29T12:00:00-07:00
weight: 2
toc: true
---

Subscribing to receive message from a topic with the Go CDK takes three steps:

1. [Open a subscription][] to a topic with the Pub/Sub service of your choice (once per
   subscription).
2. [Receive and acknowledge messages][] from the topic. After completing any
   work related to the message, use the Ack method to prevent it from being
   redelivered.

[Open a subscription]: {{< ref "#opening" >}}
[Receive and acknowledge messages]: {{< ref "#receiving" >}}

<!--more-->

## Opening a Subscription {#opening}

The first step in subscribing to receive messages from a topic is
to instantiate a portable [`*pubsub.Subscription`][] for your service.

The easiest way to do so is to use [`pubsub.OpenSubscription`][]
and a service-specific URL pointing to the topic, making sure you
["blank import"][] the driver package to link it in.

```go
import (
    "context"

    "gocloud.dev/pubsub"
    _ "gocloud.dev/pubsub/<driver>"
)
...
ctx := context.Background()
subs, err := pubsub.OpenSubscription(ctx, "<driver-url>")
if err != nil {
    return fmt.Errorf("could not open topic subscription: %v", err)
}
defer subs.Shutdown(ctx)
// subs is a *pubsub.Subscription; see usage below
...
```

See [Concepts: URLs][] for general background and the [guide below][]
for URL usage for each supported service.

Alternatively, if you need fine-grained
control over the connection settings, you can call the constructor function in
the driver package directly (like `gcppubsub.OpenSubscription`).

```go
import "gocloud.dev/pubsub/<driver>"
...
subs, err := <driver>.OpenSubscription(...)
...
```

You may find the [`wire` package][] useful for managing your initialization code
when switching between different backing services.

See the [guide below][] for constructor usage for each supported service.

[guide below]: {{< ref "#services" >}}
[`pubsub.OpenSubscription`]:
https://godoc.org/gocloud.dev/pubsub#OpenTopic
["blank import"]: https://golang.org/doc/effective_go.html#blank_import
[Concepts: URLs]: {{< ref "/concepts/urls.md" >}}
[`wire` package]: http://github.com/google/wire

## Receiving and Acknowledging Messages {#receiving}

A simple subscriber that operates on
[messages](https://godoc.org/gocloud.dev/pubsub#Message) serially looks like
this:

{{< goexample src="gocloud.dev/pubsub.ExampleSubscription_Receive" imports="0" >}}

If you want your subscriber to operate on incoming messages concurrently,
you can start multiple goroutines:

{{< goexample src="gocloud.dev/pubsub.ExampleSubscription_Receive_concurrent" imports="0" >}}

Note that the [semantics of message delivery][] can vary by backing service.

[`*pubsub.Subscription`]: https://godoc.org/gocloud.dev/pubsub#Subscription
[semantics of message delivery]: https://godoc.org/gocloud.dev/pubsub#hdr-At_most_once_and_At_least_once_Delivery

## Other Usage Samples

* [CLI Sample](https://github.com/google/go-cloud/tree/master/samples/gocdk-pubsub)
* [Order Processor sample](https://gocloud.dev/tutorials/order/)
* [pubsub package examples](https://godoc.org/gocloud.dev/pubsub#pkg-examples)

## Supported Pub/Sub Services {#services}

### Google Cloud Pub/Sub {#gcp}

The Go CDK can receive messages from a Google [Cloud Pub/Sub][] subscription.
The URLs use the project ID and the subscription ID.

[Cloud Pub/Sub]: https://cloud.google.com/pubsub/docs/

`pubsub.OpenSubscription` will use Application Default Credentials; if you have
authenticated via [`gcloud auth application-default login`][], it will use those credentials. See
[Application Default Credentials][GCP creds] to learn about authentication
alternatives, including using environment variables.

[GCP creds]: https://cloud.google.com/docs/authentication/production
[`gcloud auth application-default login`]: https://cloud.google.com/sdk/gcloud/reference/auth/application-default/login

{{< goexample "gocloud.dev/pubsub/gcppubsub.Example_openSubscriptionFromURL" >}}

#### Google Cloud Pub/Sub Constructor {#gcp-ctor}

The [`gcppubsub.OpenSubscription`][] constructor opens a Cloud Pub/Sub
subscription. You must first obtain [GCP credentials][GCP creds] and then
create a gRPC connection to Cloud Pub/Sub. (This gRPC connection can be
reused among subscriptions.)

{{< goexample "gocloud.dev/pubsub/gcppubsub.ExampleOpenSubscription" >}}

[`gcppubsub.OpenSubscription`]: https://godoc.org/gocloud.dev/pubsub/gcppubsub#OpenSubscription

### Amazon Simple Queueing Service {#sqs}

The Go CDK can subscribe to an Amazon [Simple Queueing Service][SQS] (SQS)
topic. SQS URLs closely resemble the the queue URL, except the leading
`https://` is replaced with `awssqs://`. You should specify the `region`
query parameter to ensure your application connects to the correct region.

[SQS]: https://aws.amazon.com/sqs/

`pubsub.OpenSubscription` will open a subscription using a default AWS Config.

{{< goexample "gocloud.dev/pubsub/awssnssqs.Example_openSubscriptionFromURL" >}}

If your messages are being sent to SQS directly, or if they are being delivered
via an SNS topic with `RawMessageDelivery` enabled, set a `raw=true` query
parameter in your URL, or set `SubscriberOptions.Raw` to `true` if you're using
the constructors. By default, the subscription will use heuristics to identify
whether the message bodies are raw or [SNS JSON][].

Messages with a `base64encoded` message attribute will be automatically
[Base64][] decoded before being returned. See the [SNS publishing guide][]
or the [SQS publishing guide][] for more details.

[Base64]: https://en.wikipedia.org/wiki/Base64
[SNS publishing guide]: {{< ref "./publish.md#sns" >}}
[SQS publishing guide]: {{< ref "./publish.md#sqs" >}}
[SNS JSON]: https://aws.amazon.com/sns/faqs/#Raw_message_delivery

#### Amazon SQS Constructor {#sqs-ctor}

The [`awssnssqs.OpenSubscription`][] constructor opens an SQS queue. You must
first create an AWS Config with the same region as your topic:

{{< goexample "gocloud.dev/pubsub/awssnssqs.ExampleOpenSubscription" >}}

[`awssnssqs.OpenSubscription`]: https://godoc.org/gocloud.dev/pubsub/awssnssqs#OpenSubscription

### Azure Service Bus {#azure}

The Go CDK can recieve messages from an [Azure Service Bus][] subscription.
The URL for subscribing is the topic name with the
subscription name in the `subscription` query parameter.
`pubsub.OpenSubscription` will use the environment variable
`SERVICEBUS_CONNECTION_STRING` to obtain the Service Bus Connection String
you need to copy [from the Azure portal][Azure connection string].

{{< goexample "gocloud.dev/pubsub/azuresb.Example_openSubscriptionFromURL" >}}

[AMQP 1.0]: https://www.amqp.org/
[Azure connection string]: https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-dotnet-how-to-use-topics-subscriptions#get-the-connection-string
[Azure Service Bus]: https://azure.microsoft.com/en-us/services/service-bus/

#### Azure Service Bus Constructor {#azure-ctor}

The [`azuresb.OpenSubscription`][] constructor opens an Azure Service Bus
subscription. You must first connect to the topic and subscription using the
[Azure Service Bus library][] and then pass the subscription to
`azuresb.OpenSubscription`. There are also helper functions in the `azuresb`
package to make this easier.

{{< goexample "gocloud.dev/pubsub/azuresb.ExampleOpenSubscription" >}}

[`azuresb.OpenSubscription`]: https://godoc.org/gocloud.dev/pubsub/azuresb#OpenSubscription
[Azure Service Bus library]: https://pkg.go.dev/github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus

### RabbitMQ {#rabbitmq}

The Go CDK can receive messages from an [AMQP 0.9.1][] queue, the dialect of
AMQP spoken by [RabbitMQ][]. A RabbitMQ URL only includes the queue name.
The RabbitMQ's server is discovered from the `RABBIT_SERVER_URL` environment
variable (which is something like `amqp://guest:guest@localhost:5672/`).

{{< goexample "gocloud.dev/pubsub/rabbitpubsub.Example_openSubscriptionFromURL" >}}

[AMQP 0.9.1]: https://www.rabbitmq.com/protocol.html
[RabbitMQ]: https://www.rabbitmq.com

#### RabbitMQ Constructor {#rabbitmq-ctor}

The [`rabbitpubsub.OpenSubscription`][] constructor opens a RabbitMQ queue.
You must first create an [`*amqp.Connection`][] to your RabbitMQ instance.

{{< goexample "gocloud.dev/pubsub/rabbitpubsub.ExampleOpenSubscription" >}}

[`*amqp.Connection`]: https://pkg.go.dev/github.com/rabbitmq/amqp091-go#Connection
[`rabbitpubsub.OpenSubscription`]: https://godoc.org/gocloud.dev/pubsub/rabbitpubsub#OpenSubscription

### NATS {#nats}

The Go CDK can publish to a [NATS][] subject. A NATS URL only includes the
subject name. The NATS server is discovered from the `NATS_SERVER_URL`
environment variable (which is something like `nats://nats.example.com`).

{{< goexample "gocloud.dev/pubsub/natspubsub.Example_openSubscriptionFromURL" >}}

NATS guarantees at-most-once delivery; it will never redeliver a message.
Therefore, `Message.Ack` is a no-op.

To parse messages [published via the Go CDK][publish#nats], the NATS driver
will first attempt to decode the payload using [gob][]. Failing that, it will
return the message payload as the `Data` with no metadata to accomodate
subscribing to messages coming from a source not using the Go CDK.

[gob]: https://golang.org/pkg/encoding/gob/
[NATS]: https://nats.io/
[publish#nats]: {{< ref "./publish.md#nats" >}}

#### NATS Constructor {#nats-ctor}

The [`natspubsub.OpenSubscription`][] constructor opens a NATS subject as a
topic. You must first create an [`*nats.Conn`][] to your NATS instance.

{{< goexample "gocloud.dev/pubsub/natspubsub.ExampleOpenSubscription" >}}

[`*nats.Conn`]: https://godoc.org/github.com/nats-io/go-nats#Conn
[`natspubsub.OpenSubscription`]: https://godoc.org/gocloud.dev/pubsub/natspubsub#OpenSubscription

### Kafka {#kafka}

The Go CDK can receive messages from a [Kafka][] cluster.
A Kafka URL includes the consumer group name, plus at least one instance
of a query parameter specifying the topic to subscribe to.
The brokers in the Kafka cluster are discovered from the
`KAFKA_BROKERS` environment variable (which is a comma-delimited list of
hosts, something like `1.2.3.4:9092,5.6.7.8:9092`).

{{< goexample "gocloud.dev/pubsub/kafkapubsub.Example_openSubscriptionFromURL" >}}

[Kafka]: https://kafka.apache.org/

#### Kafka Constructor {#kafka-ctor}

The [`kafkapubsub.OpenSubscription`][] constructor creates a consumer in a
consumer group, subscribed to one or more topics.

In addition to the list of brokers, you'll need a [`*sarama.Config`][], which
exposes many knobs that can affect performance and semantics; review and set
them carefully. [`kafkapubsub.MinimalConfig`][] provides a minimal config to
get you started.

{{< goexample "gocloud.dev/pubsub/kafkapubsub.ExampleOpenSubscription" >}}

[`*sarama.Config`]: https://godoc.org/github.com/IBM/sarama#Config
[`kafkapubsub.OpenSubscription`]: https://godoc.org/gocloud.dev/pubsub/kafkapubsub#OpenSubscription
[`kafkapubsub.MinimalConfig`]: https://godoc.org/gocloud.dev/pubsub/kafkapubsub#MinimalConfig

### In-Memory {#mem}

The Go CDK includes an in-memory Pub/Sub provider useful for local testing.
The names in `mem://` URLs are a process-wide namespace, so subscriptions to
the same name will receive messages posted to that topic. For instance, if
you open a topic `mem://topicA` and open two subscriptions with
`mem://topicA`, you will have two subscriptions to the same topic.

{{< goexample "gocloud.dev/pubsub/mempubsub.Example_openSubscriptionFromURL" >}}

#### In-Memory Constructor {#mem-ctor}

To create a subscription to an in-memory Pub/Sub topic, pass the [topic you
created][publish-mem-ctor] into the [`mempubsub.NewSubscription` function][].
You will also need to pass an acknowledgement deadline: once a message is
received, if it is not acknowledged after the deadline elapses, then it will be
redelivered.

{{< goexample "gocloud.dev/pubsub/mempubsub.ExampleNewSubscription" >}}

[`mempubsub.NewSubscription` function]: https://godoc.org/gocloud.dev/pubsub/mempubsub#NewSubscription
[publish-mem-ctor]: {{< ref "./publish.md#mem-ctor" >}}
