# Go Cloud PubSub

## Summary
This document proposes a new `pubsub` package for Go Cloud.

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
when building systems in terms of the cloud-specific services such as AWS
SNS+SQS, GCP PubSub or Azure Service Bus.

Developers may wish to compare different pubsub systems in terms of their
performance, reliability, cost or other factors, and they may want the option
to move between these systems without too much friction. A `pubsub` package in
Go Cloud could lower the cost of such experiments and migrations.

## Goals
* Publish messages to an existing topic.
* Receive messages from an existing subscription.
* Peform not much worse than 90% compared to directly using the APIs of various pubsub systems.
* Work well with managed pubsub services on AWS, Azure, GCP and the most used open source pubsub systems.

## Non-goals
* Create new topics. Go Cloud focuses on developer concerns, but topic creation is an [operator concern](https://github.com/google/go-cloud/blob/master/internal/docs/design.md#developers-and-operators)

* Create new subscriptions. For the first version of the Go Cloud pubsub package, we will assume that the set of subscribers for a particular topic doesn’t change during the normal operation of the application's system. The subscribers are assumed to correspond to components of a distributed system rather than to users of that system.

## Background
[Pubsub](https://en.wikipedia.org/wiki/Publish%E2%80%93subscribe_pattern) is
a frequently requested feature for the Go Cloud project [github
issue](https://github.com/google/go-cloud/issues/312). A key use case
motivating these requests is to support [event driven
architectures](https://en.wikipedia.org/wiki/Event-driven_architecture).

There are several pubsub systems available that could be made to work with Go Cloud by writing drivers for them. Here is a [table](https://docs.google.com/a/google.com/spreadsheets/d/e/2PACX-1vQ2CML8muCrqhinxOeKTcWtwAeGk-RFFFMjB3O2u5DbbBt9R3YnUQcgRjRp6TySXe1CzSOtPVCsKACY/pubhtml) comparing some of them.

## Design overview
### Developer’s perspective
Given a topic that has already been created on the pubsub server, messages can be sent to that topic by creating a new `pubsub.Publisher` and calling its `Send` method, like this (assuming a fictional pubsub provider called "acme"):

```go
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/google/go-cloud/pubsub"
	"github.com/google/go-cloud/pubsub/acme/publisher"
)

func main() {
	ctx := context.Background()
	topic := "projects/unicornvideohub/topics/user-signup"
	pub, err := publisher.New(ctx, topic)
	if err != nil {
		log.Fatal(err)
	}
	defer pub.Close()
	http.HandleFunc("/signup", func(w http.ResponseWriter, r *http.Request) {
		err := pub.Send(ctx, pubsub.Message{Body: []byte("Someone signed up")})
		if err != nil {
			log.Println(err)
		}
	})
	log.Fatal(http.ListenAndServer(":8080", nil))
}
```

The call to `Send` will only return after the message has been sent to the
server or its sending has failed.

Messages can be received from an existing subscription to a topic by
creating a `pubsub.Subscriber` and calling its `Receive` method, like this:

```go
package main

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
package main

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

1. Write the driver, implementing the interfaces in the `github.com/go-cloud/pubsub/driver` package. The new driver could be located at `github.com/go-cloud/pubsub/${newsystem}/driver`.
2. Write the concrete implementation, including `publisher.New` to construct a `pubsub.Publisher` and `subscriber.New` to construct a `pubsub.Subscriber`. These constructors should be located at `github.com/go-cloud/pubsub/$newsystem/publisher` and `github.com/go-cloud/pubsub/$newsystem/subscriber`.

The driver interfaces are batch-oriented because some pubsub systems can more efficiently deal with batches of messages than with one at a time. Streaming was considered but it does not appear to provide enough of a performance gain to be worth the additional complexity of supporting it across different pubsub systems.

The driver interfaces will be located in the `github.com/google/go-cloud/pubsub/driver` package and will look something like this:

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

The concrete API will be located at `github.com/google/go-cloud/pubsub` and will look something like this:
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

// PublisherOptions contains configuration for Publishers.
type PublisherOptions struct {
    // SendWait tells the max duration to wait before sending the next batch of
    // messages to the server.
    SendWait time.Duration
}

// Publisher publishes messages to all its subscribers.
type Publisher struct {
    p       driver.Publisher
    opts    PublisherOptions
}

// Send publishes a message. It only returns after the message has been
// sent, or failed to be sent. Send can be called from multiple goroutines
// at once.
func (p *Publisher) Send(ctx context.Context, m *Message) error { … }

// Close disconnects the Publisher.
func (p *Publisher) Close() error

// SubscriberOptions contains configuration for Subscribers.
type SubscriberOptions struct {
    // AckWait tells the max duration to wait before sending the next batch 
    // of acknowledgements back to the server.
    AckWait     time.Duration

    // AckDeadline tells how long the server should wait before assuming a
    // received message has failed to be processed.
    AckDeadline time.Duration
}

// Subscriber receives published messages.
type Subscriber struct {
    s       driver.Subscriber
    opts    SubscriberOptions
}

// Receive receives and returns the next message from the Subscriber's queue,
// blocking if none are available. This method can be called concurrently from
// multiple goroutines. On systems that support acks, the Ack() method of the
// returned Message has to be called once the message has been processed, to
// prevent it from being received again.
func (s *Subscriber) Receive(ctx context.Context) (*Message, error) { … }

// Close disconnects the Subscriber.
func (s *Subscriber) Close() error { … }
```

## Alternative designs considered

### Batch oriented concrete API
In this alternative, the application code sends, receives and acknowledges messages in batches.
Here is an example of how it would look from the developer's perspective, in a situation where
not more than about a thousand users are signing up per second:
```go
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/google/go-cloud/pubsub"
	"github.com/google/go-cloud/pubsub/acme/publisher"
)

func main() {
	ctx := context.Background()
	topic := "projects/unicornvideohub/topics/user-signup"
	pub, err := publisher.New(ctx, topic)
	if err != nil {
		log.Fatal(err)
	}
	defer pub.Close()
	http.HandleFunc("/signup", func(w http.ResponseWriter, r *http.Request) {
		err := pub.Send(ctx, []pubsub.Message{{Body: []byte("Someone signed up")}})
		if err != nil {
			log.Println(err)
		}
	})
	log.Fatal(http.ListenAndServer(":8080", nil))
}
```
For a company experiencing explosive growth or enthusiastic spammers creating
many thousands of signups per second, the app would have to be adapted to
create non-singleton batches, like this:
```go
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/google/go-cloud/pubsub"
	"github.com/google/go-cloud/pubsub/acme/publisher"
)

const batchSize = 1000

func main() {
	ctx := context.Background()
	topic := "projects/unicornvideohub/topics/user-signup"
	pub, err := publisher.New(ctx, topic)
	if err != nil {
		log.Fatal(err)
	}
    defer pub.Close()
    c := make(chan *pubsub.Message)
    go sendBatches(ctx, pub, c)
	http.HandleFunc("/signup", func(w http.ResponseWriter, r *http.Request) {
		err := pub.Send(ctx, []*pubsub.Message{{Body: []byte("Someone signed up")}})
		if err != nil {
			log.Println(err)
		}
	})
	log.Fatal(http.ListenAndServer(":8080", nil))
}

func sendBatches(ctx context.Context, pub *pubsub.Publisher, c chan *pubsub.Message) {
    batch := make([]*pubsub.Message, batchSize)
    for {
        for i := 0; i < batchSize; i++ {
            batch[i] = <-c
        }
        if err := pub.Send(ctx, batch); err != nil {
            log.Println(err)
        }
    }
}
```
This shows how the complexity of batching has been pushed onto the application code.

In this API, the application code has to either request batches of size 1, meaning more
network traffic, or it has to explicitly manage the batches of messages it receives.
Here is an example of how it would be used for serial message processing:
```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"

    "github.com/google/go-cloud/pubsub" 
    "github.com/google/go-cloud/pubsub/acme/subscriber" 
)

const batchSize = 10

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
        msgs, err := sub.Receive(ctx, batchSize)
        switch err {
        // sub.Close() causes io.EOF to be returned from sub.Receive().
        case io.EOF:
            log.Printf("Got ctrl-C. Exiting.")
            break
        case nil:
        default:
            return err
        }
        acks := make([]pubsub.AckID, 0, batchSize)
        for _, msg := range msgs {
            // Do something with msg.
            fmt.Printf("Got message: %q\n", msg.Body)
            acks = append(acks, msg.AckID)
        }
        err := sub.SendAcks(ctx, acks)
        if err != nil { return err }
    }
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
* This style of API makes the inverted worker pool pattern verbose.
* Apps needing to send or receive a large volume of messages must have their own logic to create batches of size greater than 1.

### go-micro
Here is an example of what application code could look like for a pubsub API inspired by [`go-micro`](https://github.com/micro/go-micro)'s `broker` package: 
```go
b := somepubsub.NewBroker(...)
err := b.Connect()
if err != nil { /* handle err */ }
topic := "user-signups"
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
* The callback to the subscription returning an error to decide whether to acknowledge the message means the developer cannot forget to ack.

Con:
* Go micro has code to auto-create [topics](https://github.com/micro/go-plugins/blob/f3fcfcdf77392b4e053c8d5b361abfabc0c623d3/broker/googlepubsub/googlepubsub.go#L152) and [subscriptions](https://github.com/micro/go-plugins/blob/f3fcfcdf77392b4e053c8d5b361abfabc0c623d3/broker/googlepubsub/googlepubsub.go#L185) as needed, but this is not consistent with Go Cloud’s design principle to not get involved in operations.

## Acknowledgements
In pubsub systems with acknowledgement, messages are kept in a queue associated with the subscription on the server. When a client receives one of these messages, its counterpart on the server is marked as being processed. Once the client finishes processing the message, it sends an acknowledgement (or "ack") to the server and the server removes the message from the subscription queue. There may be a deadline for the acknowledgement, past which the server unmarks the message so that it can be received again for another try at processing.

Redis and ZeroMQ don’t support Ack, but many others do including GCP PubSub, Azure Service Bus, and RabbitMQ. Given the wide support and usefulness, it makes sense to support this in Go Cloud. For systems that don't have acknowledgement, such as Redis, it is probably best for the associated Go Cloud driver to simulate it so that users of Go Cloud pubsub never have to worry about whether acknowledgement is supported.

### Rejected alternative: `Receive` method returns an `ack` func
In this alternative, the application code would look something like this:
```go
msg, ack, err := sub.Receive(ctx)
// Do something with msg...
if /* something went wrong */ {
        // Don't acknowledge the message. Let it time out and be
        // redelivered.
        continue
}
ack(msg)
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
What is the throughput and latency of Go Cloud's `pubsub` package, relative to directly using the APIs for various providers?
* send, for 1, 10, 100 publishers, and for 1, 10, 100 goroutines sending messages via those publishers
* receive, for 1, 10, 100 subscribers, and for 1, 10, 100 goroutines receiving from each subscriber

## References
* https://github.com/google/go-cloud/issues/312
* http://queues.io/
