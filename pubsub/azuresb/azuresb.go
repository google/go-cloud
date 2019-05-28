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
//  - Message: *servicebus.Message
//  - Error: common.Retryable
package azuresb // import "gocloud.dev/pubsub/azuresb"

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"

	common "github.com/Azure/azure-amqp-common-go"
	"github.com/Azure/azure-amqp-common-go/cbs"
	"github.com/Azure/azure-amqp-common-go/rpc"
	"github.com/Azure/azure-amqp-common-go/uuid"
	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/google/wire"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/batcher"
	"gocloud.dev/internal/useragent"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
	"pack.ag/amqp"
)

const (
	// https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-amqp-request-response#update-disposition-status
	dispositionForAck  = "completed"
	dispositionForNack = "abandoned"

	listenerTimeout = 1 * time.Second
)

var sendBatcherOpts = &batcher.Options{
	MaxBatchSize: 1,   // SendBatch only supports one message at a time
	MaxHandlers:  100, // max concurrency for sends
}

func init() {
	o := new(defaultOpener)
	pubsub.DefaultURLMux().RegisterTopic(Scheme, o)
	pubsub.DefaultURLMux().RegisterSubscription(Scheme, o)
}

// Set holds Wire providers for this package.
var Set = wire.NewSet(
	SubscriptionOptions{},
	TopicOptions{},
	URLOpener{},
)

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
	return t.sbTopic.Send(ctx, sbms)
}

func (t *topic) IsRetryable(err error) bool {
	// Let the Service Bus SDK recover from any transient connectivity issue.
	return false
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
	return errorCode(err)
}

// Close implements driver.Topic.Close.
func (*topic) Close() error { return nil }

type subscription struct {
	sbSub *servicebus.Subscription
	opts  *SubscriptionOptions

	linkErr  error     // saved error for initializing amqpLink
	amqpLink *rpc.Link // nil if linkErr != nil
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
	return pubsub.NewSubscription(ds, nil, nil), nil
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
	sub := &subscription{sbSub: sbSub, opts: opts}

	// Initialize a link to the AMQP server, but save any errors to be
	// returned in ReceiveBatch instead of returning them here, because we
	// want "subscription not found" to be a Receive time error.
	host := fmt.Sprintf("amqps://%s.%s/", sbNs.Name, sbNs.Environment.ServiceBusEndpointSuffix)
	amqpClient, err := amqp.Dial(host,
		amqp.ConnSASLAnonymous(),
		amqp.ConnProperty("product", "Go-Cloud Client"),
		amqp.ConnProperty("version", servicebus.Version),
		amqp.ConnProperty("platform", runtime.GOOS),
		amqp.ConnProperty("framework", runtime.Version()),
		amqp.ConnProperty("user-agent", useragent.AzureUserAgentPrefix("pubsub")),
	)
	if err != nil {
		sub.linkErr = fmt.Errorf("failed to dial AMQP: %v", err)
		return sub, nil
	}
	entityPath := sbTop.Name + "/Subscriptions/" + sbSub.Name
	audience := host + entityPath
	if err = cbs.NegotiateClaim(ctx, audience, amqpClient, sbNs.TokenProvider); err != nil {
		sub.linkErr = fmt.Errorf("failed to negotiate claim with AMQP: %v", err)
		return sub, nil
	}
	link, err := rpc.NewLink(amqpClient, sbSub.ManagementPath())
	if err != nil {
		sub.linkErr = fmt.Errorf("failed to create link to AMQP %s: %v", sbSub.ManagementPath(), err)
		return sub, nil
	}
	sub.amqpLink = link
	return sub, nil
}

// IsRetryable implements driver.Subscription.IsRetryable.
func (s *subscription) IsRetryable(error) bool {
	// Let the Service Bus SDK recover from any transient connectivity issue.
	return false
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
	return errorCode(err)
}

// ReceiveBatch implements driver.Subscription.ReceiveBatch.
func (s *subscription) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	if s.linkErr != nil {
		return nil, s.linkErr
	}

	rctx, cancel := context.WithTimeout(ctx, listenerTimeout)
	defer cancel()
	var messages []*driver.Message
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		s.sbSub.Receive(rctx, servicebus.HandlerFunc(func(innerctx context.Context, sbmsg *servicebus.Message) error {
			metadata := map[string]string{}
			sbmsg.ForeachKey(func(k, v string) error {
				metadata[k] = v
				return nil
			})
			messages = append(messages, &driver.Message{
				Body:     sbmsg.Data,
				Metadata: metadata,
				AckID:    sbmsg.LockToken,
				AsFunc:   messageAsFunc(sbmsg),
			})
			if len(messages) >= maxMessages {
				cancel()
			}
			return nil
		}))

		select {
		case <-rctx.Done():
			wg.Done()
		}
	}()

	wg.Wait()
	return messages, nil
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
	return s.updateMessageDispositions(ctx, ids, dispositionForAck)
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
	return s.updateMessageDispositions(ctx, ids, dispositionForNack)
}

// IMPORTANT: This is a workaround to issue message dispositions in bulk which is not supported in the Service Bus SDK.
func (s *subscription) updateMessageDispositions(ctx context.Context, ids []driver.AckID, disposition string) error {
	if len(ids) == 0 {
		return nil
	}

	lockIds := []amqp.UUID{}
	for _, mid := range ids {
		if id, ok := mid.(*uuid.UUID); ok {
			lockTokenBytes := [16]byte(*id)
			lockIds = append(lockIds, amqp.UUID(lockTokenBytes))
		}
	}
	value := map[string]interface{}{
		"disposition-status": disposition,
		"lock-tokens":        lockIds,
	}
	msg := &amqp.Message{
		ApplicationProperties: map[string]interface{}{
			"operation": "com.microsoft:update-disposition",
		},
		Value: value,
	}

	// We're not actually making use of link.Retryable since we're passing 1
	// here. The portable type will retry as needed.
	//
	// We could just use link.RPC, but it returns a result with a status code
	// in addition to err, and we'd have to check both.
	_, err := s.amqpLink.RetryableRPC(ctx, 1, 0, msg)
	if err == nil {
		return nil
	}
	if !isNotFoundErr(err) {
		return err
	}
	// It's a "not found" error, probably due to the message already being
	// deleted on the server. If we're just acking 1 message, we can just
	// swallow the error, but otherwise we'll need to retry one by one.
	if len(ids) == 1 {
		return nil
	}
	for _, lockID := range lockIds {
		value["lock-tokens"] = []amqp.UUID{lockID}
		if _, err := s.amqpLink.RetryableRPC(ctx, 1, 0, msg); err != nil && !isNotFoundErr(err) {
			return err
		}
	}
	return nil
}

// isNotFoundErr returns true if the error is status code 410, Gone.
// Azure returns this when trying to ack/nack a message that no longer exists.
func isNotFoundErr(err error) bool {
	return strings.Contains(err.Error(), "status code 410")
}

func errorCode(err error) gcerrors.ErrorCode {
	// Unfortunately Azure sometimes returns common.Retryable or even
	// errors.errorString, which don't expose anything other than the error
	// string :-(.
	if strings.Contains(err.Error(), "status code 404") {
		return gcerrors.NotFound
	}
	aerr, ok := err.(*amqp.Error)
	if !ok {
		return gcerrors.Unknown
	}
	switch aerr.Condition {
	case amqp.ErrorCondition(servicebus.ErrorNotFound):
		return gcerrors.NotFound

	case amqp.ErrorCondition(servicebus.ErrorPreconditionFailed):
		return gcerrors.FailedPrecondition

	case amqp.ErrorCondition(servicebus.ErrorInternalError):
		return gcerrors.Internal

	case amqp.ErrorCondition(servicebus.ErrorNotImplemented):
		return gcerrors.Unimplemented

	case amqp.ErrorCondition(servicebus.ErrorUnauthorizedAccess), amqp.ErrorCondition(servicebus.ErrorNotAllowed):
		return gcerrors.PermissionDenied

	case amqp.ErrorCondition(servicebus.ErrorResourceLimitExceeded):
		return gcerrors.ResourceExhausted

	case amqp.ErrorCondition(servicebus.ErrorInvalidField):
		return gcerrors.InvalidArgument

	default:
		return gcerrors.Unknown
	}
}

// Close implements driver.Subscription.Close.
func (*subscription) Close() error { return nil }
