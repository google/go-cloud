// Copyright 2021 The Go Cloud Development Kit Authors
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

// Package stanpubsub provides a pubsub implementation for NATS Streaming client.
// See https://docs.nats.io/nats-streaming-concepts/intro for an overview.
//
// Use OpenTopic to construct a *pubsub.Topic, and/or OpenSubscription to construct a
// *pubsub.Subscription. This package uses gob to encode and decode driver.Message to
// []byte.
//
// URLs
//
// For pubsub.OpenTopic and pubsub.OpenSubscription, stanpubsub registers
// for the scheme "stan".
// The default URL opener will connect to a default server based on the environment variables:
// 	- STAN_SERVER_URL	- url address of the NATS server
//	- STAN_CLUSTER_ID	- NATS Streaming cluster id
//	- STAN_CLIENT_ID	- NAST Streaming client id
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// Message Delivery Semantics
//
// NATS Streaming supports at-least-once semantics; by default applications don't need to call Message.Ack,
// and must not call Message.Nack.
// Optionally a user can set a subscription to be manually acked by using `stan.SetManualAckMode` option
// or by setting `manualAck` query parameter when setting from URL.
// See https://godoc.org/gocloud.dev/pubsub#hdr-At_most_once_and_At_least_once_Delivery
// for more background.
//
// As
//
// stanpubsub exposes the following types for As:
//  - Topic: stan.Conn
//  - Subscription: stan.Subscription
//  - Message.BeforeSend: None.
//  - Message.AfterSend: None.
//  - Message: *stan.Msg

package stanpubsub

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/stan.go"
	"gocloud.dev/gcerrors"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/batcher"
	"gocloud.dev/pubsub/driver"
)

func init() {
	dd := new(defaultDialer)
	pubsub.DefaultURLMux().RegisterTopic(Scheme, dd)
	pubsub.DefaultURLMux().RegisterSubscription(Scheme, dd)
}

func connect() (*URLOpener, error) {
	serverURL := os.Getenv("STAN_SERVER_URL")
	if serverURL == "" {
		return nil, errors.New("STAN_SERVER_URL environment variable not set")
	}
	clusterID := os.Getenv("STAN_CLUSTER_ID")
	if clusterID == "" {
		return nil, errors.New("STAN_CLUSTER_ID environment variable not set")
	}
	clientID := os.Getenv("STAN_CLIENT_ID")
	if clientID == "" {
		return nil, errors.New("STAN_CLIENT_ID environment variable not set")
	}
	conn, err := stan.Connect(clusterID, clientID, stan.NatsURL(serverURL))
	if err != nil {
		return nil, fmt.Errorf("failed to dial STAN_SERVER_URL %q: STAN_CLUSTER_ID: %q STAN_CLIENT_ID: %q,  %v", serverURL, clusterID, clientID, err)
	}
	return &URLOpener{Connection: conn, SubscriptionOptions: DefaultSubscriptionOptions}, nil
}

// errNotInitialized is an error returned when the topic or subscription is not initialized.
var errNotInitialized = errors.New("stanpubsub: not initialized")

// defaultDialer dials a default NATS Streaming server based on three required environment variables:
//	- STAN_SERVER_URL - defines NATS Server Url
//	- STAN_CLUSTER_ID - defines NATS Streaming Cluster ID
//	- STAN_CLIENT_ID  - defined NATS Streaming Client ID
type defaultDialer struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

// OpenTopicURL implements pubsub.TopicURLOpener interface.
func (o *defaultDialer) OpenTopicURL(ctx context.Context, u *url.URL) (*pubsub.Topic, error) {
	opener, err := o.defaultConn()
	if err != nil {
		return nil, fmt.Errorf("open topic %v: failed to open default connection: %v", u, err)
	}
	return opener.OpenTopicURL(ctx, u)
}

// OpenSubscriptionURL implements pubsub.SubscriptionURLOpener interface.
func (o *defaultDialer) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*pubsub.Subscription, error) {
	opener, err := o.defaultConn()
	if err != nil {
		return nil, fmt.Errorf("open subscription %v: failed to open default connection: %v", u, err)
	}
	return opener.OpenSubscriptionURL(ctx, u)
}

func (o *defaultDialer) defaultConn() (*URLOpener, error) {
	o.init.Do(func() {
		o.opener, o.err = connect()
	})
	return o.opener, o.err
}

// Scheme is the URL scheme stanpubsub registers its URLOpeners under on pubsub.DefaultMux.
const Scheme = "stan"

// URLOpener opens NATS URLs like "stan://mysubject".
//
// The URL host+path is used as the subject.
//
// No topic query parameters are supported.
//
// The following query parameters are supported for subscription:
//	- queue	 		 - sets the name of the queue.
//						see: https://docs.nats.io/developing-with-nats-streaming/queues for more information.
//	- durable_name	 - sets the durable name for the subscription -
//					   	see: https://docs.nats.io/developing-with-nats-streaming/durables for more information.
//	- max_inflight 	 - sets the maximum number of messages the cluster will send without an ACK.
//						see: https://docs.nats.io/developing-with-nats-streaming/acks#max-in-flight for more information.
//	- start_sequence - sets the desired start sequence position and state
//	- ack_wait 		 - sets the timeout for waiting for an ACK from the cluster's point of view for delivered messages
//	- manual_acks	 - sets the subscription to handle message acks manually by using pubsub.Message.Ack().
type URLOpener struct {
	// Connection to use for communication with the server.
	Connection stan.Conn
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
	subject := path.Join(u.Host, u.Path)
	return OpenTopic(o.Connection, subject, &o.TopicOptions)
}

// OpenSubscriptionURL opens a pubsub.Subscription based on input url.
func (o *URLOpener) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*pubsub.Subscription, error) {
	opts := o.SubscriptionOptions
	for param, values := range u.Query() {
		switch param {
		case "queue":
			opts.Queue = values[0]
		case "durable_name":
			opts.DurableName = values[0]
		case "max_inflight":
			maxInflight, err := strconv.Atoi(values[0])
			if err != nil {
				return nil, fmt.Errorf("open subscription %v: invalid value for maxInflight query parameter: %s, %w", u, values[0], err)
			}
			opts.MaxInflight = maxInflight
		case "start_sequence":
			startSequence, err := strconv.ParseUint(values[0], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("open subscription %v: invalid value for startSequence query parameter: %s, %w", u, values[0], err)
			}
			opts.StartSequence = startSequence
			opts.StartAt = StartPositionSequenceStart
		case "ack_wait":
			ackWait, err := time.ParseDuration(values[0])
			if err != nil {
				return nil, fmt.Errorf("open subscription %v: invalid value for ackWait query parameter: %s, %w", u, values[0], err)
			}
			opts.AckWait = ackWait
		case "manual_acks":
			if values[0] != "" {
				return nil, fmt.Errorf("open subscription %v: no values are permitted for the manualAck query parameter", u)
			}
			opts.ManualAcks = true
		default:
			return nil, fmt.Errorf("open subscription %v: invalid query parameter %s", u, param)
		}
	}
	subject := path.Join(u.Host, u.Path)

	sub, err := openSubscription(o.Connection, subject, &opts)
	if err != nil {
		return nil, err
	}
	return pubsub.NewSubscription(sub, &batcher.Options{MaxBatchSize: opts.MaxInflight}, &batcher.Options{MaxBatchSize: opts.MaxInflight}), nil
}

// TopicOptions sets options for constructing a *pubsub.Topic backed by NATS Streaming.
type TopicOptions struct{}

// OpenTopic returns a *pubsub.Topic for use with NATS.
// The subject is the NATS Subject; for more info, see
// https://nats.io/documentation/writing_applications/subjects.
func OpenTopic(nc stan.Conn, subject string, _ *TopicOptions) (*pubsub.Topic, error) {
	dt, err := openTopic(nc, subject)
	if err != nil {
		return nil, err
	}
	return pubsub.NewTopic(dt, nil), nil
}

// SubscriptionOptions sets options for constructing a *pubsub.Subscription backed by NATS Streaming.
type SubscriptionOptions struct {
	// Queue defines the subscription queue name.
	// See: https://docs.nats.io/developing-with-nats-streaming/queues for more information.
	Queue string
	// DurableName,defines the durable subscription name.
	// See: https://docs.nats.io/developing-with-nats-streaming/durables for more information.
	DurableName string
	// MaxInflight controls the maximum number of messages the cluster will have inflight without an ACK.
	// See: https://docs.nats.io/developing-with-nats-streaming/acks#max-in-flight for more information.
	MaxInflight int
	// AckWait defines the duration the cluster will wait for an ACK for a given message.
	AckWait time.Duration
	// StartPosition defines the position where the subscription would start..
	// By default it sets the position to take only new messages.
	StartAt StartPosition
	// StartSequence is an option used together with the StartPositionSequenceStart
	// which defines what is the sequence start position.
	StartSequence uint64
	// StartTime is the option used together with the StartPositionTimeStart
	// which sets the desired start time position and state.
	StartTime time.Time
	// ManualAcks is an option that requires the user to manually ack pubsub.Messages.
	ManualAcks bool
}

// Validate checks if the subscription options are valid.
func (s *SubscriptionOptions) Validate() error {
	if s.StartSequence != 0 && s.StartAt != StartPositionSequenceStart {
		return errors.New("stanpubsub: subscription option StartSequence should be paired with the StartAt = StartPositionSequenceStart")
	}
	if !s.StartTime.IsZero() && s.StartAt != StartPositionTimeStart {
		return errors.New("stanpubsub: subscription option StartTime should be paired with the StartAt = StartPositionTimeDeltaStart")
	}
	return nil
}

// StartWithLastReceived sets start position to last received.
func (s *SubscriptionOptions) StartWithLastReceived() {
	s.StartAt = StartPositionLastReceived
}

// DeliverAllAvailable sets the start subscription position to the first available message.
// This should result in delivery of all available messages.
func (s *SubscriptionOptions) DeliverAllAvailable() {
	s.StartAt = StartPositionFirst
}

// StartAtTime sets the desired start time position and state.
func (s *SubscriptionOptions) StartAtTime(ts time.Time) {
	s.StartAt = StartPositionTimeStart
	s.StartTime = ts
}

// StartAtSequence starts the subscription at given sequence number.
func (s *SubscriptionOptions) StartAtSequence(sequence uint64) {
	s.StartAt = StartPositionSequenceStart
	s.StartSequence = sequence
}

// StartPosition is the enum that defines start position of subscription.
type StartPosition int

const (
	// StartPositionNewOnly is the default subscription start position which delivers only new messages.
	StartPositionNewOnly StartPosition = iota
	// StartPositionLastReceived sets the subscription start position to the last received message.
	StartPositionLastReceived
	// StartPositionTimeStart  sets the subscription start position at given StartTime option.
	StartPositionTimeStart
	// StartPositionSequenceStart sets subscription start position to provided StartSequence option.
	StartPositionSequenceStart
	// StartPositionFirst sets subscription start position to the first available message, which results in delivery of all available messages..
	StartPositionFirst
)

// DefaultSubscriptionOptions are the default subscription options used in the NATS Streaming.
var DefaultSubscriptionOptions = SubscriptionOptions{MaxInflight: stan.DefaultMaxInflight}

// OpenSubscription returns a *pubsub.Subscription representing a NATS subscription or NATS queue subscription.
// The subject is the NATS Subject to subscribe to;
// for more info, see https://nats.io/documentation/writing_applications/subjects.
func OpenSubscription(nc stan.Conn, subject string, options *SubscriptionOptions) (*pubsub.Subscription, error) {
	ds, err := openSubscription(nc, subject, options)
	if err != nil {
		return nil, err
	}
	maxInflight := stan.DefaultMaxInflight
	if options != nil {
		maxInflight = options.MaxInflight
	}
	return pubsub.NewSubscription(ds, &batcher.Options{MaxBatchSize: maxInflight}, nil), nil
}

// openTopic returns the driver for OpenTopic. This function exists so the test
// harness can get the driver interface implementation if it needs to.
func openTopic(nc stan.Conn, subject string) (driver.Topic, error) {
	if nc == nil {
		return nil, errors.New("stanpubsub: stan.Conn is required")
	}
	return &topic{nc, subject}, nil
}

type topic struct {
	nc      stan.Conn
	subject string
}

// SendBatch implements driver.Topic interface.
func (t *topic) SendBatch(ctx context.Context, ms []*driver.Message) error {
	if t == nil || t.nc == nil {
		return errNotInitialized
	}

	var buf bytes.Buffer
	for _, m := range ms {
		if err := ctx.Err(); err != nil {
			return err
		}
		payload, err := encodeMessage(&buf, m)
		if err != nil {
			return err
		}
		buf.Reset()

		if m.BeforeSend != nil {
			asFunc := func(i interface{}) bool { return false }
			if err := m.BeforeSend(asFunc); err != nil {
				return err
			}
		}
		if err := t.nc.Publish(t.subject, payload); err != nil {
			return err
		}

		if m.AfterSend != nil {
			asFunc := func(i interface{}) bool { return false }
			if err := m.AfterSend(asFunc); err != nil {
				return err
			}
		}
	}
	return nil
}

// IsRetryable implements driver.Topic interface.
func (t *topic) IsRetryable(err error) bool { return false }

// As implements driver.Topic interface.
func (t *topic) As(i interface{}) bool {
	nc, ok := i.(*stan.Conn)
	if !ok {
		return false
	}
	*nc = t.nc
	return true
}

// ErrorAs implement driver.Topic interface.
func (t *topic) ErrorAs(_ error, _ interface{}) bool { return false }

// ErrorCode implements driver.Topic interface.
func (t *topic) ErrorCode(err error) gcerrors.ErrorCode {
	switch err {
	case nil:
		return gcerrors.OK
	case context.Canceled:
		return gcerrors.Canceled
	case errNotInitialized:
		return gcerrors.NotFound
	case nats.ErrBadSubject, stan.ErrBadConnection:
		return gcerrors.FailedPrecondition
	case nats.ErrAuthorization:
		return gcerrors.PermissionDenied
	case nats.ErrMaxPayload, nats.ErrReconnectBufExceeded:
		return gcerrors.ResourceExhausted
	case stan.ErrTimeout:
		return gcerrors.DeadlineExceeded
	}
	return gcerrors.Unknown
}

// Close implements driver.Topic interface.
func (t *topic) Close() error { return nil }

func openSubscription(nc stan.Conn, subject string, options *SubscriptionOptions) (_ driver.Subscription, err error) {
	if options == nil {
		o := DefaultSubscriptionOptions
		options = &o
	}
	// Validate if options are composed correctly.
	if err = options.Validate(); err != nil {
		return nil, err
	}

	// MaxInflight option controls how many messages could be received at given
	// client without an ack. This perfectly matches with the size of the message channel.
	if options.MaxInflight == 0 {
		options.MaxInflight = stan.DefaultMaxInflight
	}
	s := &subscription{
		nc:        nc,
		msgChan:   make(chan *stan.Msg, options.MaxInflight),
		manualAck: options.ManualAcks,
	}

	if options.Queue != "" {
		s.sub, err = nc.QueueSubscribe(subject, options.Queue, s.enqueue, extractSubscriptionOptions(options)...)
	} else {
		s.sub, err = nc.Subscribe(subject, s.enqueue, extractSubscriptionOptions(options)...)
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

type subscription struct {
	nc        stan.Conn
	sub       stan.Subscription
	manualAck bool
	msgChan   chan *stan.Msg
}

// ReceiveBatch implements driver.Subscription.
func (s *subscription) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	if s == nil {
		return nil, stan.ErrBadSubscription
	}
	var batch []*driver.Message
	ac := time.After(time.Second)
	for {
		select {
		case msg := <-s.msgChan:
			dm, err := s.decode(msg)
			if err != nil {
				return nil, err
			}
			batch = append(batch, dm)
			if len(batch) == maxMessages {
				return batch, nil
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ac:
			return batch, nil
		}
	}
}

// SendAcks implements driver.Subscription.
func (s *subscription) SendAcks(ctx context.Context, ackIDs []driver.AckID) error {
	if !s.manualAck {
		// ack is no-op here.
		return nil
	}
	for _, ai := range ackIDs {
		msg, ok := ai.(*stan.Msg)
		if !ok {
			return fmt.Errorf("send acks - unexpected message ackId type: %T", ai)
		}
		if err := msg.Ack(); err != nil {
			return err
		}
	}
	return nil
}

// CanNack implements driver.Subscription interface.
func (s *subscription) CanNack() bool { return false }

// SendNacks implements driver.Subscription. NATS doesn't allow nack messages, CanNack returns false
// message, thus code is treated as unreachable.
func (s *subscription) SendNacks(ctx context.Context, ackIDs []driver.AckID) error {
	panic("unreachable")
}

// IsRetryable implements driver.Subscription interface.
func (s *subscription) IsRetryable(err error) bool { return false }

// As implements driver.Subscription interface.
func (s *subscription) As(i interface{}) bool {
	ns, ok := i.(*stan.Subscription)
	if !ok {
		return false
	}
	*ns = s.sub
	return true
}

// ErrorAs implements driver.Subscription interface.
func (s *subscription) ErrorAs(err error, i interface{}) bool { return false }

// ErrorCode implements driver.Subscription interface.
func (s *subscription) ErrorCode(err error) gcerrors.ErrorCode {
	switch err {
	case nil:
		return gcerrors.OK
	case context.Canceled:
		return gcerrors.Canceled
	case errNotInitialized, stan.ErrBadSubscription, nats.ErrBadSubscription:
		return gcerrors.NotFound
	case nats.ErrBadSubject, nats.ErrTypeSubscription:
		return gcerrors.FailedPrecondition
	case nats.ErrAuthorization:
		return gcerrors.PermissionDenied
	case nats.ErrMaxMessages, nats.ErrSlowConsumer:
		return gcerrors.ResourceExhausted
	case nats.ErrTimeout, stan.ErrTimeout, stan.ErrSubReqTimeout, stan.ErrCloseReqTimeout:
		return gcerrors.DeadlineExceeded
	}
	return gcerrors.Unknown
}

// Close implements driver.Subscription.
func (s *subscription) Close() error {
	if s == nil {
		return nil
	}
	if err := s.sub.Close(); err != nil {
		return err
	}
	close(s.msgChan)
	return nil
}

func (s *subscription) enqueue(msg *stan.Msg) {
	s.msgChan <- msg
}

// Convert NATS msgs to *driver.Message.
func (s *subscription) decode(msg *stan.Msg) (*driver.Message, error) {
	if msg == nil {
		return nil, nats.ErrInvalidMsg
	}
	var dm driver.Message
	if err := decodeMessage(msg.Data, &dm); err != nil {
		return nil, err
	}
	if s.manualAck {
		dm.AckID = msg
	} else {
		dm.AckID = -1
	}
	dm.AsFunc = messageAsFunc(msg)
	return &dm, nil
}

func messageAsFunc(msg *stan.Msg) func(interface{}) bool {
	return func(i interface{}) bool {
		p, ok := i.(**stan.Msg)
		if !ok {
			return false
		}
		*p = msg
		return true
	}
}

func encodeMessage(buf *bytes.Buffer, dm *driver.Message) ([]byte, error) {
	enc := gob.NewEncoder(buf)
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

func extractSubscriptionOptions(o *SubscriptionOptions) (options []stan.SubscriptionOption) {
	if o.AckWait != 0 {
		options = append(options, stan.AckWait(o.AckWait))
	}
	if o.ManualAcks {
		options = append(options, stan.SetManualAckMode())
	}
	if o.MaxInflight != 0 {
		options = append(options, stan.MaxInflight(o.MaxInflight))
	}
	if o.DurableName != "" {
		options = append(options, stan.DurableName(o.DurableName))
	}
	switch o.StartAt {
	case StartPositionSequenceStart:
		options = append(options, stan.StartAtSequence(o.StartSequence))
	case StartPositionFirst:
		options = append(options, stan.DeliverAllAvailable())
	case StartPositionLastReceived:
		options = append(options, stan.StartWithLastReceived())
	case StartPositionTimeStart:
		options = append(options, stan.StartAtTime(o.StartTime))
	}
	return options
}
