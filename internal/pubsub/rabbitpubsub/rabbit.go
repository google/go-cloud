package rabbitpubsub

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/go-cloud/internal/pubsub"
	"github.com/google/go-cloud/internal/pubsub/driver"
	"github.com/streadway/amqp"
)

type topic struct {
	exchange string // the AMQP exchange
	conn     *amqp.Connection

	mu   sync.Mutex
	ch   *amqp.Channel            // AMQP channel used for all communication.
	pubc <-chan amqp.Confirmation // Go channel for server acks of publishes
}

// Values for the amqp client.
// See https://www.rabbitmq.com/amqp-0-9-1-reference.html.
const (
	// Many methods of the amqp client take a "no-wait" parameter, which
	// if true causes the client to return without waiting for a server
	// response. We always want to wait.
	wait = false

	// Always use the empty routing key. This driver expects to be used with topic
	// exchanges, which disregard the routing key.
	routingKey = ""

	// If the message can't be enqueued, return it to the sender rather than silently dropping it.
	mandatory = true

	// If there are no waiting consumers, enqueue the message instead of dropping it
	immediate = false
)

// OpenTopic returns a *pubsub.Topic corresponding to the named exchange. The
// exchange must have been previously created (for instance, by using
// amqp.Channel.ExchangeDeclare). For the model of Go Cloud Pub/Sub to make sense,
// the exchange should be a fanout exchange, although nothing in this package
// enforces that.
//
// OpenTopic uses the supplied amqp.Connection for all communication. It is the caller's
// responsibility to establish this connection before calling OpenTopic, and to close
// it when Close has been called on all Topics opened with it.
//
// The documentation of the amqp package recommends using separate connections for
// publishing and subscribing.
func OpenTopic(conn *amqp.Connection, name string) (*pubsub.Topic, error) {
	// TODO(jba): support context.Context
	t, err := newTopic(conn, name)
	if err != nil {
		return nil, err
	}
	return pubsub.NewTopic(t), nil

}

func newTopic(conn *amqp.Connection, name string) (*topic, error) {
	t := &topic{
		exchange: name,
		conn:     conn,
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	if err := t.establishChannel(); err != nil {
		return nil, err
	}
	return t, nil
}

// establishChannel creates an AMQP channel if necessary. According to the amqp
// package docs, once an error is returned from the channel, it must be discarded and
// a new one created.
//
// Must be called with the t.mu held.
func (t *topic) establishChannel() error {
	// TODO(jba): support context.Context
	if t.ch != nil {
		// We already have a channel; nothing to do.
		return nil
	}
	// Create a new channel.
	ch, err := t.conn.Channel()
	if err != nil {
		return err
	}
	// Put the channel into a mode where confirmations are delivered for each publish.
	if err := ch.Confirm(wait); err != nil {
		return err
	}
	t.ch = ch
	// Get a Go channel which will hold acks from the server. The server
	// will send an ack for each published message to confirm that it was received.
	t.pubc = ch.NotifyPublish(make(chan amqp.Confirmation)) // NotifyPublish returns its arg
	return nil
}

// SendBatch implements driver.SendBatch.
func (t *topic) SendBatch(ctx context.Context, ms []*driver.Message) error {
	// It is simplest to allow only one SendBatch at a time. Allowing concurrent
	// calls to SendBatch would complicate the logic of receiving publish
	// confirmations and returns. We can go that route if performance warrants it.
	t.mu.Lock()
	defer t.mu.Unlock()

	if err := t.establishChannel(); err != nil {
		return err
	}

	// Read from the channel established with NotifyPublish.
	// Do so concurrently or we will deadlock with the Publish RPC.
	// (The amqp package docs recommend setting the capacity of the channel
	// to the number of messages to be published, but we can't do that because
	// we want to reuse the channel for all calls to SendBatch.)
	errc := make(chan error, 1)
	go func() {
		// Consume all the acknowledgments for the messages we are publishing.
		// Since this method holds the lock, we expect exactly as many acks as messages.
		//
		// This goroutine can safely access the mutex-protected fields t.ch and t.pubc
		// because its lifetime is within the lifetime of the lock held by SendBatch.
		//
		// TODO(jba): look at AMQP "returns" for more information about publish errors.
		// See https://godoc.org/github.com/streadway/amqp#Channel.NotifyReturn.
		ok := true
		for range ms {
			select {
			case <-ctx.Done():
				errc <- ctx.Err()
				return
			case conf, ok := <-t.pubc:
				if !ok {
					// t.pubc was closed
					errc <- errors.New("rabbitpubsub: publish listener closed unexpectedly")
					t.ch = nil // re-create the channel on next use
					return
				}
				if !conf.Ack {
					ok = false
				}
			}
		}
		if !ok {
			errc <- errors.New("rabbitpubsub: ack failed on publish")
		} else {
			errc <- nil
		}
	}()

	for _, m := range ms {
		pub := toPublishing(m)
		if err := t.ch.Publish(t.exchange, routingKey, mandatory, immediate, pub); err != nil {
			t.ch = nil // AMQP channel is broken after error
			return err
		}
	}
	return <-errc
}

// toPublishing converts a driver.Message to an amqp.Publishing.
func toPublishing(m *driver.Message) amqp.Publishing {
	h := amqp.Table{}
	for k, v := range m.Metadata {
		h[k] = v
	}
	return amqp.Publishing{
		Headers: h,
		Body:    m.Body,
	}
}

// IsRetryable implements driver.Topic.IsRetryable.
func (*topic) IsRetryable(error) bool {
	// TODO(jba): figure out what errors can be retried.
	return false
}

// OpenSubscription returns a *pubsub.Subscription corresponding to the named queue. The
// queue must have been previously created (for instance, by using
// amqp.Channel.QueueDeclare) and bound to an exchange.
//
// OpenSubscription uses the supplied amqp.Connection for all communication. It is
// the caller's responsibility to establish this connection before calling OpenSubscription
// and to close it when Close has been called on all Subscriptions opened with it.
//
// The documentation of the amqp package recommends using separate connections for
// publishing and subscribing.
func OpenSubscription(conn *amqp.Connection, name string) (*pubsub.Subscription, error) {
	// TODO(jba): support context.Context
	s, err := newSubscription(conn, name)
	if err != nil {
		return nil, err
	}
	return pubsub.NewSubscription(s), nil
}

type subscription struct {
	conn     *amqp.Connection
	queue    string // the AMQP queue name
	consumer string // the client-generated name for this particular subscriber

	mu   sync.Mutex
	ch   *amqp.Channel // AMQP channel used for all communication.
	delc <-chan amqp.Delivery
}

var nextConsumer int64 // atomic

func newSubscription(conn *amqp.Connection, name string) (*subscription, error) {
	s := &subscription{
		conn:     conn,
		queue:    name,
		consumer: fmt.Sprintf("c%d", atomic.AddInt64(&nextConsumer, 1)),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.establishChannel(); err != nil {
		return nil, err
	}
	return s, nil
}

// Must be called with s.mu held.
func (s *subscription) establishChannel() error {
	// TODO(jba): support context.Context
	if s.ch != nil {
		// We already have a channel; nothing to do.
		return nil
	}
	// Create a new channel.
	ch, err := s.conn.Channel()
	if err != nil {
		return err
	}
	// Subscribe to messages from the queue.
	s.delc, err = ch.Consume(s.queue, s.consumer,
		false, // autoAck
		false, // exclusive
		false, // noLocal
		wait,
		nil) // args
	if err != nil {
		return err
	}
	s.ch = ch
	return nil
}

// ReceiveBatch implements driver.Subscription.ReceiveBatch.
func (s *subscription) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.establishChannel(); err != nil {
		return nil, err
	}

	// Get up to maxMessages waiting messages, but don't take too long.
	var ms []*driver.Message
	maxTime := time.After(50 * time.Millisecond)
	for {
		select {
		case <-ctx.Done():
			// Cancel the Consume.
			_ = s.ch.Cancel(s.consumer, wait) // ignore the error
			s.ch = nil
			return nil, ctx.Err()

		case d, ok := <-s.delc:
			if !ok { // channel closed
				s.ch = nil // re-establish the channel next time
				if len(ms) > 0 {
					return ms, nil
				}
				// TODO(jba): get more information for a better message
				return nil, errors.New("delivery channel closed")
			}
			ms = append(ms, toMessage(d))
			if len(ms) >= maxMessages {
				return ms, nil
			}

		case <-maxTime:
			// Timed out. Return whatever we have.
			return ms, nil
		}
	}
}

// toMessage converts an amqp.Delivery (a received message) to a driver.Message.
func toMessage(d amqp.Delivery) *driver.Message {
	// Delivery.Headers is a map[string]interface{}, so we have to
	// convert each value to a string.
	md := map[string]string{}
	for k, v := range d.Headers {
		md[k] = fmt.Sprint(v)
	}
	return &driver.Message{
		Body:     d.Body,
		AckID:    d.DeliveryTag,
		Metadata: md,
	}
}

// SendAcks implements driver.Subscription.SendAcks.
func (s *subscription) SendAcks(ctx context.Context, ackIDs []driver.AckID) error {
	// TODO(#853): consider a separate channel for acks, so ReceiveBatch and SendAcks don't
	// block each other.
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.establishChannel(); err != nil {
		return err
	}

	// The Ack call doesn't wait for a response, so this loop should execute relatively
	// quickly.
	// It wouldn't help to make it concurrent, because Channel.Ack grabs a channel-wide mutex.
	// (We could consider using multiple channels if performance becomes an issue.)
	for _, id := range ackIDs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := s.ch.Ack(id.(uint64), false) // multiple=false: acking only this ID
		if err != nil {
			s.ch = nil // re-establish channel after an error
			return err
		}
	}
	return nil
}

// IsRetryable implements driver.Subscription.IsRetryable.
func (*subscription) IsRetryable(error) bool {
	// TODO(jba): figure out what errors can be retried.
	return false
}
