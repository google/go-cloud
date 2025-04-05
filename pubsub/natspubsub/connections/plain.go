package connections

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"github.com/nats-io/nats.go"
	"gocloud.dev/pubsub/driver"
	"net/url"
	"time"
)

func NewPlainWithEncodingV1(natsConn *nats.Conn, useV1Encoding bool) (Connection, error) {
	sv, err := ServerVersion(natsConn.ConnectedServerVersion())
	if err != nil {
		return nil, err
	}

	return newPlainConnection(natsConn, sv, useV1Encoding), nil
}

func NewPlain(natsConn *nats.Conn) (Connection, error) {
	return NewPlainWithEncodingV1(natsConn, false)
}

func newPlainConnection(natsConn *nats.Conn, version *Version, useV1Encoding bool) Connection {
	return &plainConnection{natsConnection: natsConn, version: version, useV1Encoding: useV1Encoding}
}

type plainConnection struct {
	// Connection to use for communication with the server.
	natsConnection *nats.Conn
	useV1Encoding  bool
	version        *Version
}

func (c *plainConnection) Raw() interface{} {
	return c.natsConnection
}

func (c *plainConnection) CreateTopic(ctx context.Context, opts *TopicOptions) (Topic, error) {

	useV1Encoding := !c.version.V2Supported() || c.useV1Encoding
	return &plainNatsTopic{subject: opts.Subject, plainConn: c.natsConnection, useV1Encoding: useV1Encoding}, nil
}

func (c *plainConnection) CreateSubscription(ctx context.Context, opts *SubscriptionOptions) (Queue, error) {

	//We force the batch fetch size to 1, as only jetstream enabled connections can do batch fetches
	// see: https://pkg.go.dev/github.com/nats-io/nats.go@v1.30.1#Conn.QueueSubscribeSync
	opts.ConsumerRequestBatch = 1

	// Determine if we should use V1 encoding - either we need to because server doesn't support V2
	// or the client explicitly requested V1 encoding
	useV1Decoding := !c.version.V2Supported() || c.useV1Encoding

	if opts.Durable != "" {

		subsc, err := c.natsConnection.QueueSubscribeSync(opts.Subjects[0], opts.Durable)
		if err != nil {
			return nil, err
		}

		return &natsConsumer{consumer: subsc, isQueueGroup: true,
			batchFetchTimeout: time.Duration(opts.ConsumerRequestTimeoutMs) * time.Millisecond,
			useV1Decoding:     useV1Decoding}, nil
	}

	// Using nats without any form of queue mechanism is fine only where
	// loosing some messages is ok as this essentially is an atmost once delivery situation here.
	subsc, err := c.natsConnection.SubscribeSync(opts.Subjects[0])
	if err != nil {
		return nil, err
	}

	return &natsConsumer{consumer: subsc, isQueueGroup: false,
		batchFetchTimeout: time.Duration(opts.ConsumerRequestTimeoutMs) * time.Millisecond,
		useV1Decoding:     useV1Decoding}, nil

}

func (c *plainConnection) DeleteSubscription(ctx context.Context, opts *SubscriptionOptions) error {
	return nil
}

type plainNatsTopic struct {
	subject       string
	plainConn     *nats.Conn
	useV1Encoding bool
}

func (t *plainNatsTopic) UseV1Encoding() bool {
	return t.useV1Encoding
}
func (t *plainNatsTopic) Subject() string {
	return t.subject
}
func (t *plainNatsTopic) PublishMessage(_ context.Context, msg *nats.Msg) (string, error) {
	var err error
	if t.UseV1Encoding() {
		err = t.plainConn.Publish(msg.Subject, msg.Data)
		return "", err
	}
	err = t.plainConn.PublishMsg(msg)
	return "", nil
}

type natsConsumer struct {
	consumer          *nats.Subscription
	isQueueGroup      bool
	batchFetchTimeout time.Duration
	useV1Decoding     bool
}

func (q *natsConsumer) UseV1Decoding() bool {
	return q.useV1Decoding
}
func (q *natsConsumer) IsQueueGroup() bool {
	return false
}

func (q *natsConsumer) Unsubscribe() error {
	return q.consumer.Unsubscribe()
}

func (q *natsConsumer) ReceiveMessages(_ context.Context, _ int) ([]*driver.Message, error) {

	var messages []*driver.Message

	msg, err := q.consumer.NextMsg(q.batchFetchTimeout)
	if err != nil {
		if errors.Is(err, nats.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
			return messages, nil
		}

		return nil, err
	}

	var driverMsg *driver.Message
	if q.UseV1Decoding() {
		driverMsg, err = decodeV1Message(msg)
	} else {
		driverMsg, err = decodeMessage(msg)
	}

	if err != nil {
		return nil, err
	}

	messages = append(messages, driverMsg)

	return messages, nil

}

func (q *natsConsumer) Ack(_ context.Context, ids []driver.AckID) error {
	for _, id := range ids {
		msg, ok := id.(*nats.Msg)
		if !ok {
			continue
		}
		_ = msg.Ack()
	}

	return nil
}

func (q *natsConsumer) Nack(_ context.Context, ids []driver.AckID) error {
	for _, id := range ids {
		msg, ok := id.(*nats.Msg)
		if !ok {
			continue
		}
		_ = msg.Nak()
	}

	return nil
}

func messageAsFunc(msg *nats.Msg) func(interface{}) bool {
	return func(i any) bool {
		p, ok := i.(**nats.Msg)
		if !ok {
			return false
		}
		*p = msg
		return true
	}
}

func decodeV1Message(msg *nats.Msg) (*driver.Message, error) {
	if msg == nil {
		return nil, nats.ErrInvalidMsg
	}
	var dm driver.Message

	dm.AckID = msg // Set to the original NATS message for proper acking
	dm.AsFunc = messageAsFunc(msg)

	// Try to decode as a v1 encoded message (with metadata and body encoded using gob)
	buf := bytes.NewBuffer(msg.Data)
	dec := gob.NewDecoder(buf)

	metadata := make(map[string]string)
	if err := dec.Decode(&metadata); err != nil {
		// If we can't decode as v1 format, treat the entire payload as body
		dm.Metadata = nil
		dm.Body = msg.Data
		return &dm, nil
	}

	dm.Metadata = metadata
	
	// Now decode the body
	var body []byte
	if err := dec.Decode(&body); err != nil {
		return nil, err
	}
	dm.Body = body
	
	return &dm, nil
}

func decodeMessage(msg *nats.Msg) (*driver.Message, error) {
	if msg == nil {
		return nil, nats.ErrInvalidMsg
	}

	dm := driver.Message{
		AsFunc: messageAsFunc(msg),
		Body:   msg.Data,
	}

	if msg.Header != nil {
		dm.Metadata = map[string]string{}
		for k, v := range msg.Header {
			var sv string
			if len(v) > 0 {
				sv = v[0]
			}
			kb, err := url.QueryUnescape(k)
			if err != nil {
				return nil, err
			}
			vb, err := url.QueryUnescape(sv)
			if err != nil {
				return nil, err
			}
			dm.Metadata[kb] = vb
		}
	}

	dm.AckID = msg

	return &dm, nil
}
