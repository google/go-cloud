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

// Package azuresb provides an implementation of pubsub using Azure Service
// Bus Topic and Subscription.
// See https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-messaging-overview for an overview.
//
// URLs
//
// For pubsub.OpenTopic and pubsub.OpenSubscription, azuresb registers
// for the scheme "azuresb".
// The default URL opener will use a Service Bus Connection String based on
// the environment variable "SERVICEBUS_CONNECTION_STRING".
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// Message Delivery Semantics
//
// Azure ServiceBus supports at-least-once semantics in the default Peek-Lock
// mode; messages will be redelivered if they are not Acked, or if they are
// explicitly Nacked.
//
// ServiceBus also supports a Receive-Delete mode, which essentially auto-acks a
// message when it is delivered, resulting in at-most-once semantics. Set
// SubscriberOptions.ReceiveAndDelete to true to tell azuresb.Subscription that
// you've enabled Receive-Delete mode. When enabled, pubsub.Message.Ack is a
// no-op, pubsub.Message.Nackable will return false, and pubsub.Message.Nack
// will panic.
//
// See https://godoc.org/gocloud.dev/pubsub#hdr-At_most_once_and_At_least_once_Delivery
// for more background.
//
// As
//
// azuresb exposes the following types for As:
//  - Topic: *servicebus.Topic
//  - Subscription: *servicebus.Subscription
//  - Message.BeforeSend: *servicebus.Message
//  - Message.AfterSend: None
//  - Message: *servicebus.Message
//  - Error: common.Retryable, *amqp.Error, *amqp.DetachError
package azuresb // import "gocloud.dev/pubsub/azuresb"

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	common "github.com/Azure/azure-amqp-common-go/v3"
	"github.com/Azure/azure-amqp-common-go/v3/uuid"
	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-amqp"
	"gocloud.dev/gcerrors"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/batcher"
	"gocloud.dev/pubsub/driver"
)

const (
	listenerTimeout = 1 * time.Second
)

var sendBatcherOpts = &batcher.Options{
	MaxBatchSize: 1,   // SendBatch only supports one message at a time
	MaxHandlers:  100, // max concurrency for sends
}

var recvBatcherOpts = &batcher.Options{
	MaxBatchSize: 1,   // ReceiveBatch only returns one message at a time
	MaxHandlers:  100, // max concurrency for reads
}

func init() {
	o := new(defaultOpener)
	pubsub.DefaultURLMux().RegisterTopic(Scheme, o)
	pubsub.DefaultURLMux().RegisterSubscription(Scheme, o)
}

// defaultURLOpener creates an URLOpener with ConnectionString initialized from
// the environment variable SERVICEBUS_CONNECTION_STRING.
type defaultOpener struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

func (o *defaultOpener) defaultOpener() (*URLOpener, error) {
	o.init.Do(func() {
		cs := os.Getenv("SERVICEBUS_CONNECTION_STRING")
		if cs == "" {
			o.err = errors.New("SERVICEBUS_CONNECTION_STRING environment variable not set")
			return
		}
		o.opener = &URLOpener{ConnectionString: cs}
	})
	return o.opener, o.err
}

func (o *defaultOpener) OpenTopicURL(ctx context.Context, u *url.URL) (*pubsub.Topic, error) {
	opener, err := o.defaultOpener()
	if err != nil {
		return nil, fmt.Errorf("open topic %v: %v", u, err)
	}
	return opener.OpenTopicURL(ctx, u)
}

func (o *defaultOpener) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*pubsub.Subscription, error) {
	opener, err := o.defaultOpener()
	if err != nil {
		return nil, fmt.Errorf("open subscription %v: %v", u, err)
	}
	return opener.OpenSubscriptionURL(ctx, u)
}

// Scheme is the URL scheme azuresb registers its URLOpeners under on pubsub.DefaultMux.
const Scheme = "azuresb"

// URLOpener opens Azure Service Bus URLs like "azuresb://mytopic" for
// topics or "azuresb://mytopic?subscription=mysubscription" for subscriptions.
//
//   - The URL's host+path is used as the topic name.
//   - For subscriptions, the subscription name must be provided in the
//     "subscription" query parameter.
//
// No other query parameters are supported.
type URLOpener struct {
	// ConnectionString is the Service Bus connection string (required).
	// https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-dotnet-get-started-with-queues
	ConnectionString string

	// Options passed when creating the ServiceBus Topic/Subscription.
	ServiceBusTopicOptions        []servicebus.TopicOption
	ServiceBusSubscriptionOptions []servicebus.SubscriptionOption

	// TopicOptions specifies the options to pass to OpenTopic.
	TopicOptions TopicOptions
	// SubscriptionOptions specifies the options to pass to OpenSubscription.
	SubscriptionOptions SubscriptionOptions
}

func (o *URLOpener) namespace(kind string, u *url.URL) (*servicebus.Namespace, error) {
	if o.ConnectionString == "" {
		return nil, fmt.Errorf("open %s %v: ConnectionString is required", kind, u)
	}
	ns, err := NewNamespaceFromConnectionString(o.ConnectionString)
	if err != nil {
		return nil, fmt.Errorf("open %s %v: invalid connection string %q: %v", kind, u, o.ConnectionString, err)
	}
	return ns, nil
}

// OpenTopicURL opens a pubsub.Topic based on u.
func (o *URLOpener) OpenTopicURL(ctx context.Context, u *url.URL) (*pubsub.Topic, error) {
	ns, err := o.namespace("topic", u)
	if err != nil {
		return nil, err
	}
	for param := range u.Query() {
		return nil, fmt.Errorf("open topic %v: invalid query parameter %q", u, param)
	}
	topicName := path.Join(u.Host, u.Path)
	t, err := NewTopic(ns, topicName, o.ServiceBusTopicOptions)
	if err != nil {
		return nil, fmt.Errorf("open topic %v: couldn't open topic %q: %v", u, topicName, err)
	}
	return OpenTopic(ctx, t, &o.TopicOptions)
}

// OpenSubscriptionURL opens a pubsub.Subscription based on u.
func (o *URLOpener) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*pubsub.Subscription, error) {
	ns, err := o.namespace("subscription", u)
	if err != nil {
		return nil, err
	}
	topicName := path.Join(u.Host, u.Path)
	t, err := NewTopic(ns, topicName, o.ServiceBusTopicOptions)
	if err != nil {
		return nil, fmt.Errorf("open subscription %v: couldn't open topic %q: %v", u, topicName, err)
	}
	q := u.Query()

	subName := q.Get("subscription")
	q.Del("subscription")
	if subName == "" {
		return nil, fmt.Errorf("open subscription %v: missing required query parameter subscription", u)
	}

	for param := range q {
		return nil, fmt.Errorf("open subscription %v: invalid query parameter %q", u, param)
	}
	sub, err := NewSubscription(t, subName, o.ServiceBusSubscriptionOptions)
	if err != nil {
		return nil, fmt.Errorf("open subscription %v: couldn't open subscription %q: %v", u, subName, err)
	}
	return OpenSubscription(ctx, ns, t, sub, &o.SubscriptionOptions)
}

type topic struct {
	sbTopic *servicebus.Topic
}

// TopicOptions provides configuration options for an Azure SB Topic.
type TopicOptions struct{}

// NewNamespaceFromConnectionString returns a *servicebus.Namespace from a Service Bus connection string.
// https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-dotnet-get-started-with-queues
func NewNamespaceFromConnectionString(connectionString string) (*servicebus.Namespace, error) {
	nsOptions := servicebus.NamespaceWithConnectionString(connectionString)
	return servicebus.NewNamespace(nsOptions)
}

// NewTopic returns a *servicebus.Topic associated with a Service Bus Namespace.
func NewTopic(ns *servicebus.Namespace, topicName string, opts []servicebus.TopicOption) (*servicebus.Topic, error) {
	return ns.NewTopic(topicName, opts...)
}

// NewSubscription returns a *servicebus.Subscription associated with a Service Bus Topic.
func NewSubscription(parentTopic *servicebus.Topic, subscriptionName string, opts []servicebus.SubscriptionOption) (*servicebus.Subscription, error) {
	return parentTopic.NewSubscription(subscriptionName, opts...)
}

// OpenTopic initializes a pubsub Topic on a given Service Bus Topic.
func OpenTopic(ctx context.Context, sbTopic *servicebus.Topic, opts *TopicOptions) (*pubsub.Topic, error) {
	t, err := openTopic(ctx, sbTopic, opts)
	if err != nil {
		return nil, err
	}
	return pubsub.NewTopic(t, sendBatcherOpts), nil
}

// openTopic returns the driver for OpenTopic. This function exists so the test
// harness can get the driver interface implementation if it needs to.
func openTopic(ctx context.Context, sbTopic *servicebus.Topic, _ *TopicOptions) (driver.Topic, error) {
	if sbTopic == nil {
		return nil, errors.New("azuresb: OpenTopic requires a Service Bus Topic")
	}
	return &topic{sbTopic: sbTopic}, nil
}

// SendBatch implements driver.Topic.SendBatch.
func (t *topic) SendBatch(ctx context.Context, dms []*driver.Message) error {
	if len(dms) != 1 {
		panic("azuresb.SendBatch should only get one message at a time")
	}
	dm := dms[0]
	sbms := servicebus.NewMessage(dm.Body)
	for k, v := range dm.Metadata {
		sbms.Set(k, v)
	}
	if dm.BeforeSend != nil {
		asFunc := func(i interface{}) bool {
			if p, ok := i.(**servicebus.Message); ok {
				*p = sbms
				return true
			}
			return false
		}
		if err := dm.BeforeSend(asFunc); err != nil {
			return err
		}
	}
	err := t.sbTopic.Send(ctx, sbms)
	if err != nil {
		return err
	}
	if dm.AfterSend != nil {
		asFunc := func(i interface{}) bool { return false }
		if err := dm.AfterSend(asFunc); err != nil {
			return err
		}
	}
	return nil
}

func (t *topic) IsRetryable(err error) bool {
	_, retryable := errorCode(err)
	return retryable
}

func (t *topic) As(i interface{}) bool {
	p, ok := i.(**servicebus.Topic)
	if !ok {
		return false
	}
	*p = t.sbTopic
	return true
}

// ErrorAs implements driver.Topic.ErrorAs
func (*topic) ErrorAs(err error, i interface{}) bool {
	return errorAs(err, i)
}

func errorAs(err error, i interface{}) bool {
	switch v := err.(type) {
	case *amqp.DetachError:
		if p, ok := i.(**amqp.DetachError); ok {
			*p = v
			return true
		}
	case *amqp.Error:
		if p, ok := i.(**amqp.Error); ok {
			*p = v
			return true
		}
	case common.Retryable:
		if p, ok := i.(*common.Retryable); ok {
			*p = v
			return true
		}
	}
	return false
}

func (*topic) ErrorCode(err error) gcerrors.ErrorCode {
	code, _ := errorCode(err)
	return code
}

// Close implements driver.Topic.Close.
func (*topic) Close() error { return nil }

type subscription struct {
	sbSub *servicebus.Subscription
	opts  *SubscriptionOptions
}

// SubscriptionOptions will contain configuration for subscriptions.
type SubscriptionOptions struct {
	// If false, the serviceBus.Subscription MUST be in the default Peek-Lock mode.
	// If true, the serviceBus.Subscription MUST be in Receive-and-Delete mode.
	// When true: pubsub.Message.Ack will be a no-op, pubsub.Message.Nackable
	// will return true, and pubsub.Message.Nack will panic.
	ReceiveAndDelete bool
}

// OpenSubscription initializes a pubsub Subscription on a given Service Bus Subscription and its parent Service Bus Topic.
func OpenSubscription(ctx context.Context, parentNamespace *servicebus.Namespace, parentTopic *servicebus.Topic, sbSubscription *servicebus.Subscription, opts *SubscriptionOptions) (*pubsub.Subscription, error) {
	ds, err := openSubscription(ctx, parentNamespace, parentTopic, sbSubscription, opts)
	if err != nil {
		return nil, err
	}
	return pubsub.NewSubscription(ds, recvBatcherOpts, nil), nil
}

// openSubscription returns a driver.Subscription.
func openSubscription(ctx context.Context, sbNs *servicebus.Namespace, sbTop *servicebus.Topic, sbSub *servicebus.Subscription, opts *SubscriptionOptions) (driver.Subscription, error) {
	if sbNs == nil {
		return nil, errors.New("azuresb: OpenSubscription requires a Service Bus Namespace")
	}
	if sbTop == nil {
		return nil, errors.New("azuresb: OpenSubscription requires a Service Bus Topic")
	}
	if sbSub == nil {
		return nil, errors.New("azuresb: OpenSubscription requires a Service Bus Subscription")
	}
	if opts == nil {
		opts = &SubscriptionOptions{}
	}
	return &subscription{sbSub: sbSub, opts: opts}, nil
}

// IsRetryable implements driver.Subscription.IsRetryable.
func (s *subscription) IsRetryable(err error) bool {
	_, retryable := errorCode(err)
	return retryable
}

// As implements driver.Subscription.As.
func (s *subscription) As(i interface{}) bool {
	p, ok := i.(**servicebus.Subscription)
	if !ok {
		return false
	}
	*p = s.sbSub
	return true
}

// ErrorAs implements driver.Subscription.ErrorAs
func (s *subscription) ErrorAs(err error, i interface{}) bool {
	return errorAs(err, i)
}

func (s *subscription) ErrorCode(err error) gcerrors.ErrorCode {
	code, _ := errorCode(err)
	return code
}

// ReceiveBatch implements driver.Subscription.ReceiveBatch.
func (s *subscription) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	// ReceiveOne will block until rctx is Done; we want to return after
	// a reasonably short delay even if there are no messages. So, create a
	// sub context for the RPC.
	rctx, cancel := context.WithTimeout(ctx, listenerTimeout)
	defer cancel()

	// NOTE: there's also a Receive method, but it starts two goroutines
	// that aren't necessarily finished when Receive returns, which causes
	// data races if Receive is called again quickly. ReceiveOne is more
	// straightforward.
	var messages []*driver.Message
	err := s.sbSub.ReceiveOne(rctx, servicebus.HandlerFunc(func(_ context.Context, sbmsg *servicebus.Message) error {
		metadata := map[string]string{}
		for key, value := range sbmsg.GetKeyValues() {
			if strVal, ok := value.(string); ok {
				metadata[key] = strVal
			}
		}
		messages = append(messages, &driver.Message{
			LoggableID: sbmsg.ID,
			Body:       sbmsg.Data,
			Metadata:   metadata,
			AckID:      sbmsg.LockToken,
			AsFunc:     messageAsFunc(sbmsg),
		})
		return nil
	}))
	// Mask rctx timeouts, they are expected if no messages are available.
	if err == rctx.Err() {
		err = nil
	}
	return messages, err
}

func messageAsFunc(sbmsg *servicebus.Message) func(interface{}) bool {
	return func(i interface{}) bool {
		p, ok := i.(**servicebus.Message)
		if !ok {
			return false
		}
		*p = sbmsg
		return true
	}
}

// SendAcks implements driver.Subscription.SendAcks.
func (s *subscription) SendAcks(ctx context.Context, ids []driver.AckID) error {
	if s.opts.ReceiveAndDelete {
		// Ack is a no-op in Receive-and-Delete mode.
		return nil
	}
	return s.updateMessageDispositions(ctx, ids, servicebus.Complete)
}

// CanNack implements driver.CanNack.
func (s *subscription) CanNack() bool {
	if s == nil {
		return false
	}
	return !s.opts.ReceiveAndDelete
}

// SendNacks implements driver.Subscription.SendNacks.
func (s *subscription) SendNacks(ctx context.Context, ids []driver.AckID) error {
	if !s.CanNack() {
		panic("unreachable")
	}
	return s.updateMessageDispositions(ctx, ids, servicebus.Abort)
}

func (s *subscription) updateMessageDispositions(ctx context.Context, ids []driver.AckID, status servicebus.MessageStatus) error {
	if len(ids) == 0 {
		return nil
	}
	var lockTokenIDs []*uuid.UUID
	for _, ackID := range ids {
		if uid, ok := ackID.(*uuid.UUID); ok {
			lockTokenIDs = append(lockTokenIDs, uid)
		}
	}
	err := s.sbSub.SendBatchDisposition(ctx, servicebus.BatchDispositionIterator{
		LockTokenIDs: lockTokenIDs,
		Status:       status,
	})
	// The error returned from SendBatchDisposition is confusing. It always returns a non-nil
	// *BatchDispositionError, which holds a list of LockTokenIDs that had errors,
	// which might be empty (indicating no error).
	if err == nil {
		// Unexpected, but clearly no error.
		return nil
	}
	bderr, ok := err.(*servicebus.BatchDispositionError)
	if !ok {
		// Unexpected, some other kind of error; just return it.
		return err
	}
	if bderr == nil || len(bderr.Errors) == 0 {
		// No actual errors.
		return nil
	}
	// Ignore "not found" errors, as they are likely re-acks. Return the
	// first other error (if any).
	for _, err := range bderr.Errors {
		if isNotFoundErr(err) {
			continue
		}
		return err
	}
	return nil
}

// isNotFoundErr returns true if the error is status code 410, Gone.
// Azure returns this when trying to ack/nack a message that no longer exists.
func isNotFoundErr(err error) bool {
	return strings.Contains(err.Error(), "status code 410")
}

// errorCode returns an error code and whether err is retryable.
func errorCode(err error) (gcerrors.ErrorCode, bool) {
	// Unfortunately Azure sometimes returns common.Retryable or even
	// errors.errorString, which don't expose anything other than the error
	// string :-(.
	if strings.Contains(err.Error(), "status code 404") {
		return gcerrors.NotFound, false
	}
	var cond amqp.ErrorCondition
	if aerr, ok := err.(*amqp.DetachError); ok {
		if aerr.RemoteError == nil {
			return gcerrors.NotFound, false
		}
		cond = aerr.RemoteError.Condition
	}
	if aerr, ok := err.(*amqp.Error); ok {
		cond = aerr.Condition
	}
	switch cond {
	case amqp.ErrorCondition(servicebus.ErrorNotFound):
		return gcerrors.NotFound, false

	case amqp.ErrorCondition(servicebus.ErrorPreconditionFailed):
		return gcerrors.FailedPrecondition, false

	case amqp.ErrorCondition(servicebus.ErrorInternalError):
		return gcerrors.Internal, true

	case amqp.ErrorCondition(servicebus.ErrorNotImplemented):
		return gcerrors.Unimplemented, false

	case amqp.ErrorCondition(servicebus.ErrorUnauthorizedAccess), amqp.ErrorCondition(servicebus.ErrorNotAllowed):
		return gcerrors.PermissionDenied, false

	case amqp.ErrorCondition(servicebus.ErrorResourceLimitExceeded):
		return gcerrors.ResourceExhausted, true

	case amqp.ErrorCondition(servicebus.ErrorInvalidField):
		return gcerrors.InvalidArgument, false
	}
	return gcerrors.Unknown, true
}

// Close implements driver.Subscription.Close.
func (*subscription) Close() error { return nil }
