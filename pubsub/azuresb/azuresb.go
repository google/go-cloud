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
// # URLs
//
// For pubsub.OpenTopic and pubsub.OpenSubscription, azuresb registers
// for the scheme "azuresb".
// The default URL opener will use a Service Bus Connection String based on
// AZURE_SERVICEBUS_HOSTNAME or SERVICEBUS_CONNECTION_STRING environment variables.
// SERVICEBUS_CONNECTION_STRING takes precedence.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// # Message Delivery Semantics
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
// # As
//
// azuresb exposes the following types for As:
//   - Topic: *servicebus.Topic
//   - Subscription: *servicebus.Subscription
//   - Message.BeforeSend: *servicebus.Message
//   - Message.AfterSend: None
//   - Message: *servicebus.Message
//   - Error: common.Retryable, *amqp.Error, *amqp.LinkError
package azuresb // import "gocloud.dev/pubsub/azuresb"

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
	"time"

	common "github.com/Azure/azure-amqp-common-go/v3"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	servicebus "github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/Azure/go-amqp"
	"gocloud.dev/gcerrors"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/batcher"
	"gocloud.dev/pubsub/driver"
)

const (
	defaultListenerTimeout = 2 * time.Second
)

var sendBatcherOpts = &batcher.Options{
	MaxBatchSize: 1,   // SendBatch only supports one message at a time
	MaxHandlers:  100, // max concurrency for sends
}

var recvBatcherOpts = &batcher.Options{
	MaxBatchSize: 50,
	MaxHandlers:  100, // max concurrency for reads
}

var ackBatcherOpts = &batcher.Options{
	MaxBatchSize: 1,
	MaxHandlers:  100, // max concurrency for acks
}

func init() {
	o := new(defaultOpener)
	pubsub.DefaultURLMux().RegisterTopic(Scheme, o)
	pubsub.DefaultURLMux().RegisterSubscription(Scheme, o)
}

// defaultURLOpener creates an URLOpener with ConnectionString initialized from
// AZURE_SERVICEBUS_HOSTNAME or SERVICEBUS_CONNECTION_STRING environment variables.
// SERVICEBUS_CONNECTION_STRING takes precedence.
type defaultOpener struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

func (o *defaultOpener) defaultOpener() (*URLOpener, error) {
	o.init.Do(func() {
		cs := os.Getenv("SERVICEBUS_CONNECTION_STRING")
		sbHostname := os.Getenv("AZURE_SERVICEBUS_HOSTNAME")
		if cs == "" && sbHostname == "" {
			o.err = errors.New("Neither SERVICEBUS_CONNECTION_STRING nor AZURE_SERVICEBUS_HOSTNAME environment variables are set")
			return
		}
		o.opener = &URLOpener{ConnectionString: cs, ServiceBusHostname: sbHostname}
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
//   - For subscriptions, the ListenerTimeout can be overridden with time.Duration parseable values in "listener_timeout".
//
// No other query parameters are supported.
type URLOpener struct {
	// ConnectionString is the Service Bus connection string (required if ServiceBusHostname is not defined).
	// https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-dotnet-get-started-with-queues
	ConnectionString string

	// Azure ServiceBus hostname.
	// https://learn.microsoft.com/en-us/azure/service-bus-messaging/service-bus-go-how-to-use-queues?tabs=bash
	ServiceBusHostname string

	// ClientOptions are options when creating the Client.
	ServiceBusClientOptions *servicebus.ClientOptions

	// Options passed when creating the ServiceBus Topic/Subscription.
	ServiceBusSenderOptions   *servicebus.NewSenderOptions
	ServiceBusReceiverOptions *servicebus.ReceiverOptions

	// TopicOptions specifies the options to pass to OpenTopic.
	TopicOptions TopicOptions
	// SubscriptionOptions specifies the options to pass to OpenSubscription.
	SubscriptionOptions SubscriptionOptions
}

func (o *URLOpener) sbClient(kind string, u *url.URL) (*servicebus.Client, error) {
	if o.ConnectionString == "" && o.ServiceBusHostname == "" {
		return nil, fmt.Errorf("open %s %v: one of ConnectionString or ServiceBusHostname is required", kind, u)
	}

	// Auth using shared key.
	if o.ConnectionString != "" {
		client, err := NewClientFromConnectionString(o.ConnectionString, o.ServiceBusClientOptions)
		if err != nil {
			return nil, fmt.Errorf("open %s %v: invalid connection string %q: %v", kind, u, o.ConnectionString, err)
		}
		return client, nil
	}

	// Auth using Azure AAD Workload Identity/AAD Pod Identities/AKS Kubelet Identity/Service Principal.
	client, err := NewClientFromServiceBusHostname(o.ServiceBusHostname, o.ServiceBusClientOptions)
	if err != nil {
		return nil, fmt.Errorf("open %s %v: invalid service bus hostname %q: %v", kind, u, o.ServiceBusHostname, err)
	}
	return client, nil
}

// OpenTopicURL opens a pubsub.Topic based on u.
func (o *URLOpener) OpenTopicURL(ctx context.Context, u *url.URL) (*pubsub.Topic, error) {
	sbClient, err := o.sbClient("topic", u)
	if err != nil {
		return nil, err
	}
	for param := range u.Query() {
		return nil, fmt.Errorf("open topic %v: invalid query parameter %q", u, param)
	}
	topicName := path.Join(u.Host, u.Path)
	sbSender, err := NewSender(sbClient, topicName, o.ServiceBusSenderOptions)
	if err != nil {
		return nil, fmt.Errorf("open topic %v: couldn't open topic %q: %v", u, topicName, err)
	}
	return OpenTopic(ctx, sbSender, &o.TopicOptions)
}

// OpenSubscriptionURL opens a pubsub.Subscription based on u.
func (o *URLOpener) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*pubsub.Subscription, error) {
	sbClient, err := o.sbClient("subscription", u)
	if err != nil {
		return nil, err
	}
	topicName := path.Join(u.Host, u.Path)
	q := u.Query()
	subName := q.Get("subscription")
	q.Del("subscription")
	if subName == "" {
		return nil, fmt.Errorf("open subscription %v: missing required query parameter subscription", u)
	}
	opts := o.SubscriptionOptions
	if lts := q.Get("listener_timeout"); lts != "" {
		q.Del("listener_timeout")
		d, err := time.ParseDuration(lts)
		if err != nil {
			return nil, fmt.Errorf("open subscription %v: invalid listener_timeout %q: %v", u, lts, err)
		}
		opts.ListenerTimeout = d
	}
	if mrbss := q.Get("max_recv_batch_size"); mrbss != "" {
		q.Del("max_recv_batch_size")
		mrbs, err := strconv.Atoi(mrbss)
		if err != nil {
			return nil, fmt.Errorf("open subscription %v: invalid max_recv_batch_size %q: %v", u, mrbss, err)
		}
		opts.ReceiveBatcherOptions.MaxBatchSize = mrbs
	}
	for param := range q {
		return nil, fmt.Errorf("open subscription %v: invalid query parameter %q", u, param)
	}
	sbReceiver, err := NewReceiver(sbClient, topicName, subName, o.ServiceBusReceiverOptions)
	if err != nil {
		return nil, fmt.Errorf("open subscription %v: couldn't open subscription %q: %v", u, subName, err)
	}
	return OpenSubscription(ctx, sbClient, sbReceiver, &opts)
}

type topic struct {
	sbSender *servicebus.Sender
}

// TopicOptions provides configuration options for an Azure SB Topic.
type TopicOptions struct {
	// BatcherOptions adds constraints to the default batching done for sends.
	BatcherOptions batcher.Options
}

// NewClientFromConnectionString returns a *servicebus.Client from a Service Bus connection string, using shared key for auth.
// https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-dotnet-get-started-with-queues
func NewClientFromConnectionString(connectionString string, opts *servicebus.ClientOptions) (*servicebus.Client, error) {
	return servicebus.NewClientFromConnectionString(connectionString, opts)
}

// NewClientFromConnectionString returns a *servicebus.Client from a Service Bus connection string, using shared key for auth.
// for example you can use workload identity autorization.
// https://learn.microsoft.com/en-us/azure/service-bus-messaging/service-bus-go-how-to-use-queues?tabs=bash
func NewClientFromServiceBusHostname(serviceBusHostname string, opts *servicebus.ClientOptions) (*servicebus.Client, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}
	client, err := servicebus.NewClient(serviceBusHostname, cred, opts)
	if err != nil {
		return nil, err
	}
	return client, nil
}

// NewSender returns a *servicebus.Sender associated with a Service Bus Client.
func NewSender(sbClient *servicebus.Client, topicName string, opts *servicebus.NewSenderOptions) (*servicebus.Sender, error) {
	return sbClient.NewSender(topicName, opts)
}

// NewReceiver returns a *servicebus.Receiver associated with a Service Bus Topic.
func NewReceiver(sbClient *servicebus.Client, topicName, subscriptionName string, opts *servicebus.ReceiverOptions) (*servicebus.Receiver, error) {
	return sbClient.NewReceiverForSubscription(topicName, subscriptionName, opts)
}

// OpenTopic initializes a pubsub Topic on a given Service Bus Sender.
func OpenTopic(ctx context.Context, sbSender *servicebus.Sender, opts *TopicOptions) (*pubsub.Topic, error) {
	t, err := openTopic(ctx, sbSender, opts)
	if err != nil {
		return nil, err
	}
	if opts == nil {
		opts = &TopicOptions{}
	}
	bo := sendBatcherOpts.NewMergedOptions(&opts.BatcherOptions)
	return pubsub.NewTopic(t, bo), nil
}

// openTopic returns the driver for OpenTopic. This function exists so the test
// harness can get the driver interface implementation if it needs to.
func openTopic(ctx context.Context, sbSender *servicebus.Sender, _ *TopicOptions) (driver.Topic, error) {
	if sbSender == nil {
		return nil, errors.New("azuresb: OpenTopic requires a Service Bus Sender")
	}
	return &topic{sbSender: sbSender}, nil
}

// SendBatch implements driver.Topic.SendBatch.
func (t *topic) SendBatch(ctx context.Context, dms []*driver.Message) error {
	if len(dms) != 1 {
		panic("azuresb.SendBatch should only get one message at a time")
	}
	dm := dms[0]
	sbms := &servicebus.Message{Body: dm.Body}
	if len(dm.Metadata) > 0 {
		sbms.ApplicationProperties = map[string]interface{}{}
		for k, v := range dm.Metadata {
			sbms.ApplicationProperties[k] = v
		}
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
	err := t.sbSender.SendMessage(ctx, sbms, nil)
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
	p, ok := i.(**servicebus.Sender)
	if !ok {
		return false
	}
	*p = t.sbSender
	return true
}

// ErrorAs implements driver.Topic.ErrorAs
func (*topic) ErrorAs(err error, i interface{}) bool {
	return errorAs(err, i)
}

func errorAs(err error, i interface{}) bool {
	switch v := err.(type) {
	case *amqp.LinkError:
		if p, ok := i.(**amqp.LinkError); ok {
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
	sbReceiver *servicebus.Receiver
	opts       *SubscriptionOptions
}

// SubscriptionOptions will contain configuration for subscriptions.
type SubscriptionOptions struct {
	// If false, the serviceBus.Subscription MUST be in the default Peek-Lock mode.
	// If true, the serviceBus.Subscription MUST be in Receive-and-Delete mode.
	// When true: pubsub.Message.Ack will be a no-op, pubsub.Message.Nackable
	// will return true, and pubsub.Message.Nack will panic.
	ReceiveAndDelete bool

	// ReceiveBatcherOptions adds constraints to the default batching done for receives.
	ReceiveBatcherOptions batcher.Options

	// AckBatcherOptions adds constraints to the default batching done for acks.
	// Only used when ReceiveAndDelete is false.
	AckBatcherOptions batcher.Options

	// ListenerTimeout is the amount of time to wait before timing out the
	// ReceiveMessages RPC call. This is used to ensure the receive operation is
	// non-blocking as the RPC blocks if there are no messages.
	// Defaults to 2 seconds.
	ListenerTimeout time.Duration
}

// OpenSubscription initializes a pubsub Subscription on a given Service Bus Subscription and its parent Service Bus Topic.
func OpenSubscription(ctx context.Context, sbClient *servicebus.Client, sbReceiver *servicebus.Receiver, opts *SubscriptionOptions) (*pubsub.Subscription, error) {
	ds, err := openSubscription(ctx, sbClient, sbReceiver, opts)
	if err != nil {
		return nil, err
	}
	if opts == nil {
		opts = &SubscriptionOptions{}
	}
	rbo := recvBatcherOpts.NewMergedOptions(&opts.ReceiveBatcherOptions)
	abo := ackBatcherOpts.NewMergedOptions(&opts.AckBatcherOptions)
	return pubsub.NewSubscription(ds, rbo, abo), nil
}

// openSubscription returns a driver.Subscription.
func openSubscription(ctx context.Context, sbClient *servicebus.Client, sbReceiver *servicebus.Receiver, opts *SubscriptionOptions) (driver.Subscription, error) {
	if sbClient == nil {
		return nil, errors.New("azuresb: OpenSubscription requires a Service Bus Client")
	}
	if sbReceiver == nil {
		return nil, errors.New("azuresb: OpenSubscription requires a Service Bus Receiver")
	}
	if opts == nil {
		opts = &SubscriptionOptions{}
	}
	if opts.ListenerTimeout == 0 {
		opts.ListenerTimeout = defaultListenerTimeout
	}
	return &subscription{sbReceiver: sbReceiver, opts: opts}, nil
}

// IsRetryable implements driver.Subscription.IsRetryable.
func (s *subscription) IsRetryable(err error) bool {
	_, retryable := errorCode(err)
	return retryable
}

// As implements driver.Subscription.As.
func (s *subscription) As(i interface{}) bool {
	p, ok := i.(**servicebus.Receiver)
	if !ok {
		return false
	}
	*p = s.sbReceiver
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
	// ReceiveMessages will block until rctx is Done; we want to return after
	// a reasonably short delay even if there are no messages. So, create a
	// sub context for the RPC.
	rctx, cancel := context.WithTimeout(ctx, s.opts.ListenerTimeout)
	defer cancel()

	var messages []*driver.Message
	sbmsgs, err := s.sbReceiver.ReceiveMessages(rctx, maxMessages, nil)
	for _, sbmsg := range sbmsgs {
		metadata := map[string]string{}
		for key, value := range sbmsg.ApplicationProperties {
			if strVal, ok := value.(string); ok {
				metadata[key] = strVal
			}
		}
		messages = append(messages, &driver.Message{
			LoggableID: sbmsg.MessageID,
			Body:       sbmsg.Body,
			Metadata:   metadata,
			AckID:      sbmsg,
			AsFunc:     messageAsFunc(sbmsg),
		})
	}
	// Mask rctx timeouts, they are expected if no messages are available.
	if err == rctx.Err() {
		err = nil
	}
	return messages, err
}

func messageAsFunc(sbmsg *servicebus.ReceivedMessage) func(interface{}) bool {
	return func(i interface{}) bool {
		p, ok := i.(**servicebus.ReceivedMessage)
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
	var err error
	for _, id := range ids {
		oneErr := s.sbReceiver.CompleteMessage(ctx, id.(*servicebus.ReceivedMessage), nil)
		if oneErr != nil {
			err = oneErr
		}
	}
	return err
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
	var err error
	for _, id := range ids {
		oneErr := s.sbReceiver.AbandonMessage(ctx, id.(*servicebus.ReceivedMessage), nil)
		if oneErr != nil {
			err = oneErr
		}
	}
	return err
}

// errorCode returns an error code and whether err is retryable.
func errorCode(err error) (gcerrors.ErrorCode, bool) {
	// Unfortunately Azure sometimes returns common.Retryable or even
	// errors.errorString, which don't expose anything other than the error
	// string :-(.
	if strings.Contains(err.Error(), "status code 404") {
		return gcerrors.NotFound, false
	}
	if strings.Contains(err.Error(), "status code 401") {
		return gcerrors.PermissionDenied, false
	}
	var cond amqp.ErrCond
	var aderr *amqp.LinkError
	var aerr *amqp.Error
	if errors.As(err, &aderr) {
		if aderr.RemoteErr == nil {
			return gcerrors.NotFound, false
		}
		cond = aderr.RemoteErr.Condition
	} else if errors.As(err, &aerr) {
		cond = aerr.Condition
	}
	switch cond {
	case amqp.ErrCondNotFound:
		return gcerrors.NotFound, false

	case amqp.ErrCondPreconditionFailed:
		return gcerrors.FailedPrecondition, false

	case amqp.ErrCondInternalError:
		return gcerrors.Internal, true

	case amqp.ErrCondNotImplemented:
		return gcerrors.Unimplemented, false

	case amqp.ErrCondUnauthorizedAccess, amqp.ErrCondNotAllowed:
		return gcerrors.PermissionDenied, false

	case amqp.ErrCondResourceLimitExceeded:
		return gcerrors.ResourceExhausted, true

	case amqp.ErrCondInvalidField:
		return gcerrors.InvalidArgument, false
	}
	return gcerrors.Unknown, true
}

// Close implements driver.Subscription.Close.
func (*subscription) Close() error { return nil }
