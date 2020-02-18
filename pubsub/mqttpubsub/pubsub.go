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

package mqttpubsub

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"net/url"
	"os"
	"path"
	"sync"
	"time"

  	"gocloud.dev/gcerrors"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
)

func init() {
	o := new(defaultDialer)
	pubsub.DefaultURLMux().RegisterTopic(Scheme, o)
	pubsub.DefaultURLMux().RegisterSubscription(Scheme, o)
}

// defaultDialer dials a default MQTT server based on the environment
// variable "MQTT_SERVER_URL".
type defaultDialer struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

func (o *defaultDialer) defaultConn(ctx context.Context) (*URLOpener, error) {
	o.init.Do(func() {
		serverURL := os.Getenv("MQTT_SERVER_URL")
		if serverURL == "" {
			o.err = errors.New("MQTT_SERVER_URL environment variable not set")
			return
		}
		_, cancelF := context.WithTimeout(ctx, time.Second*3)
		defer cancelF()

		conn, err := defaultMQTTConnect(serverURL)
		if err != nil {
			o.err = err
			return
		}
		o.opener = &URLOpener{Connection: conn}
	})
	return o.opener, o.err
}

func (o *defaultDialer) OpenTopicURL(ctx context.Context, u *url.URL) (*pubsub.Topic, error) {
	opener, err := o.defaultConn(ctx)
	if err != nil {
		return nil, fmt.Errorf("open topic %v: failed to open default connection: %v", u, err)
	}
	return opener.OpenTopicURL(ctx, u)
}

func (o *defaultDialer) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*pubsub.Subscription, error) {
	opener, err := o.defaultConn(ctx)
	if err != nil {
		return nil, fmt.Errorf("open subscription %v: failed to open default connection: %v", u, err)
	}
	return opener.OpenSubscriptionURL(ctx, u)
}

// Scheme is the URL scheme mqttpubsub registers its URLOpeners under on pubsub.DefaultMux.
const Scheme = "mqtt"

// URLOpener opens RabbitMQ URLs like "mqtt://myexchange" for
// topics or "mqtt://myqueue" for subscriptions.
//
// For topics, the URL's host+path is used as the exchange name.
//
// For subscriptions, the URL's host+path is used as the queue name.
//
// No query parameters are supported.
type URLOpener struct {
	// Connection to use for communication with the server.
	Connection mqttConn

	// TopicOptions specifies the options to pass to OpenTopic.
	TopicOptions TopicOptions
	// SubscriptionOptions specifies the options to pass to OpenSubscription.
	SubscriptionOptions SubscriptionOptions
}

// OpenTopicURL opens a pubsub.Topic based on u.
func (o *URLOpener) OpenTopicURL(ctx context.Context, u *url.URL) (*pubsub.Topic, error) {
	for param := range u.Query() {
		return nil, fmt.Errorf("open topic %v: invalid query parameter %q", u, param)
	}
	exchangeName := path.Join(u.Host, u.Path)
	return OpenTopic(o.Connection, exchangeName, &o.TopicOptions), nil
}

// OpenSubscriptionURL opens a pubsub.Subscription based on u.
func (o *URLOpener) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*pubsub.Subscription, error) {
	for param := range u.Query() {
		return nil, fmt.Errorf("open subscription %v: invalid query parameter %q", u, param)
	}
	queueName := path.Join(u.Host, u.Path)
	return OpenSubscription(o.Connection, queueName, &o.SubscriptionOptions), nil
}


type topic struct {
	name string
	conn     mqttConn
}

// TopicOptions sets options for constructing a *pubsub.Topic backed by
// RabbitMQ.
type TopicOptions struct{}

// SubscriptionOptions sets options for constructing a *pubsub.Subscription
// backed by RabbitMQ.
type SubscriptionOptions struct{}

func OpenTopic(conn mqttConn, name string, _ *TopicOptions) (*pubsub.Topic, error) {
	dt, err := openTopic(conn, name)
	if err != nil {
		return nil, err
	}
	return pubsub.NewTopic(dt, nil), nil
}

// openTopic returns the driver for OpenTopic. This function exists so the test
// harness can get the driver interface implementation if it needs to.
func openTopic(conn mqttConn, name string) (driver.Topic, error) {
	if conn == nil {
		return nil, errConnRequired
	}
	return &topic{name, conn}, nil
}

// SendBatch implements driver.Topic.SendBatch.
func (t *topic) SendBatch(ctx context.Context, msgs []*driver.Message) error {
	if t == nil || t.conn == nil {
		return errNotInitialized
	}

	for _, m := range msgs {
		if err := ctx.Err(); err != nil {
			return err
		}

		payload, err := encodeMessage(m)
		if err != nil {
			return err
		}
		if m.BeforeSend != nil {
			asFunc := func(i interface{}) bool { return false }
			if err := m.BeforeSend(asFunc); err != nil {
				return err
			}
		}
		token := t.conn.Publish(t.name, defaultQOS, false, payload)
		if token.Wait() && token.Error() != nil {
			return token.Error()
		}
	}
	return nil
}

// IsRetryable implements driver.Topic.IsRetryable.
func (*topic) IsRetryable(error) bool { return false }

// As implements driver.Topic.As.
func (t *topic) As(i interface{}) bool {
	c, ok := i.(*mqttConn)
	if !ok {
		return false
	}
	*c = t.conn
	return true
}

// ErrorAs implements driver.Topic.ErrorAs
func (*topic) ErrorAs(error, interface{}) bool {
	return false
}

// ErrorCode implements driver.Topic.ErrorCode
func (*topic) ErrorCode(err error) gcerrors.ErrorCode {
	return whichError(err)

}

// Close implements driver.Topic.Close.
func (t *topic) Close() error {
	t.conn.Disconnect(0)
	if t.conn.IsConnected() {
		return errStillConnected
	}
	return nil
}

type subscription struct {
	conn   mqttConn
	topicName string

	mu sync.Mutex
	err error
}

func OpenSubscription(conn mqttConn, topicName string, _ *SubscriptionOptions) (*pubsub.Subscription, error) {
	ds, err := openSubscription(conn, topicName)
	if err != nil {
		return nil, err
	}
	return pubsub.NewSubscription(ds, nil, nil), nil
}

func openSubscription(conn mqttConn, topicName string) (driver.Subscription, error) {
	token := conn.Subscribe(topicName, defaultQOS, nil)
	if token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}
	return &subscription{conn, topicName}, nil
}

// ReceiveBatch implements driver.ReceiveBatch.
func (s *subscription) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	if s == nil || s.conn == nil {
		return nil, errConnRequired
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var dm *driver.Message
	s.conn.AddRoute(s.topicName, func(client mqtt.Client, m mqtt.Message) {
		dm, s.err = decode(m)
	})
	return []*driver.Message{dm}, s.err
}

// Convert NATS msgs to *driver.Message.
func decode(msg mqtt.Message) (*driver.Message, error) {
	if msg == nil {
		return nil, errInvalidMessage
	}
	var dm driver.Message
	if err := decodeMessage(msg.Payload(), &dm); err != nil {
		return nil, err
	}
	dm.AckID = -1 // Not applicable to NATS
	dm.AsFunc = messageAsFunc(msg)
	return &dm, nil
}

func messageAsFunc(msg mqtt.Message) func(interface{}) bool {
	return func(i interface{}) bool {
		p, ok := i.(*mqtt.Message)
		if !ok {
			return false
		}
		*p = msg
		return true
	}
}

// SendAcks implements driver.Subscription.SendAcks.
func (s *subscription) SendAcks(ctx context.Context, ids []driver.AckID) error {
	// Ack is a no-op.
	return nil
}

// CanNack implements driver.CanNack.
func (s *subscription) CanNack() bool { return false }

// SendNacks implements driver.Subscription.SendNacks. It should never be called
// because we return false for CanNack.
func (s *subscription) SendNacks(ctx context.Context, ids []driver.AckID) error {
	panic("unreachable")
}

// IsRetryable implements driver.Subscription.IsRetryable.
func (s *subscription) IsRetryable(error) bool { return false }

// As implements driver.Subscription.As.
func (s *subscription) As(i interface{}) bool {
	c, ok := i.(**nats.Subscription)
	if !ok {
		return false
	}
	*c = s.nsub
	return true
}

// ErrorAs implements driver.Subscription.ErrorAs
func (*subscription) ErrorAs(error, interface{}) bool {
	return false
}

// ErrorCode implements driver.Subscription.ErrorCode
func (*subscription) ErrorCode(err error) gcerrors.ErrorCode {
	return whichError(err)
}

// Close implements driver.Subscription.Close.
func (*subscription) Close() error { return nil }

func encodeMessage(dm *driver.Message) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if len(dm.Metadata) == 0 {
		return dm.Body, nil
	}
	if err := enc.Encode(dm.Metadata); err != nil {
		return nil, err
	}
	if err := enc.Encode(dm.Body); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeMessage(data []byte, dm *driver.Message) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	if err := dec.Decode(&dm.Metadata); err != nil {
		// This may indicate a normal NATS message, so just treat as the body.
		dm.Metadata = nil
		dm.Body = data
		return nil
	}
	return dec.Decode(&dm.Body)
}
