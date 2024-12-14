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

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"gocloud.dev/gcerrors"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
)

func init() {
	o := new(defaultDialer)
	pubsub.DefaultURLMux().RegisterTopic(Scheme, o)
	pubsub.DefaultURLMux().RegisterSubscription(Scheme, o)
}

// defaultDialer dials a default Rabbit server based on the environment
// variable "RABBIT_SERVER_URL".
type defaultDialer struct {
	mu     sync.Mutex
	conn   *amqp.Connection
	opener *URLOpener
}

func (o *defaultDialer) defaultConn(ctx context.Context) (*URLOpener, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	// Re-use the connection if possible.
	if o.opener != nil && o.conn != nil && !o.conn.IsClosed() {
		return o.opener, nil
	}

	// First time through, or last time resulted in an error, or connection
	// was closed. Initialize the connection.
	serverURL := os.Getenv("RABBIT_SERVER_URL")
	if serverURL == "" {
		return nil, errors.New("RABBIT_SERVER_URL environment variable not set")
	}
	conn, err := amqp.Dial(serverURL)
	if err != nil {
		return nil, fmt.Errorf("failed to dial RABBIT_SERVER_URL %q: %w", serverURL, err)
	}
	o.conn = conn
	o.opener = &URLOpener{Connection: conn}
	return o.opener, nil
}

func (o *defaultDialer) OpenTopicURL(ctx context.Context, u *url.URL) (*pubsub.Topic, error) {
	opener, err := o.defaultConn(ctx)
	if err != nil {
		return nil, fmt.Errorf("open topic %v: failed to open default connection: %w", u, err)
	}
	return opener.OpenTopicURL(ctx, u)
}

func (o *defaultDialer) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*pubsub.Subscription, error) {
	opener, err := o.defaultConn(ctx)
	if err != nil {
		return nil, fmt.Errorf("open subscription %v: failed to open default connection: %w", u, err)
	}
	return opener.OpenSubscriptionURL(ctx, u)
}

// Scheme is the URL scheme rabbitpubsub registers its URLOpeners under on pubsub.DefaultMux.
const Scheme = "rabbit"

// URLOpener opens RabbitMQ URLs like "rabbit://myexchange" for
// topics or "rabbit://myqueue" for subscriptions.
//
// For topics, the URL's host+path is used as the exchange name.
//
// For subscriptions, the URL's host+path is used as the queue name.
//
// An optional query string can be used to set the Qos consumer prefetch on subscriptions
// like "rabbit://myqueue?prefetch_count=1000" to set the consumer prefetch count to 1000
// see also https://www.rabbitmq.com/docs/consumer-prefetch
type URLOpener struct {
	// Connection to use for communication with the server.
	Connection *amqp.Connection

	// TopicOptions specifies the options to pass to OpenTopic.
	TopicOptions TopicOptions
	// SubscriptionOptions specifies the options to pass to OpenSubscription.
	SubscriptionOptions SubscriptionOptions
}

// OpenTopicURL opens a pubsub.Topic based on u.
func (o *URLOpener) OpenTopicURL(ctx context.Context, u *url.URL) (*pubsub.Topic, error) {
	opts := o.TopicOptions
	for param, value := range u.Query() {
		switch param {
		case "key_name":
			if len(value) != 1 || len(value[0]) == 0 {
				return nil, fmt.Errorf("open topic %v: invalid query parameter %q", u, param)
			}

			opts.KeyName = value[0]
		default:
			return nil, fmt.Errorf("open topic %v: invalid query parameter %q", u, param)
		}
	}

	exchangeName := path.Join(u.Host, u.Path)
	return OpenTopic(o.Connection, exchangeName, &opts), nil
}

// OpenSubscriptionURL opens a pubsub.Subscription based on u.
func (o *URLOpener) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*pubsub.Subscription, error) {
	opts := o.SubscriptionOptions
	for param, value := range u.Query() {
		switch param {
		case "prefetch_count":
			if len(value) != 1 || len(value[0]) == 0 {
				return nil, fmt.Errorf("open subscription %v: invalid query parameter %q", u, param)
			}

			prefetchCount, err := strconv.Atoi(value[0])
			if err != nil {
				return nil, fmt.Errorf("open subscription %v: invalid query parameter %q: %w", u, param, err)
			}

			opts.PrefetchCount = &prefetchCount
		default:
			return nil, fmt.Errorf("open subscription %v: invalid query parameter %q", u, param)
		}
	}

	queueName := path.Join(u.Host, u.Path)
	return OpenSubscription(o.Connection, queueName, &opts), nil
}

type topic struct {
	exchange string // the AMQP exchange
	conn     amqpConnection
	opts     *TopicOptions

	mu     sync.Mutex
	ch     amqpChannel              // AMQP channel used for all communication.
	pubc   <-chan amqp.Confirmation // Go channel for server acks of publishes
	retc   <-chan amqp.Return       // Go channel for "returned" undeliverable messages
	closec <-chan *amqp.Error       // Go channel for AMQP channel close notifications
}

// TopicOptions sets options for constructing a *pubsub.Topic backed by
// RabbitMQ.
type TopicOptions struct {
	// KeyName optionally sets the Message.Metadata key to use as the optional
	// RabbitMQ message key. If set, and if a matching Message.Metadata key is found,
	// the value for that key will be used as the routing key when sending to
	// RabbitMQ, instead of being added to the message headers.
	KeyName string
}

// SubscriptionOptions sets options for constructing a *pubsub.Subscription
// backed by RabbitMQ.
type SubscriptionOptions struct {
	// KeyName optionally sets the Message.Metadata key in which to store the
	// RabbitMQ message key. If set, and if the RabbitMQ message key is non-empty,
	// the key value will be stored in Message.Metadata under KeyName.
	KeyName string

	// Qos property prefetch count. Optional.
	PrefetchCount *int
}

// OpenTopic returns a *pubsub.Topic corresponding to the named exchange.
// See the package documentation for an example.
//
// The exchange should already exist (for instance, by using
// amqp.Channel.ExchangeDeclare), although this won't be checked until the first call
// to SendBatch. For the Go CDK Pub/Sub model to make sense, the exchange should
// be a fanout exchange, although nothing in this package enforces that.
//
// OpenTopic uses the supplied amqp.Connection for all communication. It is the
// caller's responsibility to establish this connection before calling OpenTopic, and
// to close it when Close has been called on all Topics opened with it.
//
// The documentation of the amqp package recommends using separate connections for
// publishing and subscribing.
func OpenTopic(conn *amqp.Connection, name string, opts *TopicOptions) *pubsub.Topic {
	return pubsub.NewTopic(newTopic(&connection{conn}, name, opts), nil)
}

func newTopic(conn amqpConnection, name string, opts *TopicOptions) *topic {
	if opts == nil {
		opts = &TopicOptions{}
	}

	return &topic{
		conn:     conn,
		exchange: name,
		opts:     opts,
	}
}

// establishChannel creates an AMQP channel if necessary. According to the amqp
// package docs, once an error is returned from the channel, it must be discarded and
// a new one created.
//
// Must be called with t.mu held.
func (t *topic) establishChannel(ctx context.Context) error {
	if t.ch != nil { // We already have a channel.
		select {
		// If it was closed, open a new one.
		// (Ignore the error, if any.)
		case <-t.closec:

		// If it isn't closed, nothing to do.
		default:
			return nil
		}
	}
	var ch amqpChannel
	err := runWithContext(ctx, func() error {
		// Create a new channel in confirm mode.
		var err error
		ch, err = t.conn.Channel()
		return err
	})
	if err != nil {
		return err
	}
	t.ch = ch
	// Get Go channels which will hold acks and returns from the server. The server
	// will send an ack for each published message to confirm that it was received.
	// It will return undeliverable messages.
	// All the Notify methods return their arg.
	t.pubc = ch.NotifyPublish(make(chan amqp.Confirmation))
	t.retc = ch.NotifyReturn(make(chan amqp.Return))
	t.closec = ch.NotifyClose(make(chan *amqp.Error, 1)) // closec will get at most one element
	return nil
}

// Run f while checking to see if ctx is done.
// Return the error from f if it completes, or ctx.Err() if ctx is done.
func runWithContext(ctx context.Context, f func() error) error {
	c := make(chan error, 1) // buffer so the goroutine can finish even if ctx is done
	go func() { c <- f() }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-c:
		return err
	}
}

// SendBatch implements driver.SendBatch.
func (t *topic) SendBatch(ctx context.Context, ms []*driver.Message) error {
	// It is simplest to allow only one SendBatch at a time. Allowing concurrent
	// calls to SendBatch would complicate the logic of receiving publish
	// confirmations and returns. We can go that route if performance warrants it.
	t.mu.Lock()
	defer t.mu.Unlock()

	if err := t.establishChannel(ctx); err != nil {
		return err
	}

	// Receive from Go channels concurrently or we will deadlock with the Publish
	// RPC. (The amqp package docs recommend setting the capacity of the Go channel
	// to the number of messages to be published, but we can't do that because we
	// want to reuse the channel for all calls to SendBatch--it takes two RPCs to set
	// up.)
	errc := make(chan error, 1)
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch := t.ch // Avoid touching t.ch while goroutine is running.
	go func() {
		// This goroutine runs with t.mu held because its lifetime is within the
		// lifetime of the t.mu.Lock call at the start of SendBatch.
		errc <- t.receiveFromPublishChannels(cctx, len(ms))
	}()

	var perr error
	for _, m := range ms {
		routingKey, pub := toRoutingKeyAndAMQPPublishing(m, t.opts)
		if m.BeforeSend != nil {
			asFunc := func(i any) bool {
				if p, ok := i.(**amqp.Publishing); ok {
					*p = &pub
					return true
				}
				return false
			}
			if err := m.BeforeSend(asFunc); err != nil {
				return err
			}
		}
		if perr = ch.Publish(t.exchange, routingKey, pub); perr != nil {
			cancel()
			break
		}
		if m.AfterSend != nil {
			asFunc := func(i any) bool { return false }
			if err := m.AfterSend(asFunc); err != nil {
				return err
			}
		}
	}
	// Wait for the goroutine to finish.
	err := <-errc
	// If we got an error from Publish, prefer that.
	if perr != nil {
		// Set t.ch to nil because an AMQP channel is broken after error.
		// Do this here, after the goroutine has finished, rather than in the Publish loop
		// above, to avoid a race condition.
		t.ch = nil
		err = perr
	}
	// If there is only one error, return it rather than a MultiError. That
	// will work better with ErrorCode and ErrorAs.
	var merr MultiError
	if errors.As(err, &merr) && len(merr) == 1 {
		return merr[0]
	}
	return err
}

// Read from the channels established with NotifyPublish and NotifyReturn.
// Must be called with t.mu held.
func (t *topic) receiveFromPublishChannels(ctx context.Context, nMessages int) error {
	// Consume all the acknowledgments for the messages we are publishing, and also
	// get returned messages. The server will send exactly one ack for each published
	// message (successful or not), and one return for each undeliverable message.
	// Since SendBatch (the only caller of this method) holds the lock, we expect
	// exactly as many acks as messages.
	var merr MultiError
	nAcks := 0
	for nAcks < nMessages {
		select {
		case <-ctx.Done():
			if t.ch != nil {
				// Channel will be in a weird state (not all publish acks consumed, perhaps)
				// so re-create it next time.
				t.ch.Close()
				t.ch = nil
			}
			return ctx.Err()

		case ret, ok := <-t.retc:
			if !ok {
				// Channel closed. Handled in the pubc case below. But set
				// the channel to nil to prevent it from being selected again.
				t.retc = nil
			} else {
				// The message was returned from the server because it is unroutable.
				// Record the error and continue so we drain all
				// items from pubc. We don't need to re-establish the channel on this
				// error.
				merr = append(merr, fmt.Errorf("rabbitpubsub: message returned from %s: %s (code %d)",
					ret.Exchange, ret.ReplyText, ret.ReplyCode))
			}

		case conf, ok := <-t.pubc:
			if !ok {
				// t.pubc was closed unexpectedly.
				t.ch = nil // re-create the channel on next use
				if merr != nil {
					return merr
				}
				// t.closec must be closed too. See if it has an error.
				if err := closeErr(t.closec); err != nil {
					merr = append(merr, err)
					return merr
				}
				// We shouldn't be here, but if we are, we still want to return an
				// error.
				merr = append(merr, errors.New("rabbitpubsub: publish listener closed unexpectedly"))
				return merr
			}
			nAcks++
			if !conf.Ack {
				merr = append(merr, errors.New("rabbitpubsub: ack failed on publish"))
			}
		}
	}
	if merr != nil {
		return merr
	}
	// Returning a nil merr would mean the returned error interface value is non-nil, so return nil explicitly.
	return nil
}

// A MultiError is an error that contains multiple errors.
type MultiError []error

func (m MultiError) Error() string {
	var s []string
	for _, e := range m {
		s = append(s, e.Error())
	}
	return strings.Join(s, "; ")
}

// Return the error from a Go channel monitoring the closing of an AMQP channel.
// closec must have been registered via Channel.NotifyClose.
// When closeErr is called, we expect closec to be closed. If it isn't, we also
// consider that an error.
func closeErr(closec <-chan *amqp.Error) error {
	select {
	case aerr := <-closec:
		// This nil check is necessary. aerr is of type *amqp.Error. If we
		// returned it directly (effectively assigning it to a variable of
		// type error), then the return value would not be a nil interface
		// value even if aerr was a nil pointer, and that would break tests
		// like "if err == nil ...".
		if aerr == nil {
			return nil
		}
		return aerr
	default:
		return errors.New("rabbitpubsub: NotifyClose Go channel is unexpectedly open")
	}
}

// toRoutingKeyAndAMQPPublishing converts a driver.Message to a pair routingKey + amqp.Publishing.
func toRoutingKeyAndAMQPPublishing(m *driver.Message, opts *TopicOptions) (routingKey string, msg amqp.Publishing) {
	h := amqp.Table{}
	for k, v := range m.Metadata {
		if opts.KeyName == k {
			routingKey = v
		} else {
			h[k] = v
		}
	}

	msg = amqp.Publishing{
		Headers: h,
		Body:    m.Body,
	}

	return routingKey, msg
}

// IsRetryable implements driver.Topic.IsRetryable.
func (*topic) IsRetryable(err error) bool {
	return isRetryable(err)
}

func (*topic) ErrorCode(err error) gcerrors.ErrorCode {
	return errorCode(err)
}

var errorCodes = map[int]gcerrors.ErrorCode{
	amqp.NotFound:           gcerrors.NotFound,
	amqp.PreconditionFailed: gcerrors.FailedPrecondition,
	// These next indicate a bug in our driver, not the user's code.
	amqp.SyntaxError:    gcerrors.Internal,
	amqp.CommandInvalid: gcerrors.Internal,
	amqp.InternalError:  gcerrors.Internal,
	amqp.NotImplemented: gcerrors.Unimplemented,
	amqp.ChannelError:   gcerrors.FailedPrecondition, // typically channel closed
}

func errorCode(err error) gcerrors.ErrorCode {
	var aerr *amqp.Error
	if !errors.As(err, &aerr) {
		return gcerrors.Unknown
	}
	if ec, ok := errorCodes[aerr.Code]; ok {
		return ec
	}
	return gcerrors.Unknown
}

func isRetryable(err error) bool {
	var aerr *amqp.Error
	if !errors.As(err, &aerr) {
		return false
	}
	// amqp.Error has a Recover field which sounds like it should mean "retryable".
	// But it actually means "can be recovered by retrying later or with different
	// parameters," which is not what we want. The error codes for which Recover is
	// true, defined in the isSoftExceptionCode function of
	// https://github.com/rabbitmq/amqp091-go/blob/main/spec091.go, including things
	// like NotFound and AccessRefused, which require outside action.
	//
	// The following are the codes which might be resolved by retry without external
	// action, according to the AMQP 0.91 spec
	// (https://www.rabbitmq.com/amqp-0-9-1-reference.html#constants). The quotations
	// are from that page.
	switch aerr.Code {
	case amqp.ContentTooLarge:
		// "The client attempted to transfer content larger than the server could
		// accept at the present time. The client may retry at a later time."
		return true

	case amqp.ConnectionForced:
		// "An operator intervened to close the connection for some reason. The
		// client may retry at some later date."
		return true

	default:
		return false
	}
}

// As implements driver.Topic.As.
func (t *topic) As(i any) bool {
	c, ok := i.(**amqp.Connection)
	if !ok {
		return false
	}
	conn, ok := t.conn.(*connection)
	if !ok { // running against the fake
		return false
	}
	*c = conn.conn
	return true
}

// ErrorAs implements driver.Topic.ErrorAs
func (*topic) ErrorAs(err error, i any) bool {
	return errorAs(err, i)
}

func errorAs(err error, i any) bool {
	var aerr *amqp.Error
	if errors.As(err, &aerr) {
		if p, ok := i.(**amqp.Error); ok {
			*p = aerr
			return true
		}
	}

	var merr MultiError
	if errors.As(err, &merr) {
		if p, ok := i.(*MultiError); ok {
			*p = merr
			return true
		}
	}

	return false
}

// Close implements driver.Topic.Close.
func (*topic) Close() error { return nil }

// OpenSubscription returns a *pubsub.Subscription corresponding to the named queue.
// See the package documentation for an example.
//
// The queue must have been previously created (for instance, by using
// amqp.Channel.QueueDeclare) and bound to an exchange.
//
// OpenSubscription uses the supplied amqp.Connection for all communication. It is
// the caller's responsibility to establish this connection before calling
// OpenSubscription and to close it when Close has been called on all Subscriptions
// opened with it.
//
// The documentation of the amqp package recommends using separate connections for
// publishing and subscribing.
func OpenSubscription(conn *amqp.Connection, name string, opts *SubscriptionOptions) *pubsub.Subscription {
	return pubsub.NewSubscription(newSubscription(&connection{conn}, name, opts), nil, nil)
}

type subscription struct {
	conn     amqpConnection
	queue    string // the AMQP queue name
	consumer string // the client-generated name for this particular subscriber

	opts *SubscriptionOptions

	mu     sync.Mutex
	ch     amqpChannel // AMQP channel used for all communication.
	delc   <-chan amqp.Delivery
	closec <-chan *amqp.Error

	receiveBatchHook func() // for testing
}

var nextConsumer int64 // atomic

func newSubscription(conn amqpConnection, name string, opts *SubscriptionOptions) *subscription {
	if opts == nil {
		opts = &SubscriptionOptions{}
	}

	return &subscription{
		conn:             conn,
		queue:            name,
		consumer:         fmt.Sprintf("c%d", atomic.AddInt64(&nextConsumer, 1)),
		opts:             opts,
		receiveBatchHook: func() {},
	}
}

// Must be called with s.mu held.
func (s *subscription) establishChannel(ctx context.Context) error {
	if s.ch != nil { // We already have a channel.
		select {
		// If it was closed, open a new one.
		// (Ignore the error, if any.)
		case <-s.closec:

		// If it isn't closed, nothing to do.
		default:
			return nil
		}
	}
	var ch amqpChannel
	err := runWithContext(ctx, func() error {
		// Create a new channel.
		var err error
		ch, err = s.conn.Channel()
		if err != nil {
			return err
		}
		// Apply subscription options to channel.
		err = applyOptionsToChannel(s.opts, ch)
		if err != nil {
			return err
		}
		// Subscribe to messages from the queue.
		s.delc, err = ch.Consume(s.queue, s.consumer)
		return err
	})
	if err != nil {
		return err
	}

	s.ch = ch
	s.closec = ch.NotifyClose(make(chan *amqp.Error, 1)) // closec will get at most one element

	return nil
}

func applyOptionsToChannel(opts *SubscriptionOptions, ch amqpChannel) error {
	if opts.PrefetchCount == nil {
		return nil
	}

	if err := ch.Qos(*opts.PrefetchCount, 0, false); err != nil {
		return fmt.Errorf("unable to set channel Qos: %w", err)
	}

	return nil
}

// ReceiveBatch implements driver.Subscription.ReceiveBatch.
func (s *subscription) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.establishChannel(ctx); err != nil {
		return nil, err
	}

	s.receiveBatchHook()

	// Get up to maxMessages waiting messages, but don't take too long.
	var ms []*driver.Message
	maxTime := time.NewTimer(50 * time.Millisecond)
	for {
		select {
		case <-ctx.Done():
			// Cancel the Consume.
			_ = s.ch.Cancel(s.consumer) // ignore the error
			s.ch = nil
			return nil, ctx.Err()

		case d, ok := <-s.delc:
			if !ok { // channel closed
				s.ch = nil // re-establish the channel next time
				if len(ms) > 0 {
					return ms, nil
				}
				// s.closec must be closed too. See if it has an error.
				if err := closeErr(s.closec); err != nil {
					// PreconditionFailed can happen if we send an Ack or Nack for a
					// message that has already been acked/nacked. Ignore those errors.
					var aerr *amqp.Error
					if errors.As(err, &aerr) && aerr.Code == amqp.PreconditionFailed {
						return nil, nil
					}
					return nil, err
				}
				// We shouldn't be here, but if we are, we still want to return an
				// error.
				return nil, errors.New("rabbitpubsub: delivery channel closed unexpectedly")
			}
			ms = append(ms, toDriverMessage(d, s.opts))
			if len(ms) >= maxMessages {
				return ms, nil
			}

		case <-maxTime.C:
			// Timed out. Return whatever we have. If we have nothing, we'll get
			// called again soon, but returning allows us to give up the lock in
			// case there are acks/nacks to be sent.
			return ms, nil
		}
	}
}

// toDriverMessage converts an amqp.Delivery (a received message) to a driver.Message.
func toDriverMessage(d amqp.Delivery, opts *SubscriptionOptions) *driver.Message {
	// Delivery.Headers is a map[string]any, so we have to
	// convert each value to a string.
	md := map[string]string{}
	for k, v := range d.Headers {
		md[k] = fmt.Sprint(v)
	}
	// Add a metadata entry for the message routing key if appropriate.
	if d.RoutingKey != "" && opts.KeyName != "" {
		md[opts.KeyName] = d.RoutingKey
	}
	loggableID := d.MessageId
	if loggableID == "" {
		loggableID = d.CorrelationId
	}
	if loggableID == "" {
		loggableID = fmt.Sprintf("DeliveryTag %d", d.DeliveryTag)
	}
	return &driver.Message{
		LoggableID: loggableID,
		Body:       d.Body,
		AckID:      d.DeliveryTag,
		Metadata:   md,
		AsFunc: func(i any) bool {
			p, ok := i.(*amqp.Delivery)
			if !ok {
				return false
			}
			*p = d
			return true
		},
	}
}

// SendAcks implements driver.Subscription.SendAcks.
func (s *subscription) SendAcks(ctx context.Context, ackIDs []driver.AckID) error {
	return s.sendAcksOrNacks(ctx, ackIDs, true)
}

// CanNack implements driver.CanNack.
func (s *subscription) CanNack() bool { return true }

// SendNacks implements driver.Subscription.SendNacks.
func (s *subscription) SendNacks(ctx context.Context, ackIDs []driver.AckID) error {
	return s.sendAcksOrNacks(ctx, ackIDs, false)
}

func (s *subscription) sendAcksOrNacks(ctx context.Context, ackIDs []driver.AckID, ack bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.establishChannel(ctx); err != nil {
		return err
	}

	// Ack/Nack calls don't wait for a response, so this loop should execute relatively
	// quickly.
	// It wouldn't help to make it concurrent, because Channel.Ack/Nack grabs a
	// channel-wide mutex. (We could consider using multiple channels if performance
	// becomes an issue.)
	for _, id := range ackIDs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var err error
		if ack {
			err = s.ch.Ack(id.(uint64))
		} else {
			err = s.ch.Nack(id.(uint64))
		}
		if err != nil {
			s.ch = nil // re-establish channel after an error
			return err
		}
	}
	return nil
}

// IsRetryable implements driver.Subscription.IsRetryable.
func (*subscription) IsRetryable(err error) bool {
	return isRetryable(err)
}

func (*subscription) ErrorCode(err error) gcerrors.ErrorCode {
	return errorCode(err)
}

// As implements driver.Subscription.As.
func (s *subscription) As(i any) bool {
	c, ok := i.(**amqp.Connection)
	if !ok {
		return false
	}
	conn, ok := s.conn.(*connection)
	if !ok { // running against the fake
		return false
	}
	*c = conn.conn
	return true
}

// ErrorAs implements driver.Subscription.ErrorAs
func (*subscription) ErrorAs(err error, i any) bool {
	return errorAs(err, i)
}

// Close implements driver.Subscription.Close.
func (*subscription) Close() error { return nil }
