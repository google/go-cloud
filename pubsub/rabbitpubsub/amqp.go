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

// Interfaces for the AMQP protocol, and adapters for the real amqp client.
// Fake implementations of the interfaces are in fake_test.go

import "github.com/streadway/amqp"

// Values we use for the amqp client.
// See https://www.rabbitmq.com/amqp-0-9-1-reference.html.
const (
	// Many methods of the amqp client take a "no-wait" parameter, which
	// if true causes the client to return without waiting for a server
	// response. We always want to wait.
	wait = false

	// Always use the empty routing key. This driver expects to be used with topic
	// exchanges, which disregard the routing key.
	routingKey = ""

	// If the message can't be enqueued, return it to the sender rather than silently
	// dropping it.
	mandatory = true

	// If there are no waiting consumers, enqueue the message instead of dropping it.
	immediate = false
)

// See https://godoc.org/github.com/streadway/amqp#Connection for the documentation of these methods.
type amqpConnection interface {
	Channel() (amqpChannel, error)
	Close() error
}

// See https://godoc.org/github.com/streadway/amqp#Channel for the documentation of these methods.
type amqpChannel interface {
	Publish(exchange string, msg amqp.Publishing) error
	Consume(queue, consumer string) (<-chan amqp.Delivery, error)
	Ack(tag uint64) error
	Nack(tag uint64) error
	Cancel(consumer string) error
	Close() error
	NotifyPublish(chan amqp.Confirmation) chan amqp.Confirmation
	NotifyReturn(chan amqp.Return) chan amqp.Return
	NotifyClose(chan *amqp.Error) chan *amqp.Error
	ExchangeDeclare(string) error
	QueueDeclareAndBind(qname, ename string) error
	ExchangeDelete(string) error
	QueueDelete(qname string) error
}

// connection adapts an *amqp.Connection to the amqpConnection interface.
type connection struct {
	conn *amqp.Connection
}

// Channel creates a new channel. We always want the channel in confirm mode (where
// confirmations are delivered for each publish), so we do that here as well.
func (c *connection) Channel() (amqpChannel, error) {
	ch, err := c.conn.Channel()
	if err != nil {
		return nil, err
	}
	if err := ch.Confirm(wait); err != nil {
		return nil, err
	}
	return &channel{ch}, nil
}

func (c *connection) Close() error {
	return c.conn.Close()
}

// channel adapts an *amqp.Channel to the amqpChannel interface.
type channel struct {
	ch *amqp.Channel
}

func (ch *channel) Publish(exchange string, msg amqp.Publishing) error {
	return ch.ch.Publish(exchange, routingKey, mandatory, immediate, msg)
}

func (ch *channel) Consume(queue, consumer string) (<-chan amqp.Delivery, error) {
	return ch.ch.Consume(queue, consumer,
		false, // autoAck
		false, // exclusive
		false, // noLocal
		wait,
		nil) // args
}

func (ch *channel) Ack(tag uint64) error {
	return ch.ch.Ack(tag, false) // multiple=false: acking only this ID
}

func (ch *channel) Nack(tag uint64) error {
	return ch.ch.Nack(tag, false, true) // multiple=false: acking only this ID, requeue: true to redeliver
}

func (ch *channel) Confirm() error {
	return ch.ch.Confirm(wait)
}

func (ch *channel) Cancel(consumer string) error {
	return ch.ch.Cancel(consumer, wait)
}

func (ch *channel) Close() error {
	return ch.ch.Close()
}

func (ch *channel) NotifyPublish(c chan amqp.Confirmation) chan amqp.Confirmation {
	return ch.ch.NotifyPublish(c)
}

func (ch *channel) NotifyReturn(c chan amqp.Return) chan amqp.Return {
	return ch.ch.NotifyReturn(c)
}

func (ch *channel) NotifyClose(c chan *amqp.Error) chan *amqp.Error {
	return ch.ch.NotifyClose(c)
}

func (ch *channel) ExchangeDeclare(name string) error {
	return ch.ch.ExchangeDeclare(name,
		"fanout", // kind
		false,    // durable
		false,    // delete when unused
		false,    // internal
		wait,
		nil) // args
}

// QueueDeclareAndBind declares a queue and binds it to an exchange.
func (ch *channel) QueueDeclareAndBind(queueName, exchangeName string) error {
	q, err := ch.ch.QueueDeclare(queueName,
		false, // durable
		false, // delete when unused
		false, // exclusive
		wait,
		nil) // args
	if err != nil {
		return err
	}
	return ch.ch.QueueBind(q.Name, q.Name, exchangeName, wait, nil)
}

func (ch *channel) ExchangeDelete(name string) error {
	return ch.ch.ExchangeDelete(name, false, false)
}

func (ch *channel) QueueDelete(qname string) error {
	_, err := ch.ch.QueueDelete(qname, false, false, false)
	return err
}
