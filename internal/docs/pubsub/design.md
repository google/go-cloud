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

Publish messages to an existing topic.
Receive messages from an existing subscription.
Performance should not be much worse than 90% as fast as directly using existing pubsub APIs.
Work well with managed pubsub services on AWS, Azure, GCP and the most used open source pubsub systems.

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
Write the concrete implementation, including publisher.New to construct a pubsub.Publisher and subscriber.New to construct a pubsub.Subscriber. These constructors could be located at github.com/go-cloud/pubsub/$newsystem/publisher and github.com/go-cloud/pubsub/$newsystem/subscriber.

The driver interfaces are batch-oriented because some pubsub systems can more efficiently deal with batches of messages than with one at a time. Streaming was considered but it does not appear to provide enough of a performance gain to be worth the additional complexity that it would impose.

The driver interfaces will be located in the github.com/google/go-cloud/pubsub/driver package and will look something like this:

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
DETAILED DESIGN
The developer experience of using Go Cloud pubsub involves sending, receiving and acknowledging one message at a time, all in terms of synchronous calls. Behind the scenes, the driver implementations deal with batches of messages and acks. The concrete API, to be written by the Go Cloud team, takes care of creating the batches in the case of Send or Ack, and dealing out messages one at a time in the case of Receive.

The concrete API will be located at github.com/google/go-cloud/pubsub and will probably look like this:
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

ALTERNATIVES CONSIDERED
DESIGNS
BATCH ORIENTED CONCRETE API
In this design, pub.Send and sub.Receive are done in terms of batches of messages.
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

Here’s what it might look like to use this batch-only API with the inverted worker pool pattern:
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

Pro:
The semantics are simple, making it straightforward to implement the concrete API and the drivers for most pubsub providers, making it easy for developers to reason about how it will behave, and minimizing the risk that bugs will be present in the concrete API.
Efficient sending and receiving of messages is possible by tuning batch size and the number of goroutines sending or receiving messages.
Con:
It makes the inverted worker pool pattern verbose.
Apps needing to send more than a few thousand messages per second (see benchmark below) need their own logic for creating batches with size greater than 1.
LIKE GO-MICRO
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

Pro:
The callback to the subscription returning an error to decide about ack/nack is concise, and might allow unportable message IDs to be hidden entirely in the driver.
Con:
Go micro has code to auto-create topics and subscriptions as needed, but this is not consistent with Go Cloud’s design principle to not get involved in ops.

Inspiration: go-micro broker
LIKE GOOGLE3 PUBSUB2
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

[google3 pubsub2 Go example]

Pro:
The API looks simple, only requiring a small number of calls.
publisher and subscriber are clear names.
Efficient batching is done behind the scenes.
Con:
Exposing a channel in the API with the Chan() method means having no way to cancel a receive.
The Go pubsub2 client violates the Go Best Practice: Avoid Concurrency in Your API, by spawning goroutines behind the scenes from the moment a subscriber is created. Because of this, forgetting to call Close on a Subscription means goroutines are leaked.

Implement in terms of RPCs doing batches or single messages?
Can we use an implementation that operates on a single message at a time in the drivers? Only if we can accept a max throughput of a few thousand messages per second on GCP and likely others.

Here is the result of running a benchmark showing messages per second for various batch sizes and numbers of goroutines on a GCE instance, publishing messages to GCP PubSub. The optimum is about 100 goroutines sending batches of 1000 messages, yielding over 800k messages per second. With a batch size of 1, the max throughput is about 2k messages per second.
$ go run bench.go
 batch size| # goroutines|msgs/sec
 ----------| ------------|--------
          1|            1|4.5e+01
          1|           10|9.4e+02
          1|          100|2.3e+03
          1|         1000|2.1e+03
         10|            1|1.2e+03
         10|           10|1.1e+04
         10|          100|2.2e+04
         10|         1000|2.1e+04
        100|            1|8.8e+03
        100|           10|7.9e+04
        100|          100|2.1e+05
        100|         1000|1.9e+05
       1000|            1|2.4e+04
       1000|           10|2.0e+05
       1000|          100|8.2e+05
       1000|         1000|6.0e+05



Auto-Create Topics?
When publishing to a topic ID that doesn’t exist yet on the server, should Go Cloud drivers detect this case and automatically create the topic on the fly? No, because topic creation is an operator concern. Note that some providers such as Redis always create topics on the fly, so for them this is moot.
Expose Named Subscriptions?
Yes. Some pubsub systems, such as GCP PubSub and Azure Service Bus have named subscriptions, but others such as Redis do not. The STOMP protocol uses named subscriptions, so this is probably a fairly portable thing to do.
Auto-Create Subscriptions?
Provider decides. Providers can decide whether listening on a subscription with an unrecognized name will cause server resources to be allocated for it, or whether it will cause an error to be returned.
Guarantee Delivery?
This probably has to be provider specific. It would be unreasonable to expect Go Cloud to somehow paper over the differences between providers as far as avoiding repeated delivery or ensuring at least once delivery.
Authentication and Provisioning
These are operator concerns, so not explicitly handled by the API.
Subscribe to Topics Based on Patterns?
No, this isn’t portable. It works on Redis but not on GCP PubSub for example.
Implicitly publish and receive batches of messages?
Yes. There is precedent: GCP PubSub does it and pubsub2 does it. The goroutines spawned behind the scenes are terminated when the Close method is called.
Naming
“Topic”, “Publisher”, “Destination” or “Channel”?
The consensus seems to be “Publisher”. “Topic” is short and clear but is an unpopular choice on the Go Cloud team. “Destination” sounds like a place to take a vacation. “Channel” seems too easy to confuse with Go channels.

GCP uses “Topic”. STOMP uses “Destination”. RabbitMQ uses “Channel”.
“Publish” or “Send”?
Let’s go with Publish since people will be looking for such a method with a package called pubsub.

“Send” is probably better because it is shorter and it goes with “Receive”. We “Send” messages and “Receive” them. We could also say we “Publish” messages but we don’t “Subscribe” them.

STOMP and ZeroMQ both use “Send”. Various others use “Publish”.
“Topic” or “Publisher”?
“Topic” is shorter and about equally clear.
“Subscription” or “Subscriber” or “Queue”?
“Subscriber” goes with “Publisher”, but we’re going with “Topic” so it probably makes sense to use “Subscription”. “Queue” would make the pulling nature clearer, but would obscure the connection to a pubsub system.
“Next”, “Receive”, or “Recv”?
“Receive” goes with “Send” and is more suggestive of the network activity that may be involved.

“Next” is consistent with the Iterator Guidelines.
Publication acks or synchronous Publish method?
A synchronous Publish method that returns only after the message has been delivered, or failed to be delivered, would be fine.
Message consumption via callbacks, iterators, or channels?
Callbacks have the advantage that their error returns can be used by the system to decide whether to Ack or Nack the message. This fits well with the idea that processing a message is like a transaction in which acking is like committing and nacking is like rolling back, and the callback function is the scope of that transaction. Callbacks can be used in an asynchronous way by running them in goroutines, or synchronously by simply calling them.

Channels are attractive for a couple of reasons. For one, they allow the use of range. Also, cancellation can be done with channels by using a select statement, although that won’t cancel any networking calls that are being done to get the next item in the channel. Errors can be supported by adding a field of type error to the structs being sent over the channel, as is done in go-stomp.

Drawbacks of channels include the necessity of spawning at least one goroutine behind the scenes to feed the channel, and the ease of ignoring any error field in the structs being sent over the channel. Callbacks also do not appear to be compatible with inverted worker pools.

Iterators don’t work with range but they do allow cancellation and can return errors when appropriate. They do not require any goroutines to be spawned. 
Transactions?
No. GCP PubSub doesn’t have them, for one. Adding them later is probably possible, and it’s better to be demand-driven. See also the discussion in Pieter Hintjen’s article: “Transactions - the conventional unit of reliability - also fit very uncomfortably on top of asynchronous message transportation”. 
Acknowledgement API
In pubsub systems with acknowledgement, messages are sent again, possibly to another subscriber, if they are either non-acknowledged (nacked) or not acknowledged within some deadline.

Redis and ZeroMQ don’t support Ack/Nack, but many others do including GCP PubSub, Azure Service Bus, and RabbitMQ. Given the wide support and usefulness, it makes sense to support this in Go Cloud, probably with a no-op on systems that don’t have it.

Let’s look at some ways that acknowledgement could work in Go Cloud pubsub.

Option 0: Receive method returns a closure that acks or nacks

Usage would look like this.
for {
        msg, ack, err := sub.Receive(ctx)
        // Do something with msg...
        if somethingWentWrong {
                ack(msg, false) // a.k.a. "nack"
                continue
        }
        ack(msg, true)
}
Pro:
Caller has to deliberately not acknowledge by using the blank identifier a wildcard (msg, _, err := sub.Receive(ctx)). Otherwise, the compiler will complain if no ack is done. 

Con:
Receive has one more return value.
Passing the ack function around is inconvenient.

The driver interface would have a similar method:

type Subscription interface {
    // ...
    func Receive(context.Context) (Message, func(Message, bool), error)
    // ...
}

ack may hit the network, so it might also need to take a ctx parameter.

Option 0a: Receive method returns separate ack and nack closures
msg, ack, nack, err := sub.Receive(ctx)
if somethingWentWrong {
    nack(msg)
}
ack(msg)
This may be a bit clearer, though it may also be an annoyance if nack isn’t needed.

Option 0b: ack closure takes a pointer to error, as in sqliteutil

func processNextMessage(ctx context.Context, sub *pubsub.Subscription) (err error) {
    msg, ack, err := sub.Receive(ctx)  
    defer ack(&err)  // ack just looks at whether err is nil, ignoring its contents
    // Do something with msg.
}
…
for {
    err := processNextMessage(ctx, sub)
}

See https://godoc.org/crawshaw.io/sqlite/sqliteutil#Save.

Pro:
Being able to defer the ack.
Con:
Using a pointer to an error interface is peculiar.
Only the non-nilness of the error pointer referent is used.
Acking may occur for a message that resulted in a processing error, and nacking could happen for non-error situations such as shutdown or cancellation.

Option 1: Have a driver.Message struct with a field of type driver.Acker that can ack or nack the message

It could look something like this:

type Message struct {
        Data []byte
        // … more pure fields

        Acker Acker
}

type Acker interface {
        // Errors on these methods could mean either transient failure or
        // lack of support. 
        Ack(context.Context) error
        Nack(context.Context) error
}
Pro:
Looks similar to the GCP API, although that has Ack() and Nack() methods directly on its Message type
Message seems like the natural target of acknowledgement

Con:
The operation of the Acker on the Message is implicit.
It’s easy to forget to call Ack/Nack and the compiler won’t say anything about it.
Option 2: Separate the pure data of driver.Message from mutable state of acknowledgement 
type Message struct {
        Data []byte
        // … more pure fields
}

type Subscription interface {
        // … regular subscription stuff

        // Ack acknowledges that a message has been processed.
        // If acknowledgement is not supported, an error is returned.
        // An error could also signal a transient failure.
        Ack(context.Context, Message) error

        // Ack reports that a message has failed to be processed.
        // If dis-acknowledgement is not supported, an error is returned.
        // An error could also signal a transient failure.
        Nack(context.Context, Message) error
}
Pro:
Target of Ack and Nack (a Message) is made explicit
Message is strictly a value, having no state

Con:
Transient ack/nack failures and total lack of ack/nack support are both signaled by errors from Ack and Nack, making it harder to distinguish them.

Option 3: Subscription returns an Acker that acts on Message
type Message struct {
        Data []byte
        // … more pure fields
}

type Subscription interface {
        // … regular subscription stuff

        // Acker returns an Acker, or an error if acknowledgement is not supported.
        Acker() (Acker, error)  
}

type Acker interface {
        // Errors on these methods mean transient failure, not lack of support. 
        Ack(context.Context, Message) error
        Nack(context.Context, Message) error
}

Pro:
The Message type is just a value, as in Option 2.
Acknowledgement can be obtained once from a subscription and then passed around as a capability.
Lack of support for acknowledgement is signaled by an error from Subscription.Acker(). Transient ack failures are signaled separately by errors from Acker.Ack() and Acker.Nack(). 
Con:
Yet another type
TESTING PLAN
Tests:
What happens when making a local topic with an ID that doesn’t exist on the server? An error.
What happens when making a subscription with an ID that doesn’t exist on the server? An error.
What happens if a message is never acknowledged? Ack deadline is hit. Message gets sent again.
What happens if messages in a batch are acknowledged in the order they were received? They never get sent again.
What if all messages in a batch are nacked? They all get sent again.

Benchmarks
Compare messages per second for one publisher and ten subscribers on raw API versus Go Cloud for AWS, GCP, Azure, RabbitMQ
MORE READING
Github issue: https://github.com/google/go-cloud/issues/312
Message structure in pubsub APIs
Go Cloud Pub/Sub meeting notes
Go Cloud design
AWS Go API documentation for SNS (Simple Notification Service)
http://queues.io/
http://zguide.zeromq.org/
http://www.imatix.com/articles:whats-wrong-with-amqp/



