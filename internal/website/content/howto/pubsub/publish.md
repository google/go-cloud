---
title: "Publish Messages to a Topic"
date: 2019-03-26T09:44:15-07:00
lastmod: 2019-07-29T12:00:00-07:00
weight: 1
toc: true
---

Publishing a message to a topic with the Go CDK takes two steps:

1. [Open a topic][] with the Pub/Sub provider of your choice (once per topic).
2. [Send messages][] on the topic.

[Open a topic]: {{< ref "#opening" >}}
[Send messages]: {{< ref "#sending" >}}

<!--more-->

## Opening a Topic {#opening}

The first step in publishing messages to a topic is to instantiate a
portable [`*pubsub.Topic`][] for your service.

The easiest way to do so is to use [`pubsub.OpenTopic`][] and a service-specific URL
pointing to the topic, making sure you ["blank import"][] the driver package to
link it in.

```go
import (
    "context"

    "gocloud.dev/pubsub"
    _ "gocloud.dev/pubsub/<driver>"
)
...
ctx := context.Background()
topic, err := pubsub.OpenTopic(ctx, "<driver-url>")
if err != nil {
    return fmt.Errorf("could not open topic: %v", err)
}
defer topic.Shutdown(ctx)
// topic is a *pubsub.Topic; see usage below
...
```

See [Concepts: URLs][] for general background and the [guide below][]
for URL usage for each supported service.

Alternatively, if you need fine-grained
control over the connection settings, you can call the constructor function in
the driver package directly (like `gcppubsub.OpenTopic`).

```go
import "gocloud.dev/pubsub/<driver>"
...
topic, err := <driver>.OpenTopic(...)
...
```

You may find the [`wire` package][] useful for managing your initialization code
when switching between different backing services.

See the [guide below][] for constructor usage for each supported service.

[guide below]: {{< ref "#services" >}}
[`*pubsub.Topic`]: https://godoc.org/gocloud.dev/pubsub#Topic
[`pubsub.OpenTopic`]:
https://godoc.org/gocloud.dev/pubsub#OpenTopic
["blank import"]: https://golang.org/doc/effective_go.html#blank_import
[Concepts: URLs]: {{< ref "/concepts/urls.md" >}}
[`wire` package]: http://github.com/google/wire

## Sending Messages on a Topic {#sending}

Sending a message on a [Topic](https://godoc.org/gocloud.dev/pubsub#Topic) looks
like this:

{{< goexample src="gocloud.dev/pubsub.ExampleTopic_Send" imports="0" >}}

Note that the [semantics of message delivery][] can vary by backing service.

[semantics of message delivery]: https://godoc.org/gocloud.dev/pubsub#hdr-At_most_once_and_At_least_once_Delivery

## Other Usage Samples

* [CLI Sample](https://github.com/google/go-cloud/tree/master/samples/gocdk-pubsub)
* [Order Processor sample](https://gocloud.dev/tutorials/order/)
* [pubsub package examples](https://godoc.org/gocloud.dev/pubsub#pkg-examples)

## Supported Pub/Sub Services {#services}

### Google Cloud Pub/Sub {#gcp}

The Go CDK can publish to a Google [Cloud Pub/Sub][] topic. The URLs use the
project ID and the topic ID.

[Cloud Pub/Sub]: https://cloud.google.com/pubsub/docs/

`pubsub.OpenTopic` will use Application Default Credentials; if you have
authenticated via [`gcloud auth application-default login`][], it will use those credentials. See
[Application Default Credentials][GCP creds] to learn about authentication
alternatives, including using environment variables.

[GCP creds]: https://cloud.google.com/docs/authentication/production
[`gcloud auth application-default login`]: https://cloud.google.com/sdk/gcloud/reference/auth/application-default/login

{{< goexample "gocloud.dev/pubsub/gcppubsub.Example_openTopicFromURL" >}}

#### Google Cloud Pub/Sub Constructor {#gcp-ctor}

The [`gcppubsub.OpenTopic`][] constructor opens a Cloud Pub/Sub topic. You
must first obtain [GCP credentials][GCP creds] and then create a gRPC
connection to Cloud Pub/Sub. (This gRPC connection can be reused among
topics.)

{{< goexample "gocloud.dev/pubsub/gcppubsub.ExampleOpenTopic" >}}

[`gcppubsub.OpenTopic`]: https://godoc.org/gocloud.dev/pubsub/gcppubsub#OpenTopic

### Amazon Simple Notification Service {#sns}

The Go CDK can publish to an Amazon [Simple Notification Service][SNS] (SNS)
topic. SNS URLs in the Go CDK use the Amazon Resource Name (ARN) to identify
the topic. You should specify the `region` query parameter to ensure your
application connects to the correct region.

[SNS]: https://aws.amazon.com/sns/

It will create an AWS Config based on the AWS SDK V2; see [AWS V2 Config][] to learn more.

{{< goexample "gocloud.dev/pubsub/awssnssqs.Example_openSNSTopicFromURL" >}}

SNS messages are restricted to UTF-8 clean payloads. If your application
sends a message that contains non-UTF-8 bytes, then the Go CDK will
automatically [Base64][] encode the message and add a `base64encoded` message
attribute. When subscribing to messages on the topic through the Go CDK,
these will be [automatically Base64 decoded][SQS Subscribe], but if you are
receiving messages from a topic in a program that does not use the Go CDK,
you may need to manually Base64 decode the message payload.

[Base64]: https://en.wikipedia.org/wiki/Base64
[SQS Subscribe]: {{< relref "./subscribe.md#sqs" >}}
[AWS V2 Config]: https://aws.github.io/aws-sdk-go-v2/docs/configuring-sdk/

#### Amazon SNS Constructor {#sns-ctor}

The [`awssnssqs.OpenSNSTopic`][] constructor opens an SNS topic. You must first
create an AWS Config with the same region as your topic:

{{< goexample "gocloud.dev/pubsub/awssnssqs.ExampleOpenSNSTopic" >}}

[`awssnssqs.OpenSNSTopic`]: https://godoc.org/gocloud.dev/pubsub/awssnssqs#OpenSNSTopic

### Amazon Simple Queue Service {#sqs}

The Go CDK can publish to an Amazon [Simple Queue Service][SQS] (SQS)
topic. SQS URLs closely resemble the the queue URL, except the leading
`https://` is replaced with `awssqs://`. You can specify the `region`
query parameter to ensure your application connects to the correct region, but
otherwise `pubsub.OpenTopic` will use the region found in the environment
variables or your AWS CLI configuration.

{{< goexample "gocloud.dev/pubsub/awssnssqs.Example_openSQSTopicFromURL" >}}

SQS messages are restricted to UTF-8 clean payloads. If your application
sends a message that contains non-UTF-8 bytes, then the Go CDK will
automatically [Base64][] encode the message and add a `base64encoded` message
attribute. When subscribing to messages on the topic through the Go CDK,
these will be [automatically Base64 decoded][SQS Subscribe], but if you are
receiving messages from a topic in a program that does not use the Go CDK,
you may need to manually Base64 decode the message payload.

[Base64]: https://en.wikipedia.org/wiki/Base64
[SQS Subscribe]: {{< relref "./subscribe.md#sqs" >}}
[SQS]: https://aws.amazon.com/sqs/

#### Amazon SQS Constructor {#sqs-ctor}

The [`awssnssqs.OpenSQSTopic`][] constructor opens an SQS topic. You must first
create an AWS Config with the same region as your topic:

{{< goexample "gocloud.dev/pubsub/awssnssqs.ExampleOpenSQSTopic" >}}

[`awssnssqs.OpenSQSTopic`]: https://godoc.org/gocloud.dev/pubsub/awssnssqs#OpenSQSTopic

### Azure Service Bus {#azure}

The Go CDK can publish to an [Azure Service Bus][] topic.
The URL for publishing is the topic name. `pubsub.OpenTopic` will use the
environment variable `SERVICEBUS_CONNECTION_STRING` to obtain the Service Bus
connection string. The connection string can be obtained
[from the Azure portal][Azure connection string].

{{< goexample "gocloud.dev/pubsub/azuresb.Example_openTopicFromURL" >}}

[Azure connection string]: https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-dotnet-how-to-use-topics-subscriptions#get-the-connection-string
[Azure Service Bus]: https://azure.microsoft.com/en-us/services/service-bus/

#### Azure Service Bus Constructor {#azure-ctor}

The [`azuresb.OpenTopic`][] constructor opens an Azure Service Bus topic. You
must first connect to the topic using the [Azure Service Bus library][] and
then pass it to `azuresb.OpenTopic`. There are also helper functions in the
`azuresb` package to make this easier.

{{< goexample "gocloud.dev/pubsub/azuresb.ExampleOpenTopic" >}}

[`azuresb.OpenTopic`]: https://godoc.org/gocloud.dev/pubsub/azuresb#OpenTopic
[Azure Service Bus library]: https://pkg.go.dev/github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus

### RabbitMQ {#rabbitmq}

The Go CDK can publish to an [AMQP 0.9.1][] fanout exchange, the dialect of
AMQP spoken by [RabbitMQ][]. A RabbitMQ URL only includes the exchange name.
The RabbitMQ's server is discovered from the `RABBIT_SERVER_URL` environment
variable (which is something like `amqp://guest:guest@localhost:5672/`).

{{< goexample "gocloud.dev/pubsub/rabbitpubsub.Example_openTopicFromURL" >}}

[AMQP 0.9.1]: https://www.rabbitmq.com/protocol.html
[RabbitMQ]: https://www.rabbitmq.com

#### RabbitMQ Constructor {#rabbitmq-ctor}

The [`rabbitpubsub.OpenTopic`][] constructor opens a RabbitMQ exchange. You
must first create an [`*amqp.Connection`][] to your RabbitMQ instance.

{{< goexample "gocloud.dev/pubsub/rabbitpubsub.ExampleOpenTopic" >}}

[`*amqp.Connection`]: https://pkg.go.dev/github.com/rabbitmq/amqp091-go#Connection
[`rabbitpubsub.OpenTopic`]: https://godoc.org/gocloud.dev/pubsub/rabbitpubsub#OpenTopic

### NATS {#nats}

The Go CDK can publish to a [NATS][] subject. A NATS URL only includes the
subject name. The NATS server is discovered from the `NATS_SERVER_URL`
environment variable (which is something like `nats://nats.example.com`).

{{< goexample "gocloud.dev/pubsub/natspubsub.Example_openTopicFromURL" >}}

Because NATS does not natively support metadata, messages sent to NATS will
be encoded with [gob][].

[gob]: https://golang.org/pkg/encoding/gob/
[NATS]: https://nats.io/

#### NATS Constructor {#nats-ctor}

The [`natspubsub.OpenTopic`][] constructor opens a NATS subject as a topic. You
must first create an [`*nats.Conn`][] to your NATS instance.

{{< goexample "gocloud.dev/pubsub/natspubsub.ExampleOpenTopic" >}}

[`*nats.Conn`]: https://godoc.org/github.com/nats-io/go-nats#Conn
[`natspubsub.OpenTopic`]: https://godoc.org/gocloud.dev/pubsub/natspubsub#OpenTopic

### Kafka {#kafka}

The Go CDK can publish to a [Kafka][] cluster. A Kafka URL only includes the
topic name. The brokers in the Kafka cluster are discovered from the
`KAFKA_BROKERS` environment variable (which is a comma-delimited list of
hosts, something like `1.2.3.4:9092,5.6.7.8:9092`).

{{< goexample "gocloud.dev/pubsub/kafkapubsub.Example_openTopicFromURL" >}}

[Kafka]: https://kafka.apache.org/

#### Kafka Constructor {#kafka-ctor}

The [`kafkapubsub.OpenTopic`][] constructor opens a Kafka topic to publish
messages to. Depending on your Kafka cluster configuration (see
`auto.create.topics.enable`), you may need to provision the topic beforehand.

In addition to the list of brokers, you'll need a [`*sarama.Config`][], which
exposes many knobs that can affect performance and semantics; review and set
them carefully. [`kafkapubsub.MinimalConfig`][] provides a minimal config to get
you started.

{{< goexample "gocloud.dev/pubsub/kafkapubsub.ExampleOpenTopic" >}}

[`*sarama.Config`]: https://godoc.org/github.com/IBM/sarama#Config
[`kafkapubsub.OpenTopic`]: https://godoc.org/gocloud.dev/pubsub/kafkapubsub#OpenTopic
[`kafkapubsub.MinimalConfig`]: https://godoc.org/gocloud.dev/pubsub/kafkapubsub#MinimalConfig

### In-Memory {#mem}

The Go CDK includes an in-memory Pub/Sub provider useful for local testing.
The names in `mem://` URLs are a process-wide namespace, so subscriptions to
the same name will receive messages posted to that topic. This is detailed
more in the [subscription guide][subscribe-mem].

{{< goexample "gocloud.dev/pubsub/mempubsub.Example_openTopicFromURL" >}}

[subscribe-mem]: {{< ref "./subscribe.md#mem" >}}

#### In-Memory Constructor {#mem-ctor}

To create an in-memory Pub/Sub topic, use the [`mempubsub.NewTopic`
function][]. You can use the returned topic to create in-memory
subscriptions, as detailed in the [subscription guide][subscribe-mem-ctor].

{{< goexample "gocloud.dev/pubsub/mempubsub.ExampleNewTopic" >}}

[`mempubsub.NewTopic` function]: https://godoc.org/gocloud.dev/pubsub/mempubsub#NewTopic
[subscribe-mem-ctor]: {{< ref "./subscribe.md#mem-ctor" >}}
