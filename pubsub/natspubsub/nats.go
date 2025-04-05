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

//
// natspubsub exposes the following types for use:
//   - Connection: *nats.Conn
//   - Subscription: *nats.Subscription
//   - Message.BeforeSend: *nats.Msg for v2.
//   - Message.AfterSend: None.
//   - Message: *nats.Msg
//
//	This implementation does not support nats version 1.0, actually from nats v2.2 onwards only.
//
//

package natspubsub

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"gocloud.dev/pubsub/natspubsub/connections"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"

	"gocloud.dev/gcerrors"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/batcher"
	"gocloud.dev/pubsub/driver"
)

const (
	natsV1QueryParameter    = "nats_v1"
	jetstreamQueryParameter = "jetstream"
)

var allowedParameters = []string{natsV1QueryParameter, jetstreamQueryParameter, "subject", "stream_name", "stream_description", "stream_subjects",
	"consumer_max_count", "consumer_request_batch", "consumer_request_max_batch_bytes", "consumer_durable",
	"consumer_request_timeout_ms", "consumer_max_request_expires_ms", "consumer_max_waiting", "consumer_ack_wait_timeout_ms",
	"consumer_max_ack_pending"}

var errInvalidUrl = errors.New("natspubsub: invalid connection url")
var errNotSubjectInitialized = errors.New("natspubsub: subject not initialized")
var errDuplicateParameter = errors.New("natspubsub: avoid specifying parameters more than once")
var errNotSupportedParameter = fmt.Errorf("natspubsub: invalid parameter used, "+
	"only the parameters [ %s ] are supported and can be used",
	strings.Join(allowedParameters, ", "))

func init() {
	o := new(defaultDialer)
	pubsub.DefaultURLMux().RegisterTopic(Scheme, o)
	pubsub.DefaultURLMux().RegisterSubscription(Scheme, o)
}

// defaultDialer dials a NATS server based on the provided url
// see: https://docs.nats.io/using-nats/developer/connecting
// Guidance :
//   - This dialer will only use the url formart nats://...
//   - The dialer stores a map of unique nats connections without the parameters
type defaultDialer struct {
	mutex sync.Mutex

	openerMap sync.Map
}

// defaultConn
func (o *defaultDialer) defaultConn(_ context.Context, serverUrl *url.URL) (*URLOpener, error) {

	o.mutex.Lock()
	defer o.mutex.Unlock()

	connectionUrl := strings.Replace(serverUrl.String(), serverUrl.RequestURI(), "", 1)

	for param, values := range serverUrl.Query() {
		paramName := strings.ToLower(param)
		if !slices.Contains(allowedParameters, paramName) {
			return nil, errNotSupportedParameter
		}

		if len(values) != 1 {
			return nil, errDuplicateParameter
		}

	}

	storedOpener, ok := o.openerMap.Load(connectionUrl)
	if ok {
		return storedOpener.(*URLOpener), nil
	}

	useV1Encoding := o.featureIsEnabledViaUrl(serverUrl.Query(), natsV1QueryParameter)
	isJetstreamEnabled := o.featureIsEnabledViaUrl(serverUrl.Query(), jetstreamQueryParameter)

	conn, err := o.createConnection(connectionUrl, isJetstreamEnabled, useV1Encoding)
	if err != nil {
		return nil, err
	}

	opener := &URLOpener{
		Connection:          conn,
		TopicOptions:        connections.TopicOptions{},
		SubscriptionOptions: connections.SubscriptionOptions{},
	}

	o.openerMap.Store(connectionUrl, opener)

	return opener, nil
}

func (o *defaultDialer) createConnection(connectionUrl string, isJetstreamEnabled bool, useV1Encoding bool) (connections.Connection, error) {
	natsConn, err := nats.Connect(connectionUrl)
	if err != nil {
		return nil, fmt.Errorf("natspubsub: failed to dial server using %q: %v", connectionUrl, err)
	}

	sv, err := connections.ServerVersion(natsConn.ConnectedServerVersion())
	if err != nil {
		return nil, fmt.Errorf("failed to parse NATS server version %q: %v", natsConn.ConnectedServerVersion(), err)
	}

	var conn connections.Connection
	if !sv.JetstreamSupported() || !isJetstreamEnabled {
		return connections.NewPlainWithEncodingV1(natsConn, useV1Encoding)
	}

	var js jetstream.JetStream
	js, err = jetstream.New(natsConn)
	if err != nil {
		return nil, fmt.Errorf("natspubsub: failed to convert server to jetstream : %v", err)
	}

	conn = connections.NewJetstream(js)
	return conn, nil

}

func (o *defaultDialer) featureIsEnabledViaUrl(q url.Values, key string) bool {
	if len(q) == 0 {
		return false
	}
	v, ok := q[key]
	if !ok {
		return false
	}

	if len(v) == 0 {
		// If the query parameter was provided without any value i.e. nats://mysubject?natsv2
		// it assumes the value is true.
		return true
	}
	if len(v) > 1 {
		return false
	}
	if v[0] == "" {
		return true
	}
	useV2, err := strconv.ParseBool(v[0])
	if err != nil {
		return false
	}
	return useV2
}

func (o *defaultDialer) OpenTopicURL(ctx context.Context, u *url.URL) (*pubsub.Topic, error) {
	opener, err := o.defaultConn(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("open topic %v: failed to open default connection: %v", u, err)
	}
	return opener.OpenTopicURL(ctx, u)
}

func (o *defaultDialer) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*pubsub.Subscription, error) {
	opener, err := o.defaultConn(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("open subscription %v: failed to open default connection: %v", u, err)
	}
	return opener.OpenSubscriptionURL(ctx, u)
}

// Scheme is the URL scheme natspubsub registers its URLOpeners under on pubsub.DefaultMux.
const Scheme = "nats"

// URLOpener opens NATS URLs like "nats://mysubject".
//
// The URL host+path is used as the subject.
//
// No query parameters are supported.
type URLOpener struct {
	Connection connections.Connection
	// TopicOptions specifies the options to pass to OpenTopic.
	TopicOptions connections.TopicOptions
	// SubscriptionOptions specifies the options to pass to OpenSubscription.
	SubscriptionOptions connections.SubscriptionOptions
}

func cleanSubjectFromUrl(u *url.URL) (string, error) {
	subject := u.Query().Get("subject")

	// Clean the leading slash from the path
	pathPart := strings.TrimPrefix(u.Path, "/")

	if pathPart != "" {
		if subject == "" {
			subject = pathPart
		} else {
			subject += "." + pathPart
		}
	}

	if subject == "" {
		return "", errNotSubjectInitialized
	}

	return subject, nil
}

// OpenTopicURL opens a pubsub.Topic based on a url supplied.
//
//	A topic can be specified in the subject and suffixed by the url path
//	These definitions will yield the subject shown infront of them
//
//		- nats://host:8934?subject=foo --> foo
//		- nats://host:8934/bar?subject=foo --> foo/bar
//		- nats://host:8934/bar --> /bar
//		- nats://host:8934?no_subject=foo --> [this yields an error]
func (o *URLOpener) OpenTopicURL(ctx context.Context, u *url.URL) (*pubsub.Topic, error) {

	opts := &o.TopicOptions

	var err error
	opts.Subject, err = cleanSubjectFromUrl(u)
	if err != nil {
		return nil, err
	}

	return OpenTopic(ctx, o.Connection, opts)

}

// OpenSubscriptionURL opens a pubsub.Subscription based on url supplied.
//
//	 A subscription also creates the required underlaying queue or streams
//	 There are many more parameters checked in this case compared to the publish topic section.
//	 If required, the list of parameters can be extended, but for now only a subset is defined and
//	 the remaining ones utilize the sensible defaults that nats come with.
//
//		The list of parameters includes:
//
//			- subject,
//			- stream_name,
//			- stream_description,
//			- stream_subjects,
//			- consumer_max_count,
//			- consumer_queue
//			- consumer_max_waiting

func (o *URLOpener) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*pubsub.Subscription, error) {

	opts := &o.SubscriptionOptions

	subject, err := cleanSubjectFromUrl(u)
	if err != nil {
		return nil, err
	}

	opts.Subjects = []string{subject}
	opts.Durable = u.Query().Get("consumer_durable")

	opts.ConsumersMaxCount, err = strconv.Atoi(u.Query().Get("consumer_max_count"))
	if err != nil {
		opts.ConsumersMaxCount = 10
	}
	opts.ConsumerRequestBatch, err = strconv.Atoi(u.Query().Get("consumer_request_batch"))
	if err != nil {
		opts.ConsumerRequestBatch = 50
	}
	opts.ConsumerRequestMaxBatchBytes, err = strconv.Atoi(u.Query().Get("consumer_request_max_batch_bytes"))
	if err != nil {
		opts.ConsumerRequestMaxBatchBytes = 0
	}

	opts.ConsumerRequestTimeoutMs, err = strconv.Atoi(u.Query().Get("consumer_request_timeout_ms"))
	if err != nil {
		opts.ConsumerRequestTimeoutMs = 1000
	}

	opts.ConsumerMaxRequestExpiresMs, err = strconv.Atoi(u.Query().Get("consumer_max_request_expires_ms"))
	if err != nil {
		opts.ConsumerMaxRequestExpiresMs = 30000
	}

	opts.ConsumerMaxWaiting, err = strconv.Atoi(u.Query().Get("consumer_max_waiting"))
	if err != nil {
		opts.ConsumerMaxWaiting = 100
	}

	opts.ConsumerAckWaitTimeoutMs, err = strconv.Atoi(u.Query().Get("consumer_ack_wait_timeout_ms"))
	if err != nil {
		opts.ConsumerAckWaitTimeoutMs = 300000
	}
	opts.ConsumerMaxAckPending, err = strconv.Atoi(u.Query().Get("consumer_max_ack_pending"))
	if err != nil {
		opts.ConsumerMaxAckPending = 100
	}

	opts.StreamName = u.Query().Get("stream_name")
	opts.StreamDescription = u.Query().Get("stream_description")
	streamSubject := u.Query().Get("stream_subject")
	if streamSubject != "" {
		opts.Subjects = append(opts.Subjects, strings.Split(streamSubject, ",")...)
	}

	return OpenSubscription(ctx, o.Connection, opts)

}

type topic struct {
	iTopic connections.Topic
}

// OpenTopic returns a *pubsub.Topic for use with NATS at least version 2.2.0.
// This changes the encoding of the message as, starting with version 2.2.0, NATS supports message headers.
// In previous versions the message headers were encoded along with the message content using gob.Encoder,
// which limits the subscribers only to Go clients.
// This implementation uses native NATS message headers, and native message content, which provides full support
// for non-Go clients.
func OpenTopic(ctx context.Context, conn connections.Connection, opts *connections.TopicOptions) (*pubsub.Topic, error) {
	dt, err := openTopic(ctx, conn, opts)
	if err != nil {
		return nil, err
	}
	return pubsub.NewTopic(dt, nil), nil
}

// openTopic returns the driver for OpenTopic. This function exists so the test
// harness can get the driver interface implementation if it needs to.
func openTopic(ctx context.Context, conn connections.Connection, opts *connections.TopicOptions) (driver.Topic, error) {
	if conn == nil {
		return nil, errInvalidUrl
	}

	itopic, err := conn.CreateTopic(ctx, opts)
	if err != nil {
		return nil, err
	}

	return &topic{iTopic: itopic}, nil
}

// SendBatch implements driver.Connection.SendBatch.
func (t *topic) SendBatch(ctx context.Context, msgs []*driver.Message) error {
	if t == nil || t.iTopic == nil {
		return errNotSubjectInitialized
	}

	for _, m := range msgs {
		err := ctx.Err()
		if err != nil {
			return err
		}

		err = t.sendMessage(ctx, m)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *topic) sendMessage(ctx context.Context, m *driver.Message) error {
	var msg *nats.Msg
	var err error
	if t.iTopic.UseV1Encoding() {
		msg, err = encodeV1Message(m, t.iTopic.Subject())
		if err != nil {
			return err
		}
	} else {
		msg = encodeMessage(m, t.iTopic.Subject())
	}

	if m.BeforeSend != nil {
		asFunc := func(i interface{}) bool {
			if nm, ok := i.(**nats.Msg); ok {
				*nm = msg
				return true
			}
			return false
		}
		if err = m.BeforeSend(asFunc); err != nil {
			return err
		}
	}

	_, err = t.iTopic.PublishMessage(ctx, msg)
	if err != nil {
		return err
	}

	if m.AfterSend != nil {
		asFunc := func(i interface{}) bool { return false }
		err = m.AfterSend(asFunc)
		if err != nil {
			return err
		}
	}
	return nil
}

// IsRetryable implements driver.Connection.IsRetryable.
func (*topic) IsRetryable(error) bool { return false }

// As implements driver.Connection.As.
func (t *topic) As(i interface{}) bool {
	c, ok := i.(*connections.Topic)
	if !ok {
		return false
	}
	*c = t.iTopic
	return true
}

// ErrorAs implements driver.Connection.ErrorAs
func (*topic) ErrorAs(error, interface{}) bool {
	return false
}

// ErrorCode implements driver.Connection.ErrorCode
func (*topic) ErrorCode(err error) gcerrors.ErrorCode {
	switch {
	case err == nil:
		return gcerrors.OK
	case errors.Is(err, context.Canceled):
		return gcerrors.Canceled
	case errors.Is(err, errNotSubjectInitialized):
		return gcerrors.NotFound
	case errors.Is(err, nats.ErrBadSubject):
		return gcerrors.FailedPrecondition
	case errors.Is(err, nats.ErrAuthorization):
		return gcerrors.PermissionDenied
	case errors.Is(err, nats.ErrMaxPayload), errors.Is(err, nats.ErrReconnectBufExceeded):
		return gcerrors.ResourceExhausted
	}
	return gcerrors.Unknown
}

// Close implements driver.Connection.Close.
func (*topic) Close() error { return nil }

type subscription struct {
	queue connections.Queue
}

// OpenSubscription returns a *pubsub.Subscription representing a NATS subscription
// or NATS queue subscription for use with NATS at least version 2.2.0.
// This changes the encoding of the message as, starting with version 2.2.0, NATS supports message headers.
// In previous versions the message headers were encoded along with the message content using gob.Encoder,
// which limits the subscribers only to Go clients.
// This implementation uses native NATS message headers, and native message content, which provides full support
// for non-Go clients.
func OpenSubscription(ctx context.Context, conn connections.Connection, opts *connections.SubscriptionOptions) (*pubsub.Subscription, error) {
	ds, err := openSubscription(ctx, conn, opts)
	if err != nil {
		return nil, err
	}

	maxConsumerCount := opts.ConsumersMaxCount
	if maxConsumerCount <= 0 {
		maxConsumerCount = 1
	}

	maxBatchSize := opts.ConsumerRequestBatch
	if maxBatchSize <= 0 {
		maxBatchSize = 1
	}

	maxBatchBytesSize := opts.ConsumerRequestMaxBatchBytes

	var recvBatcherOpts = &batcher.Options{
		MaxHandlers:      maxConsumerCount, // max concurrency for receives
		MaxBatchSize:     maxBatchSize,
		MaxBatchByteSize: maxBatchBytesSize,
	}

	return pubsub.NewSubscription(ds, recvBatcherOpts, nil), nil
}

func openSubscription(ctx context.Context, conn connections.Connection, opts *connections.SubscriptionOptions) (driver.Subscription, error) {
	if opts == nil {
		return nil, errors.New("natspubsub: subscription options missing")
	}

	queue, err := conn.CreateSubscription(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &subscription{queue: queue}, nil
}

// ReceiveBatch implements driver.ReceiveBatch.
func (s *subscription) ReceiveBatch(ctx context.Context, batchCount int) ([]*driver.Message, error) {

	if s == nil || s.queue == nil {
		return nil, nats.ErrBadSubscription
	}

	return s.queue.ReceiveMessages(ctx, batchCount)
}

// SendAcks implements driver.Subscription.SendAcks.
func (s *subscription) SendAcks(ctx context.Context, ids []driver.AckID) error {
	// Ack is a no-op.
	return s.queue.Ack(ctx, ids)
}

// CanNack implements driver.CanNack.
func (s *subscription) CanNack() bool { return s != nil && s.queue != nil && s.queue.IsQueueGroup() }

// SendNacks implements driver.Subscription.SendNacks
func (s *subscription) SendNacks(ctx context.Context, ids []driver.AckID) error {
	return s.queue.Nack(ctx, ids)
}

// IsRetryable implements driver.Subscription.IsRetryable.
func (s *subscription) IsRetryable(error) bool { return false }

// As implements driver.Subscription.As.
func (s *subscription) As(i interface{}) bool {

	if p, ok := i.(*connections.Queue); ok {
		*p = s.queue
		return true
	}

	return false

}

// ErrorAs implements driver.Subscription.ErrorAs
func (*subscription) ErrorAs(error, interface{}) bool {
	return false
}

// ErrorCode implements driver.Subscription.ErrorCode
func (*subscription) ErrorCode(err error) gcerrors.ErrorCode {
	switch {
	case err == nil:
		return gcerrors.OK
	case errors.Is(err, context.Canceled):
		return gcerrors.Canceled
	case errors.Is(err, errNotSubjectInitialized), errors.Is(err, nats.ErrBadSubscription):
		return gcerrors.NotFound
	case errors.Is(err, nats.ErrBadSubject), errors.Is(err, nats.ErrTypeSubscription):
		return gcerrors.FailedPrecondition
	case errors.Is(err, nats.ErrAuthorization):
		return gcerrors.PermissionDenied
	case errors.Is(err, nats.ErrMaxMessages), errors.Is(err, nats.ErrSlowConsumer):
		return gcerrors.ResourceExhausted
	case errors.Is(err, nats.ErrTimeout):
		return gcerrors.DeadlineExceeded
	}
	return gcerrors.Unknown
}

// Close implements driver.Subscription.Close.
func (*subscription) Close() error { return nil }

func encodeV1Message(dm *driver.Message, sub string) (*nats.Msg, error) {

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	// Always encode metadata, even if empty - this ensures consistent message format
	if err := enc.Encode(dm.Metadata); err != nil {
		return nil, err
	}
	if err := enc.Encode(dm.Body); err != nil {
		return nil, err
	}
	return &nats.Msg{
		Subject: sub,
		Data:    buf.Bytes(),
	}, nil

}

func encodeMessage(dm *driver.Message, sub string) *nats.Msg {
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
