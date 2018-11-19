// Copyright 2018 The Go Cloud Authors
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

// Package rabbitpubsub provides a pubsub driver for RabbitMQ.
package rabbitpubsub

import (
	"context"
	"errors"

	"github.com/google/go-cloud/internal/pubsub"
	"github.com/google/go-cloud/internal/pubsub/driver"
	"github.com/streadway/amqp"
)

type Client struct {
	conn *amqp.Connection
}

func NewClient(ctx context.Context, url string) (*Client, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn}, nil
}

func (c *Client) Close() error {
	// for _, ch := range c.chans {
	// 	ch.Close()
	// }
	return c.conn.Close()
}

type topic struct {
	exchange string
	c        *Client
}

func OpenTopic(c *Client, name string) (*pubsub.Topic, error) {
	t := &topic{exchange: name, c: c}
	return pubsub.NewTopic(t), nil
}

// Values we use for AMQP publishing.
// See https://www.rabbitmq.com/amqp-0-9-1-reference.html.
const (
	// If the message can't be enqueued, return it to the sender rather than silently dropping it.
	mandatory = true

	// If there are no waiting consumers, enqueue the message instead of dropping it
	immediate = true
)

// SendBatch implements driver.SendBatch.
func (t *topic) SendBatch(ctx context.Context, ms []*driver.Message) error {
	ch, err := t.c.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	// Put the channel into a mode where confirmations are delivered for each publish.
	// The false means that this call blocks.
	if err := ch.Confirm(false); err != nil {
		return err
	}
	// NotifyPublish returns its arg.
	confirmc := ch.NotifyPublish(make(chan amqp.Confirmation, len(ms)))
	for _, m := range ms {
		pub := toPublishing(m)
		if err := ch.Publish(t.exchange, "", mandatory, !immediate, pub); err != nil {
			return err
		}
	}
	for range ms {
		conf := <-confirmc
		if !conf.Ack {
			return errors.New("rabbitpubsub: publish failed")
		}
	}
	return nil
}

func toPublishing(m *driver.Message) amqp.Publishing {
	h := amqp.Table{}
	for k, v := range m.Metadata {
		h[k] = v
	}
	return amqp.Publishing{
		Headers:      h,
		Body:         m.Body,
		DeliveryMode: amqp.Persistent,
	}
}

// TODO(jba): figure out what errors can be retried.
func (*topic) IsRetryable(error) bool {
	return false
}
