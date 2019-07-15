# Go CDK `pubsub` Design

## Summary

This document proposes a new `pubsub` package for the Go CDK.

## Motivation

A developer designing a new system with cross-cloud portability in mind could
choose a messaging system supporting pubsub, such as ZeroMQ, Kafka or RabbitMQ.
These pubsub systems run on AWS, Azure, GCP and others, so they pose no obstacle
to portability between clouds. They can also be run on-prem. Users wanting
managed pubsub could go with Confluent Cloud for Kafka (AWS, GCP), or CloudAMQP
for RabbitMQ (AWS, Azure) without losing much in the way of portability.

So what’s missing? The solution described above means being locked into a
particular implementation of pubsub. There is also a potential for lock-in when
building systems in terms of the cloud-specific services such as AWS SNS+SQS,
GCP PubSub or Azure Service Bus.

Developers may wish to compare different pubsub systems in terms of their
performance, reliability, cost or other factors, and they may want the option to
move between these systems without too much friction. A `pubsub` package in the
Go CDK could lower the cost of such experiments and migrations.

## Goals

*   Publish messages to an existing topic.
*   Receive messages from an existing subscription.
*   Perform not much worse than 90% compared to directly using the APIs of
    various pubsub systems.
*   Work well with managed pubsub services on AWS, Azure, GCP and the most used
    open source pubsub systems.

## Non-goals

*   Create new topics in the cloud. The Go CDK focuses on developer concerns,
    but topic creation is an
    [operator concern](https://github.com/google/go-cloud/blob/master/internal/docs/design.md#developers-and-operators).

*   Create new subscriptions in the cloud. The subscribers are assumed to
    correspond to components of a distributed system rather than to users of
    that system.

## Background

[Pubsub](https://en.wikipedia.org/wiki/Publish%E2%80%93subscribe_pattern) is a
frequently requested feature for the Go CDK project
\[[github issue](https://github.com/google/go-cloud/issues/312)]. A key use case
motivating these requests is to support
[event driven architectures](https://en.wikipedia.org/wiki/Event-driven_architecture).

There are several pubsub systems available that could be made to work with the
Go CDK by writing drivers for them. Here is a
[table](https://docs.google.com/a/google.com/spreadsheets/d/e/2PACX-1vQ2CML8muCrqhinxOeKTcWtwAeGk-RFFFMjB3O2u5DbbBt9R3YnUQcgRjRp6TySXe1CzSOtPVCsKACY/pubhtml)
comparing some of them.

## Design overview

### Developer’s perspective

Given a topic that has already been created on the pubsub server, messages can
be sent to that topic by calling `acmepubsub.OpenTopic` and calling the `Send`
method of the returned `Topic`, like this (assuming a fictional pubsub service
called "acme"):

```go
package main

import (
    "context"
    "log"
    "net/http"

    rawacmepubsub "github.com/acme/pubsub"
    "github.com/google/go-cloud/pubsub"
    "github.com/google/go-cloud/pubsub/acmepubsub"
)

func main() {
    log.Fatal(serve())
}

func serve() error {
    ctx := context.Background()
    client, err := rawacmepubsub.NewClient(ctx, "unicornvideohub")
    if err != nil {
        return err
    }
    t, err := acmepubsub.OpenTopic(ctx, client, "user-signup", nil)
    if err != nil {
        return err
    }
    defer t.Close()
    http.HandleFunc("/signup", func(w http.ResponseWriter, r *http.Request) {
        err := t.Send(r.Context(), pubsub.Message{Body: []byte("Someone signed up")})
        if err != nil {
            log.Println(err)
        }
    })
    return http.ListenAndServe(":8080", nil)
}
```

The call to `Send` will only return after the message has been sent to the
server or its sending has failed.

Messages can be received from an existing subscription to a topic by calling the
`Receive` method on a `Subscription` object returned from
`acmepubsub.OpenSubscription`, like this:

```go
package main

import (
    "context"
    "fmt"
    "log"

    rawacmepubsub "github.com/acme/pubsub"
    "github.com/google/go-cloud/pubsub"
    "github.com/google/go-cloud/pubsub/acmepubsub"
)

func main() {
    if err := receive(); err != nil {
        log.Fatal(err)
    }
}

func receive() error {
    ctx := context.Background()
    client, err := rawacmepubsub.NewClient(ctx, "unicornvideohub")
    if err != nil {
        return err
    }
    s, err := acmepubsub.OpenSubscription(ctx, client, "user-signup-minder", nil)
    if err != nil {
        return err
    }
    defer s.Close()
    msg, err := s.Receive(ctx)
    if err != nil {
        return err
    }
    // Do something with msg.
    fmt.Printf("Got message: %s\n", msg.Body)
    // Acknowledge that we handled the message.
    msg.Ack()
}
```

A more realistic subscriber client would process messages in a loop, like this:

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"

    "github.com/google/go-cloud/pubsub"
    "github.com/google/go-cloud/pubsub/acmepubsub"
)

func main() {
    if err := receive(); err != nil {
        log.Fatal(err)
    }
}

func receive() error {
    ctx := context.Background()
    client, err := rawacmepubsub.NewClient(ctx, "unicornvideohub")
    if err != nil {
        return err
    }
    s, err := acmepubsub.OpenSubscription(ctx, client, "signup-minder", nil)
    if err != nil {
        return err
    }
    defer s.Close()

    // Process messages.
    for {
        msg, err := s.Receive(ctx)
        if err {
            return err
        }
        log.Printf("Got message: %s\n", msg.Body)
        msg.Ack()
    }
}
```

The messages can be processed concurrently with an
[inverted worker pool](https://www.youtube.com/watch?v=5zXAHh5tJqQ&t=26m58s),
like this:

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"

    "github.com/google/go-cloud/pubsub"
    "github.com/google/go-cloud/pubsub/acmepubsub"
)

func main() {
    if err := receive(); err != nil {
        log.Fatal(err)
    }
}

func receive() error {
    ctx := context.Background()
    client, err := rawacmepubsub.NewClient(ctx, "unicornvideohub")
    if err != nil {
        return err
    }
    s, err := acmepubsub.OpenSubscription(ctx, client, "user-signup-minder", nil)
    if err != nil {
        return err
    }
    defer s.Close()

    // Process messages.
    const poolSize = 10
    // Use a buffered channel as a semaphore.
    sem := make(chan struct{}, poolSize)
    for {
        msg, err := s.Receive(ctx)
        if err {
            return err
        }
        sem <- struct{}{}
        go func() {
            log.Printf("Got message: %s", msg.Body)
            msg.Ack()
            <-sem
        }()
    }
    for n := poolSize; n > 0; n-- {
        sem <- struct{}{}
    }
}
```

### Driver implementer’s perspective

Adding support for a new pubsub system involves the following steps, continuing
with the "acme" example:

1.  Add a new package called `acmepubsub`.
2.  Add private `topic` and `subscription` types to `acmepubsub` implementing
    the corresponding interfaces in the `github.com/go-cloud/pubsub/driver`
    package.
3.  Add `func OpenTopic(...)` that creates an `acmepubsub.topic` and returns a
    concrete `pubsub.Topic` object made from it.
4.  Add `func OpenSubscription(...)` that creates an `acmepubsub.subscription`
    and returns a `pubsub.Subscription` object made from it.

Here is a sketch of what the `acmepubsub` package could look like:

```go
package acmepubsub

import (
    "context"

    rawacmepubsub "github.com/acme/pubsub"
    "github.com/google/go-cloud/pubsub"
    "github.com/google/go-cloud/pubsub/driver"
)

// OpenTopic opens an existing topic on the pubsub server and returns a Topic
// that can be used to send messages to that topic.
func OpenTopic(ctx context.Context, client *rawacmepubsub.Client, topicName string) (*pubsub.Topic, error) {
    rt, err := client.Topic(ctx, topicName)
    if err != nil {
        return nil, err
    }
    rt, err := client.Topic(ctx, topicName)
    if err != nil {
        return err
    }
    t := &topic{ rawTopic: rt }
    return pubsub.NewTopic(t)
}

// OpenSubscription opens an existing subscription on the server and returns a
// Subscription that can be used to receive messages.
func OpenSubscription(ctx context.Context, client *rawacmepubsub.Client, subscriptionName string) (*pubsub.Subscription, error) {
    rs, err := client.Subscription(ctx, subscriptionName)
    if err != nil {
        return err
    }
    s := &subscription{ rawSub: rs }
    return pubsub.NewSubscription(s)
}

type topic struct {
    rawTopic    *rawacmepubsub.Topic
}

func (t *topic) SendBatch(ctx context.Context, []*pubsub.Message) error {
    // ...
}

func (t *topic) Close() error {
    // ...
}

type subscription struct {
    rawSub  *rawacmepubsub.Subscription
}

func (s *subscription) ReceiveBatch(ctx context.Context) ([]*pubsub.Message, error) {
    // ...
}

func (s *subscription) SendAcks(ctx context.Context, []pubsub.AckID) error {
    // ...
}

func (s *subscription) Close() error {
    // ...
}
```

The driver interfaces are batch-oriented because some pubsub systems can more
efficiently deal with batches of messages than with one at a time. Streaming was
considered but it does not appear to provide enough of a performance gain to be
worth the additional complexity of supporting it across different pubsub systems
\[[benchmarks](https://github.com/ijt/pubsub/tree/master/benchmarks)].

The driver interfaces will be located in the
`github.com/google/go-cloud/pubsub/driver` package and will look something like
this:

```go
package driver

type AckID interface{}

type Message struct {
    // Body contains the content of the message.
    Body []byte

    // Attributes has key/value metadata for the message.
    Attributes map[string]string

    // AckID identifies the message on the server.
    // It can be used to ack the message after it has been received.
    AckID AckID
}

// Topic publishes messages.
type Topic interface {
    // SendBatch publishes all the messages in ms.
    SendBatch(ctx context.Context, ms []*Message) error

    // Close disconnects the Topic.
    Close() error
}

// Subscription receives published messages.
type Subscription interface {
        // ReceiveBatch should return a batch of messages that have queued up
        // for the subscription on the server.
        //
        // If there is a transient failure, this method should not retry but
        // should return a nil slice and an error. The concrete API will take
        // care of retry logic.
        //
        // If the service returns no messages for some other reason, this
        // method should return the empty slice of messages and not attempt to
        // retry.
        //
        // ReceiveBatch is only called sequentially for individual
        // Subscriptions.
    ReceiveBatch(ctx context.Context) ([]*Message, error)

    // SendAcks acknowledges the messages with the given ackIDs on the
    // server so that they
    // will not be received again for this subscription. This method
    // returns only after all the ackIDs are sent.
    SendAcks(ctx context.Context, ackIDs []interface{}) error

    // Close disconnects the Subscription.
    Close() error
}
```

## Detailed design

The developer experience of using Go CDK's pubsub involves sending, receiving
and acknowledging one message at a time, all in terms of synchronous calls.
Behind the scenes, the driver implementations deal with batches of messages and
acks. The concrete API, to be written by the Go CDK team, takes care of creating
the batches in the case of Send or Ack, and dealing out messages one at a time
in the case of Receive.

The concrete API will be located at `github.com/google/go-cloud/pubsub` and will
look something like this:

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

    // sub is the Subscription this message was received from.
    sub *Subscription

    // isAcked is true if Ack has been called on this message.
    isAcked bool
}

type AckID interface{}

// Ack acknowledges the message, telling the server that it does not need to
// be sent again to the associated Subscription. This method returns
// immediately. If Ack has already been called on the message, Ack panics.
func (m *Message) Ack() {
    // Send the ack ID back to the subscriber for batching.
        // The ack is sent to the server in a separate goroutine
        // managed by the Subscription from which this message was
        // received.
        // ...
}

// Topic publishes messages to all its subscribers.
type Topic struct {
    driver   driver.Topic
    mcChan   chan msgCtx
    doneChan chan struct{}
}

// msgCtx pairs a Message with the Context of its Send call.
type msgCtx struct {
    msg *Message
    ctx context.Context
}

// Send publishes a message. It only returns after the message has been
// sent, or failed to be sent. The call will fail if ctx is canceled.
// Send can be called from multiple goroutines at once.
func (t *Topic) Send(ctx context.Context, m *Message) error {
        // Send this message over t.mcChan and then wait for the batch including
        // this message to be sent to the server.
        // ...
}

// Close disconnects the Topic.
func (t *Topic) Close() error {
    close(t.doneChan)
    return t.driver.Close()
}

// NewTopic makes a pubsub.Topic from a driver.Topic.
func NewTopic(d driver.Topic) *Topic {
    t := &Topic{
        driver:   d,
        mcChan:   make(chan msgCtx),
        doneChan: make(chan struct{}),
    }
    go func() {
        // Pull messages from t.mcChan and put them in batches. Send the current
        // batch whenever it is large enough or enough time has elapsed since
        // the last send.
        // ...
    }()
    return t
}

// Subscription receives published messages.
type Subscription struct {
    driver driver.Subscription

    // ackChan conveys ackIDs from Message.Ack to the ack batcher goroutine.
    ackChan chan AckID

    // ackErrChan reports errors back to Message.Ack.
    ackErrChan chan error

    // doneChan tells the goroutine from startAckBatcher to finish.
    doneChan chan struct{}

    // q is the local queue of messages downloaded from the server.
    q []*Message
}

// Receive receives and returns the next message from the Subscription's queue,
// blocking if none are available. This method can be called concurrently from
// multiple goroutines. On systems that support acks, the Ack() method of the
// returned Message has to be called once the message has been processed, to
// prevent it from being received again.
func (s *Subscription) Receive(ctx context.Context) (*Message, error) {
    if len(s.q) == 0 {
        // Get the next batch of messages from the server.
        // ...
    }
    m := s.q[0]
    s.q = s.q[1:]
    return m, nil
}

// Close disconnects the Subscription.
func (s *Subscription) Close() error {
    close(s.doneChan)
    return s.driver.Close()
}

// NewSubscription creates a Subscription from a driver.Subscription and opts to
// tune sending and receiving of acks and messages. Behind the scenes,
// NewSubscription spins up a goroutine to gather acks into batches and
// periodically send them to the server.
func NewSubscription(s driver.Subscription) *Subscription {
    // Details similar to the body of NewTopic should go here.
}
```

Topics will gather messages into batches for sending. The batch size will be
dynamically tuned according to how many messages are being sent concurrently.

Subscriptions will gather message acks into batches the same way, also
dynamically tuning the batch size. If sending acks back to the server fails
transiently then it will be retried, most likely within a loop in the concrete
API. If an unrecoverable error occurs while sending acks then a flag will be set
on the `pubsub.Subscription` saying that the whole `Subscription` is no longer
usable. Calls to `Receive` will fail from then on.

## Alternative designs considered

### Batch oriented concrete API

In this alternative, the application code sends, receives and acknowledges
messages in batches. Here is an example of how it would look from the
developer's perspective, in a situation where not too many signups are happening
per second.

```go
package main

import (
    "context"
    "log"
    "net/http"

    rawacmepubsub "github.com/acme/pubsub"
    "github.com/google/go-cloud/pubsub"
    "github.com/google/go-cloud/pubsub/acmepubsub"
)

func main() {
    log.Fatal(serve())
}

func serve() error {
    ctx := context.Background()
    client, err := rawacmepubsub.NewClient(ctx, "unicornvideohub")
    if err != nil {
        return err
    }
    t, err := acmepubsub.OpenTopic(ctx, client, "user-signup", nil)
    if err != nil {
        return err
    }
    defer t.Close()
    http.HandleFunc("/signup", func(w http.ResponseWriter, r *http.Request) {
        err := t.Send(r.Context(), []pubsub.Message{{Body: []byte("Someone signed up")}})
        if err != nil {
            log.Println(err)
        }
    })
    return http.ListenAndServe(":8080", nil)
}
```

For a company experiencing explosive growth or enthusiastic spammers creating
more signups than this simple-minded implementation can handle, the app would
have to be adapted to create non-singleton batches, like this:

```go
package main

import (
    "context"
    "log"
    "net/http"

    rawacmepubsub "github.com/acme/pubsub"
    "github.com/google/go-cloud/pubsub"
    "github.com/google/go-cloud/pubsub/acmepubsub"
)

const batchSize = 1000

func main() {
    log.Fatal(serve())
}

func serve() error {
    ctx := context.Background()
    client, err := rawacmepubsub.NewClient(ctx, "unicornvideohub")
    if err != nil {
        return err
    }
    t, err := acmepubsub.OpenTopic(ctx, client, "user-signup", nil)
    if err != nil {
        return err
    }
    defer t.Close()
    c := make(chan *pubsub.Message)
    go sendBatches(ctx, t, c)
    http.HandleFunc("/signup", func(w http.ResponseWriter, r *http.Request) {
        c <- &pubsub.Message{Body: []byte("Someone signed up")}
    })
    return http.ListenAndServe(":8080", nil)
}

func sendBatches(ctx context.Context, t *pubsub.Topic, c chan *pubsub.Message) {
    batch := make([]*pubsub.Message, batchSize)
    for {
        for i := 0; i < batchSize; i++ {
            batch[i] = <-c
        }
        if err := t.Send(ctx, batch); err != nil {
            log.Println(err)
        }
    }
}
```

This shows how the complexity of batching has been pushed onto the application
code. Removing messages from the batch when HTTP/2 requests are canceled would
require the application code to be even more complex, adding more risk of bugs.

In this API, the application code has to either request batches of size 1,
meaning more network traffic, or it has to explicitly manage the batches of
messages it receives. Here is an example of how this API would be used for
serial message processing:

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"

    rawacmepubsub "github.com/acme/pubsub"
    "github.com/google/go-cloud/pubsub"
    "github.com/google/go-cloud/pubsub/acmepubsub"
)

const batchSize = 10

func main() {
    if err := receive(); err != nil {
        log.Fatal(err)
    }
}

func receive() error {
    ctx := context.Background()
    client, err := rawacmepubsub.NewClient(ctx, "unicornvideohub")
    if err != nil {
        return err
    }
    s, err := acmepubsub.OpenSubscription(ctx, client, "signup-minder", nil)
    if err != nil {
        return err
    }
    defer s.Close()

    // Process messages.
    for {
        msgs, err := s.Receive(ctx, batchSize)
        if err {
            return err
        }
        acks := make([]pubsub.AckID, 0, batchSize)
        for _, msg := range msgs {
            // Do something with msg.
            fmt.Printf("Got message: %q\n", msg.Body)
            acks = append(acks, msg.AckID)
        }
        err := s.SendAcks(ctx, acks)
        if err != nil {
            return err
        }
    }
}
```

Here’s what it might look like to use this batch-only API with the inverted
worker pool pattern:

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"

    rawacmepubsub "github.com/acme/pubsub"
    "github.com/google/go-cloud/pubsub"
    "github.com/google/go-cloud/pubsub/acmepubsub"
)

const batchSize = 100
const poolSize = 10

func main() {
    if err := receive(); err != nil {
        log.Fatal(err)
    }
}

func receive() error {
    ctx := context.Background()
    client, err := rawacmepubsub.NewClient(ctx, "unicornvideohub")
    if err != nil {
        return err
    }
    s, err := acmepubsub.OpenSubscription(ctx, client, "user-signup-minder", nil)
    if err != nil {
        return err
    }
    defer s.Close()

    // Receive the messages and forward them to a chan.
    msgsChan := make(chan *pubsub.Message)
    go func() {
        for {
            msgs, err := s.Receive(ctx, batchSize)
            if err {
                log.Fatal(err)
            }
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
            batch := make([]pubsub.AckID, batchSize)
            for i := 0; i < len(batch); i++ {
                batch[i] = <-acksChan
            }
            if err := s.SendAcks(ctx, batch); err != nil {
                /* handle err */
            }
        }
    }

    // Use a buffered channel as a semaphore.
    sem := make(chan struct{}, poolSize)
    for msg := range msgsChan {
        sem <- struct{}{}
        go func(msg *pubsub.Message) {
            log.Printf("Got message: %s", msg.Body)
            acksChan <- msg.AckID
            <-sem
        }(msg)
    }
    for n := poolSize; n > 0; n-- {
        sem <- struct{}{}
    }
}
```

Here are some trade-offs of this design:

Pro:

*   The semantics are simple, making it
    *   straightforward to implement the concrete API and the drivers for most
        pubsub services
    *   easy for developers to reason about how it will behave
    *   less risky that bugs will be present in the concrete API
*   Fairly efficient sending and receiving of messages is possible by tuning
    batch size and the number of goroutines sending or receiving messages.

Con:

*   This style of API makes the inverted worker pool pattern verbose.
*   Apps needing to send or receive a large volume of messages must have their
    own logic to create batches of size greater than 1.

### go-micro

Here is an example of what application code could look like for a pubsub API
inspired by [`go-micro`](https://github.com/micro/go-micro)'s `broker` package:

```go
b := somepubsub.NewBroker(...)
if err := b.Connect(); err != nil {
    /* handle err */
}
topic := "user-signups"
subID := "user-signups-subscription-1"
s, err := b.Subscription(ctx, topic, subID, func(pub broker.Publication) error {
    fmt.Printf("%s\n", pub.Message.Body)
    return nil
})
if err := b.Publish(ctx, topic, &broker.Message{ Body: []byte("alice signed up") }); err != nil {
    /* handle err */
}
// Sometime later:
if err := s.Unsubscribe(ctx); err != nil {
    /* handle err */
}
```

Pro:

*   The callback to the subscription returning an error to decide whether to
    acknowledge the message means the developer cannot forget to ack.

Con:

*   Go micro has code to auto-create
    [topics](https://github.com/micro/go-plugins/blob/f3fcfcdf77392b4e053c8d5b361abfabc0c623d3/broker/googlepubsub/googlepubsub.go#L152)
    and
    [subscriptions](https://github.com/micro/go-plugins/blob/f3fcfcdf77392b4e053c8d5b361abfabc0c623d3/broker/googlepubsub/googlepubsub.go#L185)
    as needed, but this is not consistent with the Go CDK’s design principle to
    not get involved in operations.
*   The subscription callback idea does not appear to be compatible with
    inverted worker pools.

## Acknowledgements

In pubsub systems with acknowledgement, messages are kept in a queue associated
with the subscription on the server. When a client receives one of these
messages, its counterpart on the server is marked as being processed. Once the
client finishes processing the message, it sends an acknowledgement (or "ack")
to the server and the server removes the message from the subscription queue.
There may be a deadline for the acknowledgement, past which the server unmarks
the message so that it can be received again for another try at processing.

Redis Pub/Sub and ZeroMQ don’t support acking, but many others do including GCP
PubSub, Azure Service Bus, RabbitMQ, and
[Redis Streams](https://redis.io/topics/streams-intro). Given the wide support
and usefulness, it makes sense to support message acking in the Go CDK.

As of this writing, it is an open question as to what should be done about
pubsub systems that do not support acks. Some possibilities have been discussed,
but no clear best option has emerged yet:

1.  simulating acknowledgement by constructing queues on the server. Con: the
    magically created queues would probably be a less than pleasant surprise for
    some users.
2.  making ack a no-op for systems that don't support it. With this, do we
    return a sentinel error from `Ack`, and if so then doesn't that unduly
    complicate the code for apps that never use non-acking systems? This option
    is also potentially misleading for developers who would naturally assume
    that un-acked messages would be redelivered.

### Rejected acknowledgement API: `Receive` method returns an `ack` func

In this alternative, the application code would look something like this:

```go
msg, ack, err := s.Receive(ctx)
log.Printf("Received message: %q", msg.Body)
ack(msg)
```

Pro:

*   The compiler will complain if the returned `ack` function is not used.

Con:

*   Receive has one more return value.
*   Passing `ack` around along with `msg` is inconvenient.

## Tests

### Unit tests for the concrete API (`github.com/go-cloud/pubsub`)

We can test that the batched sending, receiving and acking work as intended by
making mock implementations of the driver interfaces.

At least the following things should be tested:

*   Calling `pubsub.Message.Ack` causes `driver.Subscription.SendAcks` to be
    called.
*   Calling `pubsub.Topic.Send` causes `driver.Topic.SendBatch` to be called.
*   Calling `pubsub.Subscription.Receive` causes
    `driver.Subscription.ReceiveBatch` to be called.

### Conformance tests for specific implementations (*e.g.*, `github.com/go-cloud/pubsub/acmepubsub`)

*   Sent messages with random contents are received with the same contents.
*   Sent messages with random attributes are received with the same attributes.
*   Error occurs when making a local topic with an ID that doesn’t exist on the
    server.
*   Error occurs when making a subscription with an ID that doesn’t exist on the
    server.
*   Message gets sent again after ack deadline if a message is never
    acknowledged.
*   ~~Acked messages don't get received again after waiting twice the ack
    deadline.~~ :point_left: This test would probably be too flakey.

## Benchmarks

What is the throughput and latency of the Go CDK's `pubsub` package, relative to
directly using the APIs for various services?

*   send, for 1, 10, 100 topics, and for 1, 10, 100 goroutines sending messages
    to those topics
*   receive, for 1, 10, 100 subscriptions, and for 1, 10, 100 goroutines
    receiving from each subscription

## References

*   https://github.com/google/go-cloud/issues/312
*   http://queues.io/
