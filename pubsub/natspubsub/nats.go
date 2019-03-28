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
// URLs
//
// For pubsub.OpenTopic and pubsub.OpenSubscription, natspubsub registers
// for the scheme "nats".
// The default URL opener will connect to a default server based on the
// environment variable "NATS_SERVER_URL".
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://godoc.org/gocloud.dev#hdr-URLs for background information.
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
	"fmt"
	"log"
	"net/url"
	"os"
	"path"
	"reflect"
	"sync"

	"github.com/nats-io/go-nats"
	"github.com/ugorji/go/codec"
	"gocloud.dev/gcerrors"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
)

var errNotInitialized = errors.New("natspubsub: topic not initialized")

func init() {
	o := new(defaultDialer)
	pubsub.DefaultURLMux().RegisterTopic(Scheme, o)
	pubsub.DefaultURLMux().RegisterSubscription(Scheme, o)
}

// defaultDialer dials a default NATS server based on the environment
// variable "NATS_SERVER_URL".
type defaultDialer struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

func (o *defaultDialer) defaultConn(ctx context.Context) (*URLOpener, error) {
	o.init.Do(func() {
		serverURL := os.Getenv("NATS_SERVER_URL")
		if serverURL == "" {
			o.err = errors.New("NATS_SERVER_URL environment variable not set")
			return
		}
		conn, err := nats.Connect(serverURL)
		if err != nil {
			o.err = fmt.Errorf("failed to dial NATS_SERVER_URL %q: %v", serverURL, err)
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

// Scheme is the URL scheme natspubsub registers its URLOpeners under on pubsub.DefaultMux.
const Scheme = "nats"

// URLOpener opens NATS URLs like "nats://mytopic".
//
// The URL host+path is used as the topic name.
//
// The following query parameters are supported:
//   - ackfunc: One of "log", "noop", "panic"; defaults to "panic". Determines
//       the behavior if pubsub.Subscription.Ack (which is a meaningless no-op
//       for NATS) is called. "log" means a log.Printf warning will be emitted;
//       "noop" means nothing will happen; and "panic" means the application
//       will panic.
// No query parameters are supported.
type URLOpener struct {
	// Connection to use for communication with the server.
	Connection *nats.Conn
	// TopicOptions specifies the options to pass to OpenTopic.
	TopicOptions TopicOptions
	// SubscriptionOptions specifies the options to pass to OpenSubscription.
	SubscriptionOptions SubscriptionOptions
}

// OpenTopicURL opens a pubsub.Topic based on u.
func (o *URLOpener) OpenTopicURL(ctx context.Context, u *url.URL) (*pubsub.Topic, error) {
	for param := range u.Query() {
		return nil, fmt.Errorf("open topic %v: invalid query parameter %s", u, param)
	}
	topicName := path.Join(u.Host, u.Path)
	return CreateTopic(o.Connection, topicName, &o.TopicOptions)
}

// AckWarning is a message that may be used in ackFuncs.
const AckWarning = "pubsub.Subscription.Ack was called for a NATS message; Ack is a meaningless no-op for NATS due to its at-most-once semantics. See the package documentation for how to disable this message."

// OpenSubscriptionURL opens a pubsub.Subscription based on u.
func (o *URLOpener) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*pubsub.Subscription, error) {
	q := u.Query()

	var ackFunc func()
	s := q.Get("ackfunc")
	switch s {
	case "log":
		ackFunc = func() { log.Printf(AckWarning) }
	case "noop":
		ackFunc = func() {}
	case "", "panic":
		ackFunc = func() { panic(AckWarning) }
	default:
		return nil, fmt.Errorf("open subscription %v: invalid ackfunc %q (valid values are log, noop, panic)", u, s)
	}
	q.Del("ackfunc")

	for param := range q {
		return nil, fmt.Errorf("open subscription %v: invalid query parameter %s", u, param)
	}
	topicName := path.Join(u.Host, u.Path)
	return CreateSubscription(o.Connection, topicName, ackFunc, &o.SubscriptionOptions)
}

// TopicOptions sets options for constructing a *pubsub.Topic backed by NATS.
type TopicOptions struct{}

// SubscriptionOptions sets options for constructing a *pubsub.Subscription
// backed by NATS.
type SubscriptionOptions struct{}

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
// For more info, see https://nats.io/documentation/writing_applications/subjects
func CreateTopic(nc *nats.Conn, topicName string, _ *TopicOptions) (*pubsub.Topic, error) {
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
func CreateSubscription(nc *nats.Conn, subscriptionName string, ackFunc func(), _ *SubscriptionOptions) (*pubsub.Subscription, error) {
	ds, err := createSubscription(nc, subscriptionName, ackFunc)
	if err != nil {
		return nil, err
	}
	return pubsub.NewSubscription(ds, 0, nil), nil
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

// SendNacks implements driver.Subscription.SendNacks. It should never be called
// because we provide a non-nil AckFunc.
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
