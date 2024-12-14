// Copyright 2018 The Go Cloud Development Kit Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rabbitpubsub

// This file implements a fake for the parts of the AMQP protocol used by our RabbitMQ
// implementation.

import (
	"context"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// fakeConnection implements the amqpConnection interface.
// It also doubles as the state of the fake server.
type fakeConnection struct {
	mu        sync.Mutex
	closed    chan struct{}
	exchanges map[string]*exchange // exchange names are server-scoped
	queues    map[string]*queue    // queue names are server-scoped
}

// fakeChannel implements the amqpChannel interface.
type fakeChannel struct {
	conn *fakeConnection
	// The following fields are protected by conn.mu.
	deliveryTag     uint64 // counter; used to distinguish published messages
	pubChans        []chan<- amqp.Confirmation
	returnChans     []chan<- amqp.Return
	closeChans      []chan<- *amqp.Error
	consumerCancels map[string]func() // from consumer name to cancel func for the context

	closeMu sync.Mutex
	closed  chan struct{}
}

// An exchange is a collection of queues.
// Every queue is also in the fakeConnection.queues map, so they can be looked up
// by name. An exchange needs a list of its own queues (the ones bound to it) so
// it can deliver incoming messages to them.
type exchange struct {
	queues []*queue
}

// A queue holds a set of messages to be delivered.
type queue struct {
	messages   []amqp.Delivery
	pendingAck map[uint64]amqp.Delivery
}

func newFakeConnection() *fakeConnection {
	return &fakeConnection{
		exchanges: map[string]*exchange{},
		queues:    map[string]*queue{},
		closed:    make(chan struct{}),
	}
}

// Channel creates a new AMQP fake channel.
func (c *fakeConnection) Channel() (amqpChannel, error) {
	if chanIsClosed(c.closed) {
		return nil, amqp.ErrClosed
	}
	return &fakeChannel{
		conn:            c,
		consumerCancels: map[string]func(){},
		closed:          make(chan struct{}),
	}, nil
}

func (c *fakeConnection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	closeChan(c.closed)
	return nil
}

// getExchange returns the named exchange, or error if it doesn't exist.
// It closes the channel on error.
// It must be called with the lock held.
func (ch *fakeChannel) getExchange(name string) (*exchange, error) {
	if ex := ch.conn.exchanges[name]; ex != nil {
		return ex, nil
	}
	return nil, ch.errorf(amqp.NotFound, "exchange %q not found", name)
}

// errorf returns an amqp.Error and closes the channel. (In the AMQP protocol, any channel error
// closes the channel and makes it unusable.)
// It must be called with ch.conn.mu held.
func (ch *fakeChannel) errorf(code int, reasonFormat string, args ...any) error {
	_ = ch.Close()
	return &amqp.Error{Code: code, Reason: fmt.Sprintf(reasonFormat, args...)}
}

// Report whether the channel or its connection is closed. Does not require the lock.
func (ch *fakeChannel) isClosed() bool {
	return chanIsClosed(ch.closed) || chanIsClosed(ch.conn.closed)
}

// ExchangeDeclare creates a new exchange with the given name if one doesn't already
// exist.
func (ch *fakeChannel) ExchangeDeclare(name string) error {
	if ch.isClosed() {
		return amqp.ErrClosed
	}

	ch.conn.mu.Lock()
	defer ch.conn.mu.Unlock()

	if _, ok := ch.conn.exchanges[name]; !ok {
		ch.conn.exchanges[name] = &exchange{}
	}
	return nil
}

// QueueDeclareAndBind binds a queue to the given exchange.
// The exchange must exist.
// If the queue doesn't exist, it's created.
func (ch *fakeChannel) QueueDeclareAndBind(queueName, exchangeName string) error {
	if ch.isClosed() {
		return amqp.ErrClosed
	}
	ch.conn.mu.Lock()
	defer ch.conn.mu.Unlock()

	ex, err := ch.getExchange(exchangeName)
	if err != nil {
		return err
	}
	if _, ok := ch.conn.queues[queueName]; ok {
		return nil
	}
	q := &queue{pendingAck: map[uint64]amqp.Delivery{}}
	ch.conn.queues[queueName] = q
	ex.queues = append(ex.queues, q)
	return nil
}

func (ch *fakeChannel) Publish(exchangeName, routingKey string, pub amqp.Publishing) error {
	return ch.PublishWithContext(context.Background(), exchangeName, routingKey, pub)
}

func (ch *fakeChannel) PublishWithContext(ctx context.Context,
	exchangeName, routingKey string,
	pub amqp.Publishing,
) error {
	if ch.isClosed() {
		return amqp.ErrClosed
	}

	ch.conn.mu.Lock()
	defer ch.conn.mu.Unlock()

	ex, err := ch.getExchange(exchangeName)
	if err != nil {
		return err
	}
	if len(ex.queues) == 0 {
		// The message is unroutable. Send a Return to all channels registered with
		// NotifyReturn.
		ret := amqp.Return{
			Exchange:   exchangeName,
			ReplyCode:  amqp.NoRoute,
			ReplyText:  "NO_ROUTE: no queues bound to exchange",
			RoutingKey: routingKey,
		}
		for _, c := range ch.returnChans {
			select {
			case c <- ret:
			case <-ch.closed:
				return amqp.ErrClosed
			case <-ch.conn.closed:
				return amqp.ErrClosed
			}
		}
	} else {
		// Each published message in the channel gets a new delivery tag, starting at 1.
		ch.deliveryTag++
		// Convert the Publishing into a Delivery.
		del := amqp.Delivery{
			Headers:     pub.Headers,
			Body:        pub.Body,
			DeliveryTag: ch.deliveryTag,
			// We don't care about the other fields.
		}
		// All exchanges are "fanout" exchanges, so the message is sent to all queues.
		for _, q := range ex.queues {
			q.messages = append(q.messages, del)
		}
	}
	// Every Go channel registered with NotifyPublish gets a confirmation message.
	// Ack is true even if the message was unroutable.
	for _, c := range ch.pubChans {
		select {
		case c <- amqp.Confirmation{DeliveryTag: ch.deliveryTag, Ack: true}:
		case <-ch.closed:
			return amqp.ErrClosed
		case <-ch.conn.closed:
			return amqp.ErrClosed
		}
	}
	return nil
}

// Consume starts a consumer that reads from the given queue.
// The consumerName can be used in a Cancel call to stop the consumer.
func (ch *fakeChannel) Consume(queueName, consumerName string) (<-chan amqp.Delivery, error) {
	if ch.isClosed() {
		return nil, amqp.ErrClosed
	}
	ch.conn.mu.Lock()
	defer ch.conn.mu.Unlock()

	q, ok := ch.conn.queues[queueName]
	if !ok {
		return nil, ch.errorf(amqp.NotFound, "queue %q not found", queueName)
	}
	if _, ok := ch.consumerCancels[consumerName]; ok {
		return nil, ch.errorf(amqp.PreconditionFailed, "consumer %q already exists", consumerName)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch.consumerCancels[consumerName] = cancel // used by fakeChannel.Cancel
	delc := make(chan amqp.Delivery)
	go func() {
		// For this simple fake, just deliver one message every once in a while if
		// any are available, until the consumer is canceled.
		for {
			m, ok := ch.takeOneMessage(q)
			if ok {
				select {
				case delc <- m:
				case <-ctx.Done():
					// ignore errors here and below
					return
				case <-ch.closed:
					return
				case <-ch.conn.closed:
					return
				}
			}
			select {
			case <-time.After(10 * time.Millisecond):
			case <-ctx.Done():
				// ignore error
				return
			}
		}
	}()
	return delc, nil
}

// Take a message from q, if one is available. We just remove
// the message from the queue permanently. In a more sophisticated implementation
// we'd mark it as outstanding and keep it around until it got acked, but we don't
// need acks for this fake.
func (ch *fakeChannel) takeOneMessage(q *queue) (amqp.Delivery, bool) {
	ch.conn.mu.Lock()
	defer ch.conn.mu.Unlock()
	if len(q.messages) == 0 {
		return amqp.Delivery{}, false
	}
	m := q.messages[0]
	q.messages = q.messages[1:]
	q.pendingAck[m.DeliveryTag] = m
	return m, true
}

// Ack removes the message from q.pendingAck.
func (ch *fakeChannel) Ack(tag uint64) error {
	if ch.isClosed() {
		return amqp.ErrClosed
	}
	ch.conn.mu.Lock()
	defer ch.conn.mu.Unlock()
	for _, q := range ch.conn.queues {
		if _, ok := q.pendingAck[tag]; ok {
			delete(q.pendingAck, tag)
			return nil
		}
	}
	// No error if we couldn't find it to ack it.
	return nil
}

// Nack moves the message from q.pendingAck back to q.messages
// to be redelivered.
func (ch *fakeChannel) Nack(tag uint64) error {
	if ch.isClosed() {
		return amqp.ErrClosed
	}
	ch.conn.mu.Lock()
	defer ch.conn.mu.Unlock()
	for _, q := range ch.conn.queues {
		if m, ok := q.pendingAck[tag]; ok {
			delete(q.pendingAck, tag)
			q.messages = append(q.messages, m)
			return nil
		}
	}
	// No error if we couldn't find it to nack it.
	return nil
}

// Cancel stops the consumer's goroutine.
func (ch *fakeChannel) Cancel(consumerName string) error {
	if ch.isClosed() {
		return amqp.ErrClosed
	}
	ch.conn.mu.Lock()
	defer ch.conn.mu.Unlock()

	cancel, ok := ch.consumerCancels[consumerName]
	if !ok {
		return ch.errorf(amqp.NotFound, "consumer %q not found", consumerName)
	}
	cancel()
	delete(ch.consumerCancels, consumerName)
	return nil
}

// NotifyPublish remembers its argument channel so it can be notified for every
// published message. It returns its argument.
func (ch *fakeChannel) NotifyPublish(c chan amqp.Confirmation) chan amqp.Confirmation {
	ch.conn.mu.Lock()
	defer ch.conn.mu.Unlock()
	ch.pubChans = append(ch.pubChans, c)
	return c
}

// NotifyReturn remembers its argument channel so it can be notified for every
// published message that's returned due to being unroutable.
func (ch *fakeChannel) NotifyReturn(c chan amqp.Return) chan amqp.Return {
	ch.conn.mu.Lock()
	defer ch.conn.mu.Unlock()
	ch.returnChans = append(ch.returnChans, c)
	return c
}

// NotifyClose remembers its argument channel so it can be notified when
// the channel is closed.
func (ch *fakeChannel) NotifyClose(c chan *amqp.Error) chan *amqp.Error {
	ch.conn.mu.Lock()
	defer ch.conn.mu.Unlock()
	ch.closeChans = append(ch.closeChans, c)
	return c
}

// Close marks the fakeChannel as closed and sends an error to all channels
// registered with NotifyClose.
func (ch *fakeChannel) Close() error {
	if chanIsClosed(ch.conn.closed) {
		return amqp.ErrClosed
	}
	ch.closeMu.Lock()
	closeChans := ch.closeChans
	closeChan(ch.closed)
	ch.closeMu.Unlock()
	for _, c := range closeChans {
		// Don't block on notifying.
		select {
		case c <- amqp.ErrClosed:
		default:
		}
	}
	return nil
}

func (ch *fakeChannel) ExchangeDelete(name string) error {
	ch.conn.mu.Lock()
	defer ch.conn.mu.Unlock()

	delete(ch.conn.exchanges, name)
	return nil
}

func (ch *fakeChannel) QueueDelete(name string) error {
	ch.conn.mu.Lock()
	defer ch.conn.mu.Unlock()

	delete(ch.conn.queues, name)
	return nil
}

func (ch *fakeChannel) Qos(_, _ int, _ bool) error {
	if ch.isClosed() {
		return amqp.ErrClosed
	}

	return nil
}

// Assumes nothing is ever written to the channel.
func chanIsClosed(ch chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// Avoid panic when closing a closed channel.
// Must be called with the lock held.
func closeChan(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}
