# Go Cloud PubSub

## Summary
This document proposes a new pubsub package for Go Cloud.

## Motivation
A developer designing a new system with cross-cloud portability in mind could
choose a messaging system supporting pubsub, such as ZeroMQ, Kafka or
RabbitMQ. These pubsub systems run on AWS, Azure, GCP and others, so they
pose no obstacle to portability between clouds. They can also be run on-prem.
Users wanting managed pubsub could go with Confluent Cloud for Kafka (AWS,
GCP), or CloudAMQP for RabbitMQ (AWS, Azure) without losing much in the way
of portability.

So what’s missing? The solution described above means being locked into a
particular implementation of pubsub. The potential lock-in is even greater
when building systems in terms of the cloud-specific APIs such as GCP PubSub
or Azure Service Bus.

Developers may wish to compare different pubsub systems in terms of their
performance, reliability, cost or other factors, and they may want the option
to move between these systems without too much friction. A pubsub package in
Go Cloud would lower the cost of such experiments and migrations.

## Goals
* Publish messages to an existing topic.
* Receive messages from an existing subscription.
* Performance should not be much worse than 90% as fast as directly using existing pubsub APIs.
* Work well with managed pubsub services on AWS, Azure, GCP and the most used open source pubsub systems.

## Non-goals
* Create new topics. This is consistent with the lack of bucket creation in the Go Cloud blob API but may be revisited if there is sufficient demand and if it can be done in a portable way. [rationale](https://github.com/google/go-cloud/blob/master/internal/docs/design.md#developers-and-operators)
* Create new subscriptions. For the first version of the Go Cloud pubsub package, we will assume that the set of subscribers for a particular topic doesn’t change during the normal operation of the application's system. The subscribers are assumed to correspond to components of a distributed system rather than to users of that system.

## Background
[Pubsub](https://en.wikipedia.org/wiki/Publish%E2%80%93subscribe_pattern) is
a frequently requested feature for the Go Cloud project [github
issue](https://github.com/google/go-cloud/issues/312). A key use case
motivating these requests is to support [event driven
architectures](https://en.wikipedia.org/wiki/Event-driven_architecture).

There are several pubsub systems available that could be made to work with Go Cloud by writing drivers for them. Here is a [table](https://docs.google.com/a/google.com/spreadsheets/d/e/2PACX-1vQ2CML8muCrqhinxOeKTcWtwAeGk-RFFFMjB3O2u5DbbBt9R3YnUQcgRjRp6TySXe1CzSOtPVCsKACY/pubhtml) comparing some of them.

The go-micro project already provides a portability layer (called broker) for several Go pubsub clients. The API is here and the implementations for various pubsub systems are here. The go-micro broker could be used as a reference for designing the Go Cloud pubsub API, bearing in mind that it isn’t structured according to the concrete type + driver paradigm used by Go Cloud.

## Design overview
### Developer’s perspective
Given a topic that has already been created on the pubsub server, messages can be sent to that topic by creating a new `pubsub.Publisher` and calling its `Send` method, like this (assuming a fictional pubsub provider called "acme"):

```go
import (
    "context"
    "log"

    "github.com/google/go-cloud/pubsub" 
    "github.com/google/go-cloud/pubsub/acme/publisher" 
)

func main() {
    if err := send(); err != nil {
        log.Fatal(err)
    }
}

func send() error {
    ctx := context.Background()
    topic := "projects/unicornvideohub/topics/user-signup"
    pub, err := publisher.New(ctx, topic)
    if err != nil { return err }
    defer pub.Close()
    err := pub.Send(ctx, pubsub.Message{ Body: []byte("Alice signed up") })
    if err != nil { return err }
    err := pub.Send(ctx, pubsub.Message{ Body: []byte("Bob signed up") })
    if err != nil { return err }
}
```

The call to `Send` will only return after the message has been sent to the
server or its sending has failed.

Messages can be received from an existing subscription to a topic by
creating a `pubsub.Subscriber` and calling its `Receive` method, like this:

```go
import (
    "context"
    "fmt"
    "log"

    "github.com/google/go-cloud/pubsub" 
    "github.com/google/go-cloud/pubsub/acme/subscriber" 
)

func main() {
    if err := receive(); err != nil {
        log.Fatal(err)
    }
}

func receive() error {
    ctx := context.Background()
    subscriptionID := "projects/unicornvideohub/subscriptions/user-signup-minder"
    sub, err := subscriber.New(ctx, subscriptionID)
    if err != nil { return err }
    defer sub.Close()
    msg, err := sub.Receive(ctx)
    if err != nil { return err }
    // Do something with msg.
    fmt.Printf("Got message: %s\n", msg.Body)
    // Acknowledge that we handled the message.
    if err := msg.Ack(); err != nil {
       return err
    }
}
```

A more realistic subscriber client would process messages in a loop, like this:

```go
import (
    "context"
    "log"
    "os"
    "os/signal"

    "github.com/google/go-cloud/pubsub" 
    "github.com/google/go-cloud/pubsub/acme/subscriber" 
)

func main() {
    if err := receive(); err != nil {
        log.Fatal(err)
    }
}

func receive() error {
    ctx := context.Background()
    subscriptionID := "projects/unicornvideohub/subscriptions/signup-minder"
    sub, err := subscriber.New(ctx, subscriptionID)
    if err != nil { return err }
    defer sub.Close(ctx)

    // Handle ctrl-C.
    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt)
    go func() {
        for sig := range c {
            err := sub.Close(ctx)
            if err != nil { log.Fatal(err) }
        }
    }()

    // Process messages until the user hits ctrl-C.
    for {
        msg, err := sub.Receive(ctx)
        switch err {
        // sub.Close() causes io.EOF to be returned from sub.Receive().
        case io.EOF:
            log.Printf("Got ctrl-C. Exiting.")
            break
        case nil:
        default:
            return err
        }
        log.Printf("Got message: %s\n", msg.Body)
        if err := msg.Ack(); err != nil {
            return err
        }
    }
}
```

The messages can be processed concurrently with an inverted worker pool, like this:
```go
import (
    "context"
    "log"
    "os"
    "os/signal"

    "github.com/google/go-cloud/pubsub" 
    "github.com/google/go-cloud/pubsub/acme/subscriber" 
)

func main() {
    if err := receive(); err != nil {
        log.Fatal(err)
    }
}

func receive() error {
    ctx := context.Background()
    subscriptionID := "projects/unicornvideohub/subscriptions/user-signup-minder"
    sub, err := subscriber.New(ctx, subscriptionID)
    if err != nil { return err }
    defer sub.Close(ctx)

    // Handle ctrl-C.
    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt)
    go func() {
        for sig := range c {
            if err := sub.Close(ctx); err != nil {
                log.Fatal(err)
            }
        }
    }()

    // Process messages until the user hits ctrl-C.
    const poolSize = 10
    // Use a buffered channel as a semaphore.
    sem := make(chan token, poolSize)
    for {
        msg, err := sub.Receive(ctx)
        switch err {
        // sub.Close() causes io.EOF to be returned from sub.Receive().
        case io.EOF:
            log.Printf("Got ctrl-C. Exiting.")
            break
        case nil:
        default:
            return err
        }

        sem <- token{}
        go func(msg *pubsub.Message) {
            log.Printf("Got message: %s", msg.Body)
            if err := msg.Ack(); err != nil {
                log.Printf("Failed to ack message: %v", err)
            }
            <-sem
        }(msg)
    }
    for n := poolSize; n > 0; n-- {
        sem <- token{}
    }
}
```

### Driver implementer’s perspective
Adding support for a new pubsub system involves two main steps:

1. Write the driver, implementing the interfaces in the github.com/go-cloud/pubsub/driver package. The new driver could be located at github.com/go-cloud/pubsub/$newsystem/driver.
2. Write the concrete implementation, including publisher.New to construct a pubsub.Publisher and subscriber.New to construct a pubsub.Subscriber. These constructors could be located at github.com/go-cloud/pubsub/$newsystem/publisher and github.com/go-cloud/pubsub/$newsystem/subscriber.

The driver interfaces are batch-oriented because some pubsub systems can more efficiently deal with batches of messages than with one at a time. Streaming was considered but it does not appear to provide enough of a performance gain to be worth the additional complexity that it would impose.

The driver interfaces will be located in the github.com/google/go-cloud/pubsub/driver package and will look something like this:

```go
type Message struct {
    // Body contains the content of the message.
    Body []byte

    // Attributes has key/value metadata for the message.
    Attributes map[string]string

    // AckID identifies the message on the server.
    // It can be used to ack the message after it has been received.
    AckID interface{}
}

// Publisher publishes messages.
type Publisher interface {
    // SendBatch publishes all the messages in ms.
    SendBatch(ctx context.Context, ms []*Message) error

    // Close disconnects the Publisher.
    Close() error
}

// Subscriber receives published messages.
type Subscriber interface {
    // ReceiveBatch returns a batch of messages that have queued up for the
    // subscription on the server.
    ReceiveBatch(ctx context.Context) ([]*Message, error)

    // SendAcks acknowledges the messages with the given ackIDs on the
    // server so that they
    // will not be received again for this subscription. This method
    // returns only after all the ackIDs are sent.
    SendAcks(ctx context.Context, ackIDs []interface{}) error

    // Close disconnects the Subscriber.
    Close() error
}
```

## Detailed design
The developer experience of using Go Cloud pubsub involves sending, receiving and acknowledging one message at a time, all in terms of synchronous calls. Behind the scenes, the driver implementations deal with batches of messages and acks. The concrete API, to be written by the Go Cloud team, takes care of creating the batches in the case of Send or Ack, and dealing out messages one at a time in the case of Receive.

The concrete API will be located at github.com/google/go-cloud/pubsub and will probably look like this:
```go
package pubsub

import (
        "context"
        "github.com/google/go-cloud/pubsub/driver"
)

// Message contains data to be published.
type Message struct {
    // Body contains the content of the message.
    Body []byte

    // Attributes contains key/value pairs with metadata about the message.
    Attributes map[string]string

    // ackID is an ID for the message on the server, used for acking.
    ackID AckID

    // sub is the Subscriber this message was received from.
    sub *Subscriber
}

type AckID interface{}

type AckNotSupportedError struct {
    Provider string
}

func (e AckNotSupportedError) Error() string {
    return fmt.Sprintf("The %s implementation of pubsub does not support Acks.", e.Provider)
}

// Ack acknowledges the message, telling the server that it does not need to
// be sent again to the associated Subscriber. This method blocks until
// the message has been confirmed as acknowledged on the server, or failure
// occurs. An AckNotSupportedError can be returned for pubsub systems that
// do not support Acks.
func (m *Message) Ack() error { … }

// Publisher publishes messages to all its subscribers.
type Publisher struct {
    p driver.Publisher
}

// Send publishes a message. It only returns after the message has been
// sent, or failed to be sent. Send can be called from multiple goroutines
// at once.
func (p *Publisher) Send(ctx context.Context, m *Message) error { … }

// Close disconnects the Publisher.
func (p *Publisher) Close() error

// Subscriber receives published messages.
type Subscriber struct {
    s driver.Subscriber
}

// Receive receives and returns the next message from the Subscriber's queue,
// blocking if none are available. This method can be called concurrently
// from multiple
// goroutines. On systems that support acks, the Ack() method of the
// returned Message has to be called once
// the message has been processed, to prevent it from being received again.
func (s *Subscriber) Receive(ctx context.Context) (*Message, error) { … }

// Close disconnects the Subscriber.
func (s *Subscriber) Close() error { … }
```

## Alternative designs considered

### Batch oriented concrete API
In this design, `pub.Send` and `sub.Receive` are done in terms of batches of messages.

Here is an example of how it would look from the developer's perspective:
```go
import (
    "github.com/go-cloud/pubsub"
    // For example, gcppubsub, azurebuspubsub, amazonmqpubsub
    "github.com/go-cloud/pubsub/somepubsub"
)

…
topic := "user-signup"
client, err := somepubsub.NewClient(ctx, ...)
if err != nil { /* handle err */ }
defer client.Close()
pub, err := somepubsub.NewPublisher(ctx, client, topic)
if err != nil { /* handle err */ }
defer pub.Close()

// Listen to an existing subscription with a known ID, processing messages with
// a pool of workers.
subscriptionID := "user-signup-subscription-1"
sub, err := somepubsub.NewSubscriber(ctx, client, subscriptionID)
if err != nil { /* handle err */ }
defer sub.Close()

// Receive and process the messages.
for {
    msgs, err := sub.Receive(ctx, 10)
    // Clean shutdown if sub.Close was called.
    if err == io.EOF { return }
    if err != nil { /* handle err */ }
    acks := []pubsub.AckID
    for _, msg := range msgs {
        // Do something with msg.
        fmt.Printf("Got message: %q\n", msg.Body)
        acks = append(acks, msg.AckID)
    }
    err := sub.SendAcks(ctx, acks)
    if err != nil { /* handle err */ }
}

// Publish some messages.
msgs := []string {"Alice signed up", "Bob signed up"}
for _, m := range msgs {
    err = pub.Send(ctx, &pubsub.Message{ Body: []byte(m) })
    if err != nil { /* handle err */ }
}
```

Here’s what it might look like to use this batch-only API with the inverted worker pool pattern:
```go
// Receive the messages and forward them to a chan.
msgsChan := make(chan *pubsub.Message)
go func() {
    for {
        msgs, err := sub.Receive(ctx, 10)
        // Clean shutdown if sub.Close was called.
        if err == io.EOF { return }
        if err != nil { /* handle err */ }
        for _, m := range msgs {
            msgsChan <- m
        }
    }
}

// Get the acks from a chan and send them back to the
// server in batches.
acksChan := make(chan pubsub.AckID)
go func() {
    for {
        const batchSize = 10
        batch := make([]pubsub.AckID, batchSize)
        for i := 0; i < len(batch); i++ {
            batch[i] = <-acksChan
        }
        if err := sub.SendAcks(ctx, batch); err != nil {
            /* handle err */
        }
    }
}

// Receive and process the messages with an inverted worker pool.
const poolSize = 10
sem := make(chan token, poolSize)
for m := range msgsChan {
    sem <- token{}
    go func(msg *Message) {
        fmt.Printf("Got message: %q\n", msg.Body)
        acksChan <- msg.AckID
        <-sem
    }(msg)

}
for n := poolSize; n > 0; n-- {
    sem <- token{}
}
```

Pro:
* The semantics are simple, making it straightforward to implement the concrete API and the drivers for most pubsub providers, making it easy for developers to reason about how it will behave, and minimizing the risk that bugs will be present in the concrete API.
* Efficient sending and receiving of messages is possible by tuning batch size and the number of goroutines sending or receiving messages.

Con:
* It makes the inverted worker pool pattern verbose.
* Apps needing to send more than a few thousand messages per second (see benchmark below) need their own logic for creating batches with size greater than 1.

### go-micro
Here is a sketch of what Go Cloud pubsub could look like from a developer's perspective, inspired by `go-micro`'s `broker` package:

```go
b := somepubsub.NewBroker(...)
err := b.Connect()
if err != nil { /* handle err */ }
topic := "user-signups"
// Listen to an existing subscription
subID := "user-signups-subscription-1"
sub, err := b.Subscription(ctx, topic, subID, func(pub broker.Publication) error {
    fmt.Printf("%s\n", pub.Message.Body)
    return nil
})
err = b.Publish(ctx, topic, &broker.Message{ Body: []byte("alice signed up") })
if err != nil { /* handle err */ }
// Sometime later:
err = sub.Unsubscribe(ctx)
if err != nil { /* handle err */ }
```

Pro:
* The callback to the subscription returning an error to decide about ack/nack is concise, and might allow unportable message IDs to be hidden entirely in the driver.

Con:
* Go micro has code to auto-create topics and subscriptions as needed, but this is not consistent with Go Cloud’s design principle to not get involved in ops.

### Google's pubsub2


```go
topic := "/pubsub2/user-signups"

// Make a subscriber.
sub, err := subscriber.New(ctx, topic, subscriber.EphemeralId(), nil /* opts */)
if err != nil { /* handle err */ }
defer sub.Close(ctx)

// Consume messages.
go func() {
        for item := range sub.Chan() {
                log.Infof("Got message: %s\n", item.Message.Data)
                item.Ack()
        }
}()

// Make a publisher.
pub, err := publisher.New(ctx, topic, publisher.EphemeralId(), nil /* opts */)
if err != nil { /* handle err */ }

// Close will disconnect the publisher from pubsub2.
defer pub.Close(ctx)

// Publish a message.
err = pub.SendString(ctx, pub, "alice signed up")
if err != nil { /* handle err */ }
```

Pro:
* The API looks simple, only requiring a small number of calls.
* publisher and subscriber are clear names.
Efficient batching is done behind the scenes.

Con:
* Exposing a channel in the API with the Chan() method means having no way to cancel a receive.

## Pubsub acknowledgements
In pubsub systems with acknowledgement, messages are kept in a queue associated with the subscription on the server. When a client receives one of these messages, its counterpart on the server is marked as being processed. Once the client finishes processing the message, it sends an acknowledgement (or "ack") to the server and the server removes the message from the subscription queue. There may be a deadline for the acknowledgement, past which the server unmarks the message so that it can be received again for another try at processing.

Redis and ZeroMQ don’t support Ack/Nack, but many others do including GCP PubSub, Azure Service Bus, and RabbitMQ. Given the wide support and usefulness, it makes sense to support this in Go Cloud. For systems that don't have acknowledgement, such as Redis, it is probably best for the associated Go Cloud driver to simulate it so that users of Go Cloud pubsub never have to worry about whether it will be supported on different implementations.

Let’s look at some possibilities for how message acknowledgement could look from a developer's perspective.

### Option 1: `Receive` method returns an `ack` func
Usage would look like this:
```go
for {
        msg, ack, err := sub.Receive(ctx)
        // Do something with msg...
        if /* something went wrong */ {
                // Don't acknowledge the message. Let it time out and be
                // redelivered.
                continue
        }
        ack(msg)
}
```
Pro:
* The compiler will complain if the returned `ack` function is not used.

Con:
* Receive has one more return value.
* Passing `ack` around along with `msg` is inconvenient.

## Tests
* Sent messages with random contents are received with the same contents.
* Sent messages with random attributes are received with the same attributes.
* Error occurs when making a local topic with an ID that doesn’t exist on the server.
* Error occurs when making a subscription with an ID that doesn’t exist on the server.
* Message gets sent again after ack deadline if a message is never acknowledged.
* Acked messages don't get received again after waiting twice the ack deadline.

## Benchmarks
Compare messages per second for one publisher and different numbers of subscribers on raw API versus Go Cloud for the first implementations.

## References
* Github issue: https://github.com/google/go-cloud/issues/312
* AWS Go API documentation for SNS (Simple Notification Service)
* http://queues.io/
* http://www.imatix.com/articles:whats-wrong-with-amqp/
