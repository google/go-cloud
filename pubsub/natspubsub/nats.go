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

// Package natspubsub provides a pubsub implementation for NATS.io. Use OpenTopic to
// construct a *pubsub.Topic, and/or OpenSubscription to construct a
// *pubsub.Subscription. This package uses gob to encode and decode driver.Message to
// []byte.
//
// # URLs
//
// For pubsub.OpenTopic and pubsub.OpenSubscription, natspubsub registers
// for the scheme "nats".
// The default URL opener will connect to a default server based on the
// environment variable "NATS_SERVER_URL".
//
// For servers that support it (NATS Server 2.2.0 or later), messages can
// be encoded using native NATS message headers, and native message content.
// This provides full support for non-Go clients. Versions prior to 2.2.0
// uses gob.Encoder to encode the message headers and content, which limits
// the subscribers only to Go clients.
// To use this feature, set the query parameter "natsv2" in the URL.
// If no value is provided, it assumes the value is true. Otherwise, the value
// needs to be parsable as a boolean. For example:
//   - nats://mysubject?natsv2
//   - nats://mysubject?natsv2=true
//
// This feature can also be enabled by setting the UseV2 field in the
// URLOpener.
// If the server does not support this feature, any attempt to use it will
// result in an error.
// Using native NATS message headers and content is more efficient than using
// gob.Encoder, and allows non-Go clients to subscribe to the topic and
// receive messages. It is recommended to use this feature if the server
// supports it.
//
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// # Message Delivery Semantics
//
// NATS supports at-most-semantics; applications need not call Message.Ack,
// and must not call Message.Nack.
// See https://godoc.org/gocloud.dev/pubsub#hdr-At_most_once_and_At_least_once_Delivery
// for more background.
//
// # As
//
// natspubsub exposes the following types for As:
//   - Topic: *nats.Conn
//   - Subscription: *nats.Subscription
//   - Message.BeforeSend: None for v1, *nats.Msg for v2.
//   - Message.AfterSend: None.
//   - Message: *nats.Msg
package natspubsub // import "gocloud.dev/pubsub/natspubsub"

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"gocloud.dev/gcerrors"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/batcher"
	"gocloud.dev/pubsub/driver"
)

var errNotInitialized = errors.New("natspubsub: topic not initialized")

var recvBatcherOpts = &batcher.Options{
	// NATS has at-most-once semantics, meaning once it delivers a message, the
	// message won't be delivered again.
	// Therefore, we can't allow the portable type to read-ahead and queue any
	// messages; they might end up undelivered if the user never calls Receive
	// to get them. Setting both the MaxBatchSize and MaxHandlers to one means
	// that we'll only return a message at a time, which should be immediately
	// returned to the user.
	//
	// Note: there is a race condition where the original Receive that
	// triggered a call to ReceiveBatch ends up failing (e.g., due to a
	// Done context), and ReceiveBatch returns a message that ends up being
	// queued for the next Receive. That message is at risk of being dropped.
	// This seems OK.
	MaxBatchSize: 1,
	MaxHandlers:  1, // max concurrency for receives
}

func init() {
	o := new(defaultDialer)
	pubsub.DefaultURLMux().RegisterTopic(Scheme, o)
	pubsub.DefaultURLMux().RegisterSubscription(Scheme, o)
}

// defaultDialer dials a default NATS server based on the environment
// variable "NATS_SERVER_URL".
type defaultDialer struct {
	init sync.Once
	err  error

	opener   URLOpener
	openerV2 URLOpener
}

func (o *defaultDialer) defaultConn(ctx context.Context) error {
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
		o.opener = URLOpener{Connection: conn}
		o.openerV2 = URLOpener{Connection: conn, UseV2: true}
	})
	return o.err
}

type serverVersion struct {
	major, minor, patch int
}

func (o *defaultDialer) OpenTopicURL(ctx context.Context, u *url.URL) (*pubsub.Topic, error) {
	err := o.defaultConn(ctx)
	if err != nil {
		return nil, fmt.Errorf("open topic %v: failed to open default connection: %v", u, err)
	}
	useV2, err := queryUseV2(u.Query())
	if err != nil {
		return nil, fmt.Errorf("open topic %v: %v", u, err)
	}
	if useV2 {
		return o.openerV2.OpenTopicURL(ctx, u)
	}
	return o.opener.OpenTopicURL(ctx, u)
}

func (o *defaultDialer) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*pubsub.Subscription, error) {
	err := o.defaultConn(ctx)
	if err != nil {
		return nil, fmt.Errorf("open subscription %v: failed to open default connection: %v", u, err)
	}
	useV2, err := queryUseV2(u.Query())
	if err != nil {
		return nil, fmt.Errorf("open subscription %v: %v", u, err)
	}
	if useV2 {
		return o.openerV2.OpenSubscriptionURL(ctx, u)
	}
	return o.opener.OpenSubscriptionURL(ctx, u)
}

var semVerRegexp = regexp.MustCompile(`\Av?([0-9]+)\.?([0-9]+)?\.?([0-9]+)?`)

func parseServerVersion(version string) (serverVersion, error) {
	m := semVerRegexp.FindStringSubmatch(version)
	if m == nil {
		return serverVersion{}, errors.New("failed to parse server version")
	}
	var (
		major, minor, patch int
		err                 error
	)
	major, err = strconv.Atoi(m[1])
	if err != nil {
		return serverVersion{}, fmt.Errorf("failed to parse server version major number %q: %v", m[1], err)
	}
	minor, err = strconv.Atoi(m[2])
	if err != nil {
		return serverVersion{}, fmt.Errorf("failed to parse server version minor number %q: %v", m[2], err)
	}
	patch, err = strconv.Atoi(m[3])
	if err != nil {
		return serverVersion{}, fmt.Errorf("failed to parse server version patch number %q: %v", m[3], err)
	}
	return serverVersion{major: major, minor: minor, patch: patch}, nil
}

// Scheme is the URL scheme natspubsub registers its URLOpeners under on pubsub.DefaultMux.
const Scheme = "nats"

// URLOpener opens NATS URLs like "nats://mysubject?natsv2=true".
//
// The URL host+path is used as the subject.
//
// No query parameters are supported.
type URLOpener struct {
	// Connection to use for communication with the server.
	Connection *nats.Conn
	// TopicOptions specifies the options to pass to OpenTopic.
	TopicOptions TopicOptions
	// SubscriptionOptions specifies the options to pass to OpenSubscription.
	SubscriptionOptions SubscriptionOptions
	// UseV2 indicates whether the NATS Server is at least version 2.2.0.
	UseV2 bool
}

const natsV2QueryParameter = "natsv2"

// OpenTopicURL opens a pubsub.Topic based on u.
func (o *URLOpener) OpenTopicURL(ctx context.Context, u *url.URL) (*pubsub.Topic, error) {
	for param := range u.Query() {
		if strings.ToLower(param) == natsV2QueryParameter {
			continue
		}
		return nil, fmt.Errorf("open topic %v: invalid query parameter %s", u, param)
	}
	subject := path.Join(u.Host, u.Path)
	if o.UseV2 {
		return OpenTopicV2(o.Connection, subject, &o.TopicOptions)
	}
	return OpenTopic(o.Connection, subject, &o.TopicOptions)
}

// OpenSubscriptionURL opens a pubsub.Subscription based on u.
func (o *URLOpener) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*pubsub.Subscription, error) {
	opts := o.SubscriptionOptions
	for param, values := range u.Query() {
		switch strings.ToLower(param) {
		case natsV2QueryParameter:
			continue
		case "queue":
			if len(values) != 1 {
				return nil, fmt.Errorf("open subscription %v: invalid query parameter %s", u, param)
			}
			opts.Queue = values[0]
		default:
			return nil, fmt.Errorf("open subscription %v: invalid query parameter %s", u, param)
		}
	}
	subject := path.Join(u.Host, u.Path)
	if o.UseV2 {
		return OpenSubscriptionV2(o.Connection, subject, &opts)
	}
	return OpenSubscription(o.Connection, subject, &opts)
}

// TopicOptions sets options for constructing a *pubsub.Topic backed by NATS.
type TopicOptions struct{}

// SubscriptionOptions sets options for constructing a *pubsub.Subscription
// backed by NATS.
type SubscriptionOptions struct {
	// Queue sets the subscription as a QueueSubcription.
	// For more info, see https://docs.nats.io/nats-concepts/queue.
	Queue string
}

type topic struct {
	useV2 bool
	nc    *nats.Conn
	subj  string
}

// OpenTopic returns a *pubsub.Topic for use with NATS.
// The subject is the NATS Subject; for more info, see
// https://nats.io/documentation/writing_applications/subjects.
func OpenTopic(nc *nats.Conn, subject string, _ *TopicOptions) (*pubsub.Topic, error) {
	dt, err := openTopic(nc, subject, false)
	if err != nil {
		return nil, err
	}
	return pubsub.NewTopic(dt, nil), nil
}

// OpenTopicV2 returns a *pubsub.Topic for use with NATS at least version 2.2.0.
// This changes the encoding of the message as, starting with version 2.2.0, NATS supports message headers.
// In previous versions the message headers were encoded along with the message content using gob.Encoder,
// which limits the subscribers only to Go clients.
// This implementation uses native NATS message headers, and native message content, which provides full support
// for non-Go clients.
func OpenTopicV2(nc *nats.Conn, subject string, _ *TopicOptions) (*pubsub.Topic, error) {
	dt, err := openTopic(nc, subject, true)
	if err != nil {
		return nil, err
	}
	return pubsub.NewTopic(dt, nil), nil
}

// openTopic returns the driver for OpenTopic. This function exists so the test
// harness can get the driver interface implementation if it needs to.
func openTopic(nc *nats.Conn, subject string, useV2 bool) (driver.Topic, error) {
	if nc == nil {
		return nil, errors.New("natspubsub: nats.Conn is required")
	}
	if useV2 {
		sv, err := parseServerVersion(nc.ConnectedServerVersion())
		if err != nil {
			return nil, fmt.Errorf("failed to parse NATS server version %q: %v", nc.ConnectedServerVersion(), err)
		}
		// Check if the server version is at least 2.2.0.
		if sv.major < 2 && sv.minor < 2 {
			return nil, fmt.Errorf("natspubsub: NATS server version %q is not supported", nc.ConnectedServerVersion())
		}
	}
	return &topic{nc: nc, subj: subject, useV2: useV2}, nil
}

// SendBatch implements driver.Topic.SendBatch.
func (t *topic) SendBatch(ctx context.Context, msgs []*driver.Message) error {
	if t == nil || t.nc == nil {
		return errNotInitialized
	}

	for _, m := range msgs {
		err := ctx.Err()
		if err != nil {
			return err
		}

		if t.useV2 {
			err = t.sendMessageV2(m)
		} else {
			err = t.sendMessage(m)
		}
		if err != nil {
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

func (t *topic) sendMessage(m *driver.Message) error {
	// TODO(jba): benchmark message encoding to see if it's
	// worth reusing a buffer.
	payload, err := encodeMessage(m)
	if err != nil {
		return err
	}
	if m.BeforeSend != nil {
		asFunc := func(i any) bool { return false }
		if err := m.BeforeSend(asFunc); err != nil {
			return err
		}
	}
	if err = t.nc.Publish(t.subj, payload); err != nil {
		return err
	}
	if m.AfterSend != nil {
		asFunc := func(i any) bool { return false }
		if err := m.AfterSend(asFunc); err != nil {
			return err
		}
	}
	return nil
}

func (t *topic) sendMessageV2(m *driver.Message) error {
	msg := encodeMessageV2(m, t.subj)
	if m.BeforeSend != nil {
		asFunc := func(i any) bool {
			if nm, ok := i.(**nats.Msg); ok {
				*nm = msg
				return true
			}
			return false
		}
		if err := m.BeforeSend(asFunc); err != nil {
			return err
		}
	}

	if err := t.nc.PublishMsg(msg); err != nil {
		return err
	}

	if m.AfterSend != nil {
		asFunc := func(i any) bool { return false }
		if err := m.AfterSend(asFunc); err != nil {
			return err
		}
	}
	return nil
}

// IsRetryable implements driver.Topic.IsRetryable.
func (*topic) IsRetryable(error) bool { return false }

// As implements driver.Topic.As.
func (t *topic) As(i any) bool {
	c, ok := i.(**nats.Conn)
	if !ok {
		return false
	}
	*c = t.nc
	return true
}

// ErrorAs implements driver.Topic.ErrorAs
func (*topic) ErrorAs(error, any) bool {
	return false
}

// ErrorCode implements driver.Topic.ErrorCode
func (*topic) ErrorCode(err error) gcerrors.ErrorCode {
	switch err {
	case nil:
		return gcerrors.OK
	case context.Canceled:
		return gcerrors.Canceled
	case errNotInitialized:
		return gcerrors.NotFound
	case nats.ErrBadSubject:
		return gcerrors.FailedPrecondition
	case nats.ErrAuthorization:
		return gcerrors.PermissionDenied
	case nats.ErrMaxPayload, nats.ErrReconnectBufExceeded:
		return gcerrors.ResourceExhausted
	}
	return gcerrors.Unknown
}

// Close implements driver.Topic.Close.
func (*topic) Close() error { return nil }

type subscription struct {
	useV2  bool
	nc     *nats.Conn
	nsub   *nats.Subscription
	nextID int
}

// OpenSubscription returns a *pubsub.Subscription representing a NATS subscription or NATS queue subscription.
// The subject is the NATS Subject to subscribe to;
// for more info, see https://nats.io/documentation/writing_applications/subjects.
func OpenSubscription(nc *nats.Conn, subject string, opts *SubscriptionOptions) (*pubsub.Subscription, error) {
	ds, err := openSubscription(nc, subject, opts, false)
	if err != nil {
		return nil, err
	}
	return pubsub.NewSubscription(ds, recvBatcherOpts, nil), nil
}

// OpenSubscriptionV2 returns a *pubsub.Subscription representing a NATS subscription or NATS queue subscription
// for use with NATS at least version 2.2.0.
// This changes the encoding of the message as, starting with version 2.2.0, NATS supports message headers.
// In previous versions the message headers were encoded along with the message content using gob.Encoder,
// which limits the subscribers only to Go clients.
// This implementation uses native NATS message headers, and native message content, which provides full support
// for non-Go clients.
func OpenSubscriptionV2(nc *nats.Conn, subject string, opts *SubscriptionOptions) (*pubsub.Subscription, error) {
	ds, err := openSubscription(nc, subject, opts, true)
	if err != nil {
		return nil, err
	}
	return pubsub.NewSubscription(ds, recvBatcherOpts, nil), nil
}

func openSubscription(nc *nats.Conn, subject string, opts *SubscriptionOptions, useV2 bool) (driver.Subscription, error) {
	var sub *nats.Subscription
	var err error
	if opts != nil && opts.Queue != "" {
		sub, err = nc.QueueSubscribeSync(subject, opts.Queue)
	} else {
		sub, err = nc.SubscribeSync(subject)
	}
	if err != nil {
		return nil, err
	}
	return &subscription{nc: nc, nsub: sub, nextID: 1, useV2: useV2}, nil
}

// ReceiveBatch implements driver.ReceiveBatch.
func (s *subscription) ReceiveBatch(ctx context.Context, _ int) ([]*driver.Message, error) {
	if s == nil || s.nsub == nil {
		return nil, nats.ErrBadSubscription
	}

	msg, err := s.nsub.NextMsg(100 * time.Millisecond)
	if err != nil {
		if err == nats.ErrTimeout {
			return nil, nil
		}
		return nil, err
	}

	var dm *driver.Message
	if s.useV2 {
		dm, err = decodeMessageV2(msg)
	} else {
		dm, err = decode(msg)
	}
	if err != nil {
		return nil, err
	}
	dm.LoggableID = fmt.Sprintf("msg #%d", s.nextID)
	s.nextID++
	return []*driver.Message{dm}, nil
}

// Convert NATS msgs to *driver.Message.
func decode(msg *nats.Msg) (*driver.Message, error) {
	if msg == nil {
		return nil, nats.ErrInvalidMsg
	}
	var dm driver.Message
	if err := decodeMessage(msg.Data, &dm); err != nil {
		return nil, err
	}
	dm.AckID = -1 // Not applicable to NATS
	dm.AsFunc = messageAsFunc(msg)
	return &dm, nil
}

func messageAsFunc(msg *nats.Msg) func(any) bool {
	return func(i any) bool {
		p, ok := i.(**nats.Msg)
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
func (s *subscription) As(i any) bool {
	c, ok := i.(**nats.Subscription)
	if !ok {
		return false
	}
	*c = s.nsub
	return true
}

// ErrorAs implements driver.Subscription.ErrorAs
func (*subscription) ErrorAs(error, any) bool {
	return false
}

// ErrorCode implements driver.Subscription.ErrorCode
func (*subscription) ErrorCode(err error) gcerrors.ErrorCode {
	switch err {
	case nil:
		return gcerrors.OK
	case context.Canceled:
		return gcerrors.Canceled
	case errNotInitialized, nats.ErrBadSubscription:
		return gcerrors.NotFound
	case nats.ErrBadSubject, nats.ErrTypeSubscription:
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

func queryUseV2(q url.Values) (bool, error) {
	if len(q) == 0 {
		return false, nil
	}
	v, ok := q[natsV2QueryParameter]
	if !ok {
		return false, nil
	}

	if len(v) == 0 {
		// If the query parameter was provided without any value i.e. nats://mysubject?natsv2
		// it assumes the value is true.
		return true, nil
	}
	if len(v) > 1 {
		return false, fmt.Errorf("invalid query parameter %s - multiple values provided", natsV2QueryParameter)
	}
	if v[0] == "" {
		return true, nil
	}
	useV2, err := strconv.ParseBool(v[0])
	if err != nil {
		return false, fmt.Errorf("invalid query parameter %s - value either needs to be parsable as a boolean or empty", natsV2QueryParameter)
	}
	return useV2, nil
}

func encodeMessageV2(dm *driver.Message, sub string) *nats.Msg {
	var header nats.Header
	if dm.Metadata != nil {
		header = nats.Header{}
		for k, v := range dm.Metadata {
			header[url.QueryEscape(k)] = []string{url.QueryEscape(v)}
		}
	}
	return &nats.Msg{
		Subject: sub,
		Data:    dm.Body,
		Header:  header,
	}
}

func decodeMessageV2(msg *nats.Msg) (*driver.Message, error) {
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
	return &dm, nil
}
