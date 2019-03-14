// Copyright 2019 The Go Cloud Development Kit Authors
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

// Package natspubsub provides a pubsub implementation for NATS.io.
// Use CreateTopic to construct a *pubsub.Topic, and/or CreateSubscription
// to construct a *pubsub.Subscription. This package uses msgPack and the
// ugorji driver to encode and decode driver.Message to []byte.
//
// As
//
// natspubsub exposes the following types for As:
//  - Topic: *nats.Conn
//  - Subscription: *nats.Subscription
//  - Message: *nats.Msg
package natspubsub // import "gocloud.dev/pubsub/natspubsub"

import (
	"context"
	"errors"
	"reflect"

	"github.com/nats-io/go-nats"
	"github.com/ugorji/go/codec"
	"gocloud.dev/gcerrors"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
)

var errNotInitialized = errors.New("natspubsub: topic not initialized")

type topic struct {
	nc   *nats.Conn
	subj string
}

// For encoding we use msgpack from github.com/ugorji/go.
// It was already imported in go-cloud and is reasonably performant.
// However the more recent version has better resource handling with
// the addition of the following:
// 	mh.ExplicitRelease = true
// 	defer enc.Release()
// However this is not compatible with etcd at the moment.
// https://github.com/etcd-io/etcd/pull/10337
var mh codec.MsgpackHandle

func init() {
	// driver.Message.Metadata type
	dm := driver.Message{}
	mh.MapType = reflect.TypeOf(dm.Metadata)
}

// We define our own version of message here for encoding that
// only encodes Body and Metadata. Otherwise we would have to
// add codec decorations to driver.Message.
type encMsg struct {
	Body     []byte            `codec:",omitempty"`
	Metadata map[string]string `codec:",omitempty"`
}

// CreateTopic returns a *pubsub.Topic for use with NATS.
// We delay checking for the proper syntax here.
// For more info, see https://nats.io/documentation/writing_applications/subjects
func CreateTopic(nc *nats.Conn, topicName string) (*pubsub.Topic, error) {
	dt, err := createTopic(nc, topicName)
	if err != nil {
		return nil, err
	}
	return pubsub.NewTopic(dt, nil), nil
}

// createTopic returns the driver for CreateTopic. This function exists so the test
// harness can get the driver interface implementation if it needs to.
func createTopic(nc *nats.Conn, topicName string) (driver.Topic, error) {
	if nc == nil {
		return nil, errors.New("natspubsub: nats.Conn is required")
	}
	return &topic{nc, topicName}, nil
}

// SendBatch implements driver.Topic.SendBatch.
func (t *topic) SendBatch(ctx context.Context, msgs []*driver.Message) error {
	if t == nil || t.nc == nil {
		return errNotInitialized
	}

	// Reuse if possible.
	var em encMsg
	var raw [1024]byte
	b := raw[:0]
	enc := codec.NewEncoderBytes(&b, &mh)

	for _, m := range msgs {
		var payload []byte

		if err := ctx.Err(); err != nil {
			return err
		}
		if len(m.Metadata) == 0 {
			payload = m.Body
		} else {
			enc.ResetBytes(&b)
			em.Body, em.Metadata = m.Body, m.Metadata
			if err := enc.Encode(em); err != nil {
				return err
			}
			payload = b
		}
		if err := t.nc.Publish(t.subj, payload); err != nil {
			return err
		}
	}
	// Per specification this is supposed to only return after
	// a message has been sent. Normally NATS is very efficient
	// at sending messages in batches on its own and also handles
	// disconnected buffering during a reconnect event. We will
	// let NATS handle this for now. If needed we could add a
	// FlushWithContext() call which ensures the connected server
	// has processed all the messages.
	return nil
}

// IsRetryable implements driver.Topic.IsRetryable.
func (*topic) IsRetryable(error) bool { return false }

// As implements driver.Topic.As.
func (t *topic) As(i interface{}) bool {
	c, ok := i.(**nats.Conn)
	if !ok {
		return false
	}
	*c = t.nc
	return true
}

// ErrorAs implements driver.Topic.ErrorAs
func (*topic) ErrorAs(error, interface{}) bool {
	return false
}

// ErrorCode implements driver.Topic.ErrorCode
func (*topic) ErrorCode(err error) gcerrors.ErrorCode {
	switch err {
	case nil:
		return gcerrors.OK
	case context.Canceled:
		return gcerrors.Canceled
	case errNotInitialized, nats.ErrBadSubject:
		return gcerrors.FailedPrecondition
	case nats.ErrAuthorization:
		return gcerrors.PermissionDenied
	case nats.ErrMaxPayload, nats.ErrReconnectBufExceeded:
		return gcerrors.ResourceExhausted
	}
	return gcerrors.Unknown
}

type subscription struct {
	nc      *nats.Conn
	nsub    *nats.Subscription
	ackFunc func()
}

// CreateSubscription returns a *pubsub.Subscription representing a NATS subscription.
//
// ackFunc will be called when the application calls pubsub.Topic.Ack on a
// received message; Ack is a meaningless no-op for NATS. You can provide an
// empty function to leave it a no-op, or panic/log a warning if you don't
// expect Ack to be called.
//
// TODO(dlc) - Options for queue groups?
func CreateSubscription(nc *nats.Conn, subscriptionName string, ackFunc func()) (*pubsub.Subscription, error) {
	ds, err := createSubscription(nc, subscriptionName, ackFunc)
	if err != nil {
		return nil, err
	}
	return pubsub.NewSubscription(ds, nil), nil
}

func createSubscription(nc *nats.Conn, subscriptionName string, ackFunc func()) (driver.Subscription, error) {
	sub, err := nc.SubscribeSync(subscriptionName)
	if err != nil {
		return nil, err
	}
	if ackFunc == nil {
		return nil, errors.New("natspubsub: ackFunc is required")
	}
	return &subscription{nc, sub, ackFunc}, nil
}

// AckFunc implements driver.Subscription.AckFunc.
func (s *subscription) AckFunc() func() {
	if s == nil {
		return nil
	}
	return s.ackFunc
}

// ReceiveBatch implements driver.ReceiveBatch.
func (s *subscription) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	if s == nil || s.nsub == nil {
		return nil, nats.ErrBadSubscription
	}

	// Return right away if the ctx has an error.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var ms []*driver.Message

	// We will assume the desired goal is at least one message since the public API only has Receive().
	// We will load all the messages that are already queued up first.
	for {
		msg, err := s.nsub.NextMsg(0)
		if err == nil {
			dm, err := decode(msg)
			if err != nil {
				return nil, err
			}
			ms = append(ms, dm)
			if len(ms) >= maxMessages {
				break
			}
		} else if err == nats.ErrTimeout {
			break
		} else {
			return nil, err
		}
	}

	// If we have anything go ahead and return here.
	if len(ms) > 0 {
		return ms, nil
	}

	// Here we will assume the ctx has a deadline and will allow it to do its thing. We may
	// get a burst of messages but we will only wait once here and let the next call grab them.
	// The reason is so that if deadline is not properly set we will not needlessly wait here
	// for more messages when the user most likely only wants one.
	msg, err := s.nsub.NextMsgWithContext(ctx)
	if err != nil {
		return nil, err
	}
	dm, err := decode(msg)
	if err != nil {
		return nil, err
	}
	ms = append(ms, dm)
	return ms, nil
}

// Convert NATS msgs to *driver.Message.
func decode(msg *nats.Msg) (*driver.Message, error) {
	if msg == nil {
		return nil, nats.ErrInvalidMsg
	}
	var dm driver.Message
	// Everything is in the msg.Data
	dec := codec.NewDecoderBytes(msg.Data, &mh)
	err := dec.Decode(&dm)
	if err != nil {
		// This may indicate a normal NATS message, so just treat as the body.
		dm.Body = msg.Data
	}
	dm.AckID = -1 // Not applicable to NATS
	dm.AsFunc = messageAsFunc(msg)
	return &dm, nil
}

func messageAsFunc(msg *nats.Msg) func(interface{}) bool {
	return func(i interface{}) bool {
		p, ok := i.(**nats.Msg)
		if !ok {
			return false
		}
		*p = msg
		return true
	}
}

// SendAcks implements driver.Subscription.SendAcks. It should never be called
// because we provide a non-nil AckFunc.
func (s *subscription) SendAcks(ctx context.Context, ids []driver.AckID) error {
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
	switch err {
	case nil:
		return gcerrors.OK
	case context.Canceled:
		return gcerrors.Canceled
	case errNotInitialized, nats.ErrBadSubject, nats.ErrBadSubscription, nats.ErrTypeSubscription:
		return gcerrors.FailedPrecondition
	case nats.ErrAuthorization:
		return gcerrors.PermissionDenied
	case nats.ErrMaxMessages, nats.ErrSlowConsumer:
		return gcerrors.ResourceExhausted
	case nats.ErrTimeout:
		return gcerrors.DeadlineExceeded
	}
	return gcerrors.Unknown
}
