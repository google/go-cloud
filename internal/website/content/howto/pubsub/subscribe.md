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

```go
func main() {
    // Parse configuration and open subscription (shutdown deferred).
    // ...

    // ... and then loop on received messages.
    for {
        msg, err := subscription.Receive(ctx)
        if err != nil {
            // Errors from Receive indicate that Receive will no longer succeed.
            log.Printf("Receiving message: %v", err)
            break
        }
        handle(ctx, msg.Data)
        msg.Ack()
    }
}

func handle(ctx context.Context, data []byte) {
    // Do work based on the message.
    fmt.Printf("Got message: %q\n", data)
}
```

If you want your subscriber to operate on the incoming messages concurrently,
you can start multiple goroutines:

```go
func main() {
    // Create context. If you want to implement graceful shutdown, cancel the
    // context (perhaps after an operating system signal).
    ctx := context.Background()

    // Parse configuration and open subscription (shutdown deferred).
    // ...

    // ... and then loop on received messages. We can use a channel as a
    // semaphore to limit how many goroutines we have active at a time as well
    // as wait on the goroutines to finish before exiting.
    const maxHandlers = 10
    sem := make(chan struct{}, maxHandlers)
recvLoop:
    for {
        msg, err := subscription.Receive(ctx)
        if err != nil {
            // Errors from Receive indicate that Receive will no longer succeed.
            log.Printf("Receiving message: %v", err)
            break
        }

        // Wait if there are too many active handle goroutines and acquire the
        // semaphore. If the context is canceled, stop waiting and start
        // shutting down.
        select {
        case sem <- struct{}{}:
        case <-ctx.Done():
            break recvLoop
        }

        // Handle the message in a new goroutine.
        go func() {
            defer func() { <-sem }() // Release the semaphore.
            handle(ctx, msg.Data)
            msg.Ack()
        }()
    }

    // We're no longer receiving messages. Wait to finish handling any
    // unacknowledged messages by totally acquiring the semaphore.
    for n := 0; n < maxHandlers; n-- {
        sem <- struct{}{}
    }
}

func handle(ctx context.Context, data []byte) {
    // Do work based on the message.
    fmt.Printf("Got message: %q\n", data)
}
```

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
[documentation on URLs]: https://godoc.org/gocloud.dev#hdr-URLs

## Amazon Simple Queueing Service {#sqs}

The Go CDK can subscribe to an Amazon [Simple Queueing Service][SQS] (SQS)
topic. SQS URLs closely resemble the the queue URL, except the leading
`https://` is replaced with `awssnssqs://`. You can specify the `region`
query parameter to ensure your application connects to the correct region, but
otherwise `pubsub.OpenSubscription` will use the region found in the environment
variables or your AWS CLI configuration.

```go
import (
    "gocloud.dev/pubsub"
    _ "gocloud.dev/pubsub/awssnssqs"
)

// ...

subscription, err := pubsub.OpenSubscription(ctx,
    "awssnssqs://sqs.us-east-2.amazonaws.com/123456789012/" +
    "MyQueue?region=us-east-2")
if err != nil {
    return err
}
defer subscription.Shutdown(ctx)
```

Messages with a `base64encoded` message attribute will be automatically
[Base64][] decoded before being returned. See the [SNS publishing guide][]
for more details.

[Base64]: https://en.wikipedia.org/wiki/Base64
[SNS publishing guide]: {{< ref "./publish.md#sns" >}}
[SQS]: https://aws.amazon.com/sqs/

### Amazon Simple Queueing Service Constructor {#sqs-ctor}

The [`awssnssqs.OpenSubscription`][] constructor opens an SQS queue. You must
first create an [AWS session][] with the same region as your topic:

```go
import (
    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/aws/session"
    "github.com/aws/aws-sdk-go/service/sqs"
    "gocloud.dev/pubsub/awssnssqs"
)
// ...

// Establish an AWS session.
// The region must match the region for "MyQueue".
sess, err := session.NewSession(&aws.Config{
    Region: aws.String("us-east-2"),
})
if err != nil {
    return err
}

// Create a *pubsub.Subscription.
client := sqs.New(sess)
const queueURL = "awssnssqs://sqs.us-east-2.amazonaws.com/123456789012/MyQueue"
subscription, err := awssnssqs.OpenSubscription(ctx, client, queueURL, nil)
if err != nil {
    return err
}
defer subscription.Shutdown(ctx)
```

[`awssnssqs.OpenSubscription`]: https://godoc.org/gocloud.dev/pubsub/awssnssqs#OpenSubscription
[AWS session]: https://docs.aws.amazon.com/sdk-for-go/api/aws/session/

## Google Cloud Pub/Sub {#gcp}

The Go CDK can receive messages from a Google [Cloud Pub/Sub][] subscription.
The URLs use the project ID and the subscription ID.
`pubsub.OpenSubscription` will use [Application Default Credentials][GCP creds].

```go
import (
    "gocloud.dev/pubsub"
    _ "gocloud.dev/pubsub/gcppubsub"
)

// ...

subscription, err := pubsub.OpenSubscription(ctx,
    "gcppubsub://my-project/my-subscription")
if err != nil {
    return err
}
defer subscription.Shutdown(ctx)
```

[Cloud Pub/Sub]: https://cloud.google.com/pubsub/docs/
[GCP creds]: https://cloud.google.com/docs/authentication/production

### Google Cloud Pub/Sub Constructor {#gcp-ctor}

The [`gcppubsub.OpenSubscription`][] constructor opens a Cloud Pub/Sub
subscription. You must first obtain [GCP credentials][GCP creds] and then
create a gRPC connection to Cloud Pub/Sub. (This gRPC connection can be
reused among subscriptions.)

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
// Get the project ID from the credentials (required by OpenSubscription).
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

// Construct a SubscriberClient using the connection.
subClient, err := gcppubsub.SubscriberClient(ctx, conn)
if err != nil {
    return err
}
defer subClient.Close()

// Construct a *pubsub.Subscription.
subscription := gcppubsub.OpenSubscription(
    subClient, projectID, "example-subscription", nil)
defer subscription.Shutdown(ctx)
```

[`gcppubsub.OpenSubscription`]: https://godoc.org/gocloud.dev/pubsub/gcppubsub#OpenSubscription

## Azure Service Bus {#azure}

The Go CDK can recieve messages from an [Azure Service Bus][] subscription
over [AMQP 1.0][]. The URL for subscribing is the topic name with the
subscription name in the `subscription` query parameter.
`pubsub.OpenSubscription` will use the environment variable
`SERVICEBUS_CONNECTION_STRING` to obtain the Service Bus Connection String
you need to copy [from the Azure portal][Azure connection string].

```go
import (
    "gocloud.dev/pubsub"
    _ "gocloud.dev/pubsub/azuresb"
)

// ...

subscription, err := pubsub.OpenSubscription(ctx,
    "azuresb://mytopic?subscription=mysubscription")
if err != nil {
    return err
}
defer subscription.Shutdown(ctx)
```

[AMQP 1.0]: https://www.amqp.org/
[Azure connection string]: https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-dotnet-how-to-use-topics-subscriptions#get-the-connection-string
[Azure Service Bus]: https://azure.microsoft.com/en-us/services/service-bus/

### Azure Service Bus Constructor {#azure-ctor}

The [`azuresb.OpenSubscription`][] constructor opens an Azure Service Bus
subscription. You must first connect to the topic and subscription using the
[Azure Service Bus library][] and then pass the subscription to
`azuresb.OpenSubscription`. There are also helper functions in the `azuresb`
package to make this easier.

```go
import (
    "os"

    "gocloud.dev/pubsub/azuresb"
)

// ...

// Change these as needed for your application.
serviceBusConnString := os.Getenv("SERVICEBUS_CONNECTION_STRING")
const topicName = "test-topic"
const subscriptionName = "test-subscription"

// Connect to Azure Service Bus for the given subscription.
busNamespace, err := azuresb.NewNamespaceFromConnectionString(serviceBusConnString)
if err != nil {
    return err
}
busTopic, err := azuresb.NewTopic(busNamespace, topicName, nil)
if err != nil {
    return err
}
defer busTopic.Close(ctx)
busSub, err := azuresb.NewSubscription(busTopic, subscriptionName, nil)
if err != nil {
    return err
}
defer busSub.Close(ctx)

// Construct a *pubsub.Subscription.
subscription, err := azuresb.OpenSubscription(ctx, busSub, nil)
if err != nil {
    return err
}
defer subscription.Shutdown(ctx)
```

[`azuresb.OpenSubscription`]: https://godoc.org/gocloud.dev/pubsub/azuresb#OpenSubscription
[Azure Service Bus library]: https://github.com/Azure/azure-service-bus-go

## RabbitMQ {#rabbitmq}

The Go CDK can receive messages from an [AMQP 0.9.1][] queue, the dialect of
AMQP spoken by [RabbitMQ][]. A RabbitMQ URL only includes the queue name.
The RabbitMQ's server is discovered from the `RABBIT_SERVER_URL` environment
variable (which is something like `amqp://guest:guest@localhost:5672/`).

```go
import (
    "gocloud.dev/pubsub"
    _ "gocloud.dev/pubsub/rabbitpubsub"
)

// ...

subscription, err := pubsub.OpenSubscription(ctx, "rabbit://myqueue")
if err != nil {
    return err
}
defer subscription.Shutdown(ctx)
```

[AMQP 0.9.1]: https://www.rabbitmq.com/protocol.html
[RabbitMQ]: https://www.rabbitmq.com

### RabbitMQ Constructor {#rabbitmq-ctor}

The [`rabbitpubsub.OpenSubscription`][] constructor opens a RabbitMQ queue.
You must first create an [`*amqp.Connection`][] to your RabbitMQ instance.

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
subscription := rabbitpubsub.OpenSubscription(rabbitConn, "myqueue", nil)
defer subscription.Shutdown(ctx)
```

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

```go
import (
    "gocloud.dev/pubsub"
    _ "gocloud.dev/pubsub/natspubsub"
)

// ...

subscription, err := pubsub.OpenSubscription(ctx,
    "nats://example.mysubject?ackfunc=panic")
if err != nil {
    return err
}
defer subscription.Shutdown(ctx)
```

To parse messages [published via the Go CDK][publish#nats], the NATS driver
will first attempt to decode the payload using [gob][]. Failing that, it will
return the message payload as the `Data` with no metadata to accomodate
subscribing to messages coming from a source not using the Go CDK.

[gob]: https://golang.org/pkg/encoding/gob/
[NATS]: https://nats.io/
[publish#nats]: {{< ref "./publish.md#nats" >}}

### NATS Constructor {#nats-ctor}

The [`natspubsub.CreateSubscription`][] constructor opens a NATS subject as a
topic. You must first create an [`*nats.Conn`][] to your NATS instance.

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

subscription, err := natspubsub.CreateSubscription(
    natsConn,
    "example.mysubject",
    func() { panic("nats does not have ack") },
    nil)
if err != nil {
    return err
}
defer subscription.Shutdown(ctx)
```

[`*nats.Conn`]: https://godoc.org/github.com/nats-io/go-nats#Conn
[`natspubsub.CreateSubscription`]: https://godoc.org/gocloud.dev/pubsub/natspubsub#CreateSubscription

## Kafka {#kafka}

The Go CDK can receive messages from a [Kafka][] cluster.
A Kafka URL includes the consumer group name, plus at least one instance
of a query parameter specifying the topic to subscribe to.
The brokers in the Kafka cluster are discovered from the
`KAFKA_BROKERS` environment variable (which is a comma-delimited list of
hosts, something like `1.2.3.4:9092,5.6.7.8:9092`).

```go
import (
    "gocloud.dev/pubsub"
    _ "gocloud.dev/pubsub/kafkapubsub"
)

// ...

subscription, err := pubsub.OpenSubscription(ctx, "kafka://my-group?topic=my-topic")
if err != nil {
    return err
}
defer subscription.Shutdown(ctx)
```

[Kafka]: https://kafka.apache.org/

### Kafka Constructor {#kafka-ctor}

The [`kafkapubsub.OpenSubscription`][] constructor creates a consumer in a
consumer group, subscribed to one or more topics.

In addition to the list of brokers, you'll need a [`*sarama.Config`][], which
exposes many knobs that can affect performance and semantics; review and set
them carefully. [`kafkapubsub.MinimalConfig`][] provides a minimal config to get
you started.

```go
import (
    "gocloud.dev/pubsub/kafkapubsub"
)

// ...

// The set of brokers in the Kafka cluster.
addrs := []string{"1.2.3.4:9092"}
// The Kafka client configuration to use.
config := kafkapubsub.MinimalConfig()

subscription, err := kafkapubsub.OpenSubscription(addrs, config, "my-group", []string{"my-topic"}, nil)
if err != nil {
  return err
}
defer subscription.Shutdown(ctx)
```

[`*sarama.Config`]: https://godoc.org/github.com/Shopify/sarama#Config
[`kafkapubsub.OpenSubscription`]: https://godoc.org/gocloud.dev/pubsub/kafkapubsub#OpenSubscription
[`kafkapubsub.MinimalConfig`]: https://godoc.org/gocloud.dev/pubsub/kafkapubsub#MinimalConfig

## In-Memory {#mem}

The Go CDK includes an in-memory Pub/Sub provider useful for local testing.
The names in `mem://` URLs are a process-wide namespace, so subscriptions to
the same name will receive messages posted to that topic. For instance, if
you open a topic `mem://topicA` and open two subscriptions with
`mem://topicA`, you will have two subscriptions to the same topic.

```go
import (
    "gocloud.dev/pubsub"
    _ "gocloud.dev/pubsub/mempubsub"
)

// ...

// Create a topic.
topic, err := pubsub.OpenTopic(ctx, "mem://topicA")
if err != nil {
    return err
}
defer topic.Shutdown(ctx)

// Create a subscription connected to that topic.
subscription, err := pubsub.OpenSubscription(ctx, "mem://topicA")
if err != nil {
    return err
}
defer subscription.Shutdown(ctx)
```

### In-Memory Constructor {#mem-ctor}

To create a subscription to an in-memory Pub/Sub topic, pass the [topic you
created][publish-mem-ctor] into the [`mempubsub.NewSubscription` function][].
You will also need to pass an acknowledgement deadline: once a message is
received, if it is not acknowledged after the deadline elapses, then it will be
redelivered.

```go
import (
    "time"

    "gocloud.dev/pubsub/mempubsub"
)

// ...

topic := mempubsub.NewTopic()
defer topic.Shutdown(ctx)
subscription := mempubsub.NewSubscription(topic, 1 * time.Minute)
```

[`mempubsub.NewSubscription` function]: https://godoc.org/gocloud.dev/pubsub/mempubsub#NewSubscription
[publish-mem-ctor]: {{< ref "./publish.md#mem-ctor" >}}

