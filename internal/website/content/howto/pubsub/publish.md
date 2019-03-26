---
title: "Publish Messages to a Topic"
date: 2019-03-26T09:44:15-07:00
draft: true
weight: 1
---

Publishing a message to a topic with the Go CDK takes two steps:

1. Open the topic with the Pub/Sub provider of your choice (once per topic).
2. Send messages on the topic.

The second step is the same across all providers because the first step
creates a value of the portable [`*pubsub.Topic`][] type. Publishing looks
like this:

```go
err := topic.Send(ctx, &pubsub.Message{
    Body: []byte("Hello, World!\n")
    Metadata: map[string]string{
        "Foo": "Bar",
    },
})
if err != nil {
    return err
}
```

The rest of this guide will discuss how to accomplish the first step: opening
a topic for your chosen Pub/Sub provider.

[`*pubsub.Topic`]: https://godoc.org/gocloud.dev/pubsub#Topic

## Constructors versus URL openers

If you know that your program is always going to use a particular Pub/Sub
provider or you need fine-grained control over the connection settings, you
should call the constructor function in the driver package directly (like
`gcppubsub.OpenTopic`). However, if you want to change providers based on
configuration, you can use `pubsub.OpenTopic`, making sure you ["blank
import"][] the driver package to link it in. This guide will show how to use
both forms for each pub/sub provider.

["blank import"]: https://golang.org/doc/effective_go.html#blank_import

## Amazon Simple Notification Service {#sns}

The Go CDK can publish to an Amazon [Simple Notification Service][SNS] (SNS)
topic. SNS URLs in the Go CDK use the Amazon Resource Name (ARN) to identify
the topic. You can specify the `region` query parameter to ensure your
application connects to the correct region, but otherwise `pubsub.OpenTopic`
will use the region found in the environment variables or your AWS CLI
configuration.

```go
import (
    "gocloud.dev/pubsub"
    _ "gocloud.dev/pubsub/awssnssqs"
)

// ...

const topicARN = "arn:aws:sns:us-east-2:123456789012:MyTopic"
topic, err := pubsub.OpenTopic(ctx,
    "awssnssqs://" + topicARN + "?region=us-east-2")
if err != nil {
    return err
}
defer topic.Shutdown(ctx)
```

SNS messages are restricted to UTF-8 clean payloads. If your application
sends a message that contains non-UTF-8 bytes, then the Go CDK will
automatically [Base64][] encode the message and add a `base64encoded` message
attribute. When subscribing to messages on the topic through the Go CDK,
these will be [automatically Base64 decoded][SQS Subscribe], but if you are
receiving messages from a topic in a program that does not use the Go CDK,
you may need to manually Base64 decode the message payload.

[Base64]: https://en.wikipedia.org/wiki/Base64
[SQS Subscribe]: {{< relref "./subscribe.md#sqs" >}}
[SNS]: https://aws.amazon.com/sns/

### Amazon Simple Notification Service Constructor {#sns-ctor}

The [`awssnssqs.OpenTopic`][] constructor opens an SNS topic. You must first
create an [AWS session][] with the same region as your topic:

```go
import (
    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/aws/session"
    "github.com/aws/aws-sdk-go/service/sns"
    "gocloud.dev/pubsub/awssnssqs"
)
// ...

// Establish an AWS session.
// The region must match the region for "MyTopic".
sess, err := session.NewSession(&aws.Config{
    Region: aws.String("us-east-2"),
})
if err != nil {
    return err
}

// Create a *pubsub.Topic.
client := sns.New(sess)
const topicARN = "arn:aws:sns:us-east-2:123456789012:MyTopic"
topic, err := awssnssqs.OpenTopic(ctx, client, topicARN, nil)
if err != nil {
    return err
}
defer topic.Shutdown(ctx)
```

[`awssnssqs.OpenTopic`]: https://godoc.org/gocloud.dev/pubsub/awssnssqs#OpenTopic
[AWS session]: https://docs.aws.amazon.com/sdk-for-go/api/aws/session/

## Google Cloud Pub/Sub {#gcp}

The Go CDK can publish to a Google [Cloud Pub/Sub][] topic. The URLs use the
project ID and the topic ID. `pubsub.OpenTopic` will use [Application Default
Credentials][GCP creds].

```go
import (
    "gocloud.dev/pubsub"
    _ "gocloud.dev/pubsub/gcppubsub"
)

// ...

topic, err := pubsub.OpenTopic(ctx,
    "gcppubsub://my-project/my-topic")
if err != nil {
    return err
}
defer topic.Shutdown(ctx)
```

[Cloud Pub/Sub]: https://cloud.google.com/pubsub/docs/
[GCP creds]: https://cloud.google.com/docs/authentication/production

### Google Cloud Pub/Sub Constructor {#gcp-ctor}

The [`gcppubsub.OpenTopic`][] constructor opens a Cloud Pub/Sub topic. You
must first obtain [GCP credentials][GCP creds] and then create a gRPC
connection to Cloud Pub/Sub. (This gRPC connection can be reused among
topics.)

```go
import (
    "gocloud.dev/gcp"
    "gocloud.dev/pubsub/gcppubsub"
)

// ...

// Your GCP credentials.
// See https://cloud.google.com/docs/authentication/production
// for more info on alternatives.
creds, err := gcp.DefaultCredentials(ctx)
if err != nil {
    return err
}
// Get the project ID from the credentials (required by OpenTopic).
projectID, err := gcp.DefaultProjectID(creds)
if err != nil {
    return err
}

// Open a gRPC connection to the GCP Pub/Sub API.
conn, cleanup, err := gcppubsub.Dial(ctx, creds.TokenSource)
if err != nil {
    return err
}
defer cleanup()

// Construct a PublisherClient using the connection.
pubClient, err := gcppubsub.PublisherClient(ctx, conn)
if err != nil {
    return err
}
defer pubClient.Close()

// Construct a *pubsub.Topic.
topic := gcppubsub.OpenTopic(pubClient, projectID, "example-topic", nil)
defer topic.Shutdown(ctx)
```

[`gcppubsub.OpenTopic`]: https://godoc.org/gocloud.dev/pubsub/gcppubsub#OpenTopic

## Azure Service Bus {#azure}

The Go CDK can publish to an [Azure Service Bus][] topic over [AMQP 1.0][].
The URL for publishing is the topic name. `pubsub.OpenTopic` will use the
environment variable `SERVICEBUS_CONNECTION_STRING` to obtain the Service Bus
Connection String you need to copy [from the Azure portal][Azure connection
string].

```go
import (
    "gocloud.dev/pubsub"
    _ "gocloud.dev/pubsub/gcppubsub"
)

// ...

topic, err := pubsub.OpenTopic(ctx, "azuresb://mytopic")
if err != nil {
    return err
}
defer topic.Shutdown(ctx)
```

[AMQP 1.0]: https://www.amqp.org/
[Azure connection string]: https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-dotnet-how-to-use-topics-subscriptions#get-the-connection-string
[Azure Service Bus]: https://azure.microsoft.com/en-us/services/service-bus/

### Azure Service Bus Constructor {#azure-ctor}

The [`azuresb.OpenTopic`][] constructor opens an Azure Service Bus topic. You
must first connect to the topic using the [Azure Service Bus library][] and
then pass it to `azuresb.OpenTopic`. There are also helper functions in the
`azuresb` package to make this easier.

```go
import (
    "os"

    "gocloud.dev/pubsub/azuresb"
)

// ...

// Change these as needed for your application.
serviceBusConnString := os.Getenv("SERVICEBUS_CONNECTION_STRING")
const topicName = "test-topic"

// Connect to Azure Service Bus for the given topic.
busNamespace, err := azuresb.NewNamespaceFromConnectionString(serviceBusConnString)
if err != nil {
    return err
}
busTopic, err := azuresb.NewTopic(busNamespace, topicName, nil)
if err != nil {
    return err
}
defer busTopic.Close(ctx)

// Construct a *pubsub.Topic.
topic, err := azuresb.OpenTopic(ctx, busTopic, nil)
if err != nil {
    return err
}
defer topic.Shutdown(ctx)
```

[`azuresb.OpenTopic`]: https://godoc.org/gocloud.dev/pubsub/azuresb#OpenTopic
[Azure Service Bus library]: https://github.com/Azure/azure-service-bus-go

## RabbitMQ {#rabbitmq}

The Go CDK can publish to an [AMQP 0.9.1][] fanout exchange, the dialect of
AMQP spoken by [RabbitMQ][]. A RabbitMQ URL only includes the exchange name.
The RabbitMQ's server is discovered from the `RABBIT_SERVER_URL` environment
variable (which is something like `amqp://guest:guest@localhost:5672/`).

```go
import (
    "gocloud.dev/pubsub"
    _ "gocloud.dev/pubsub/rabbitpubsub"
)

// ...

topic, err := pubsub.OpenTopic(ctx, "rabbit://myexchange")
if err != nil {
    return err
}
defer topic.Shutdown(ctx)
```

[AMQP 0.9.1]: https://www.rabbitmq.com/protocol.html
[RabbitMQ]: https://www.rabbitmq.com

### RabbitMQ Constructor {#rabbitmq-ctor}

The [`rabbitpubsub.OpenTopic`][] constructor opens a RabbitMQ exchange. You
must first create an [`*amqp.Connection`][] to your RabbitMQ instance.

```go
import (
    "github.com/streadway/amqp"
    "gocloud.dev/pubsub/rabbitpubsub"
)

// ...

rabbitConn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
if err != nil {
    return err
}
topic := rabbitpubsub.OpenTopic(rabbitConn, "myexchange", nil)
defer topic.Shutdown(ctx)
```

[`*amqp.Connection`]: https://godoc.org/github.com/streadway/amqp#Connection
[`rabbitpubsub.OpenTopic`]: https://godoc.org/gocloud.dev/pubsub/rabbitpubsub#OpenTopic

## NATS {#nats}

The Go CDK can publish to a [NATS][] subject. A NATS URL only includes the
subject name. The NATS server is discovered from the `NATS_SERVER_URL`
environment variable (which is something like `nats://nats.example.com`).

```go
import (
    "gocloud.dev/pubsub"
    _ "gocloud.dev/pubsub/natspubsub"
)

// ...

topic, err := pubsub.OpenTopic(ctx, "nats://example.mysubject")
if err != nil {
    return err
}
defer topic.Shutdown(ctx)
```

**TODO(light): Something about msgpack and ugorji driver? I don't understand
*from reference.**

[NATS]: https://nats.io/

### NATS Constructor {#nats-ctor}

The [`natspubsub.CreateTopic`][] constructor opens a NATS subject as a topic. You
must first create an [`*nats.Conn`][] to your NATS instance.

```go
import (
    "github.com/nats-io/go-nats"
    "gocloud.dev/pubsub/natspubsub"
)

// ...

natsConn, err := nats.Connect("nats://nats.example.com")
if err != nil {
    return err
}
defer natsConn.Close()

topic, err := natspubsub.CreateTopic(natsConn, "example.mysubject", nil)
if err != nil {
    return err
}
defer topic.Shutdown(ctx)
```

[`*nats.Conn`]: https://godoc.org/github.com/nats-io/go-nats#Conn
[`natspubsub.CreateTopic`]: https://godoc.org/gocloud.dev/pubsub/natspubsub#CreateTopic

## In-Memory {#mem}

The Go CDK includes an in-memory Pub/Sub provider useful for local testing.
The names in `mem://` URLs are a process-wide namespace, so subscriptions to
the same name will receive messages posted to that topic. This is detailed
more in the [subscription guide][subscribe-mem].

```go
import (
    "gocloud.dev/pubsub"
    _ "gocloud.dev/pubsub/mempubsub"
)

// ...

topic, err := pubsub.OpenTopic(ctx, "mem://topicA")
if err != nil {
    return err
}
defer topic.Shutdown(ctx)
```

[subscribe-mem]: {{< ref "./subscribe.md#mem" >}}

### In-Memory Constructor {#mem-ctor}

To create an in-memory Pub/Sub topic, use the [`mempubsub.NewTopic`
function][]. You can use the returned topic to create in-memory
subscriptions, as detailed in the [subscription guide][subscribe-mem-ctor].

```go
import (
    "gocloud.dev/pubsub/mempubsub"
)

// ...

topic := mempubsub.NewTopic()
defer topic.Shutdown(ctx)
```

[`mempubsub.NewTopic` function]: https://godoc.org/gocloud.dev/pubsub/mempubsub#NewTopic
[subscribe-mem-ctor]: {{< ref "./subscribe.md#mem-ctor" >}}

