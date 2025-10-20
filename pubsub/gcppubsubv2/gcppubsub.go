// Copyright 2025 The Go Cloud Development Kit Authors
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

// Package gcppubsub provides a pubsub implementation that uses GCP
// PubSub. Use OpenTopic to construct a *pubsub.Topic, and/or OpenSubscription
// to construct a *pubsub.Subscription.
//
// # URLs
//
// For pubsub.OpenTopic and pubsub.OpenSubscription, gcppubsubv2 registers
// for the scheme "gcppubsubv2".
// The default URL opener will creating a connection using use default
// credentials from the environment, as described in
// https://cloud.google.com/docs/authentication/production.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// GCP Pub/Sub emulator is supported as per https://cloud.google.com/pubsub/docs/emulator
// So, when environment variable 'PUBSUB_EMULATOR_HOST' is set
// driver connects to the specified emulator host by default.
//
// # Message Delivery Semantics
//
// GCP Pub/Sub supports at-least-once semantics; applications must
// call Message.Ack after processing a message, or it will be redelivered.
// See https://godoc.org/gocloud.dev/pubsub#hdr-At_most_once_and_At_least_once_Delivery
// for more background.
//
// # As
//
// gcppubsubv2 exposes the following types for As:
//   - Topic: *raw.Publisher
//   - Subscription: *raw.Subscriber
//   - Message.BeforeSend: *raw.Message
//   - Message.AfterSend: *string for the raw.PublishResult serverID corresponding to the message.
//   - Message: *raw.Message
//   - Error: *google.golang.org/grpc/status.Status
package gcppubsubv2 // import "gocloud.dev/pubsub/gcppubsubv2"

import (
	"context"
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

	raw "cloud.google.com/go/pubsub/v2"
	"github.com/google/wire"
	"gocloud.dev/gcerrors"
	"gocloud.dev/gcp"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/internal/useragent"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/batcher"
	"gocloud.dev/pubsub/driver"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/credentials/oauth"
	"google.golang.org/grpc/status"
)

var endPoint = "pubsub.googleapis.com:443"

var sendBatcherOpts = &batcher.Options{
	// The underlying library does its own batching, so for throughput
	// what we pick here doesn't matter much. It's simpler and more
	// likely to elicit good behavior from the underlying library to
	// do one message at a time. It also results in clearer errors,
	// at least back to the concrete type, because returning an error
	// from SendBatch when some messages were sent and others weren't
	// is always a bit unfortunate.
	MaxBatchSize: 1,
	MaxHandlers:  1000,
}

var defaultRecvBatcherOpts = &batcher.Options{
	// Single threaded; v2 wants only a single Receive. We get concurrency
	// via concurrent callbacks; the resulting messages are buffered
	// in memory until they can be returned via ReceiveBatch. When that
	// happens, we might as well return a lot of them.
	MaxBatchSize: 5000,
	MaxHandlers:  1,
}

var ackBatcherOpts = &batcher.Options{
	// Similar to SendBatch.
	MaxBatchSize: 1,
	MaxHandlers:  1000,
}

func init() {
	o := new(lazyCredsOpener)
	pubsub.DefaultURLMux().RegisterTopic(Scheme, o)
	pubsub.DefaultURLMux().RegisterSubscription(Scheme, o)
}

// Set holds Wire providers for this package.
var Set = wire.NewSet(
	Dial,
	Client,
	wire.Struct(new(SubscriptionOptions)),
	wire.Struct(new(TopicOptions)),
	wire.Struct(new(URLOpener), "Conn", "TopicOptions", "SubscriptionOptions"),
)

// lazyCredsOpener obtains Application Default Credentials on the first call
// to OpenTopicURL/OpenSubscriptionURL.
type lazyCredsOpener struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

func (o *lazyCredsOpener) defaultConn(ctx context.Context) (*URLOpener, error) {
	o.init.Do(func() {
		var conn *grpc.ClientConn
		var err error
		if e := os.Getenv("PUBSUB_EMULATOR_HOST"); e != "" {
			// Connect to the GCP pubsub emulator by overriding the default endpoint
			// if the 'PUBSUB_EMULATOR_HOST' environment variable is set.
			// Check https://cloud.google.com/pubsub/docs/emulator for more info.
			endPoint = e
			conn, err = dialEmulator(ctx, e)
			if err != nil {
				o.err = err
				return
			}
		} else {
			creds, err := gcp.DefaultCredentials(ctx)
			if err != nil {
				o.err = err
				return
			}

			conn, _, err = Dial(ctx, creds.TokenSource)
			if err != nil {
				o.err = err
				return
			}
		}
		o.opener = &URLOpener{Conn: conn}
	})
	return o.opener, o.err
}

func (o *lazyCredsOpener) OpenTopicURL(ctx context.Context, u *url.URL) (*pubsub.Topic, error) {
	opener, err := o.defaultConn(ctx)
	if err != nil {
		return nil, fmt.Errorf("open topic %v: failed to open default connection: %v", u, err)
	}
	return opener.OpenTopicURL(ctx, u)
}

func (o *lazyCredsOpener) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*pubsub.Subscription, error) {
	opener, err := o.defaultConn(ctx)
	if err != nil {
		return nil, fmt.Errorf("open subscription %v: failed to open default connection: %v", u, err)
	}
	return opener.OpenSubscriptionURL(ctx, u)
}

// Scheme is the URL scheme gcppubsubv2 registers its URLOpeners under on pubsub.DefaultMux.
const Scheme = "gcppubsubv2"

// URLOpener opens GCP Pub/Sub URLs like "gcppubsubv2://projects/myproject/topics/mytopic" for
// topics or "gcppubsubv2://projects/myproject/subscriptions/mysub" for subscriptions.
//
// The shortened forms "gcppubsubv2://myproject/mytopic" for topics or
// "gcppubsubv2://myproject/mysub" for subscriptions are also supported.
//
// The following query parameters are supported:
//
//   - max_recv_batch_size: sets SubscriptionOptions.MaxBatchSize.
//   - max_send_batch_size: sets TopicOptions.BatcherOptions.MaxBatchSize.
//
// Currently their use is limited to subscribers.
type URLOpener struct {
	// Conn must be set to a non-nil ClientConn authenticated with
	// Cloud Pub/Sub scope or equivalent.
	Conn *grpc.ClientConn

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
		case "max_send_batch_size":
			maxBatchSize, err := queryParameterInt(value)
			if err != nil {
				return nil, fmt.Errorf("open topic %v: invalid query parameter %q: %v", u, param, err)
			}

			if maxBatchSize <= 0 || maxBatchSize > 1000 {
				return nil, fmt.Errorf("open topic %v: invalid query parameter %q: must be between 1 and 1000", u, param)
			}

			opts.BatcherOptions.MaxBatchSize = maxBatchSize
		default:
			return nil, fmt.Errorf("open topic %v: invalid query parameter %q", u, param)
		}
	}
	client, err := Client(ctx, gcp.ProjectID(u.Host), o.Conn)
	if err != nil {
		return nil, err
	}
	topicPath := path.Join(u.Host, u.Path)
	if topicPathRE.MatchString(topicPath) {
		return OpenTopicByPath(client, topicPath, &opts)
	}
	// Shortened form?
	topicName := strings.TrimPrefix(u.Path, "/")
	return OpenTopic(client, topicName, &opts), nil
}

// OpenSubscriptionURL opens a pubsub.Subscription based on u.
func (o *URLOpener) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*pubsub.Subscription, error) {
	// Set subscription options to use defaults
	opts := o.SubscriptionOptions

	for param, value := range u.Query() {
		switch param {
		case "max_recv_batch_size":
			maxBatchSize, err := queryParameterInt(value)
			if err != nil {
				return nil, fmt.Errorf("open subscription %v: invalid query parameter %q: %v", u, param, err)
			}

			if maxBatchSize <= 0 || maxBatchSize > 1000 {
				return nil, fmt.Errorf("open subscription %v: invalid query parameter %q: must be between 1 and 1000", u, param)
			}

			opts.MaxBatchSize = maxBatchSize
		default:
			return nil, fmt.Errorf("open subscription %v: invalid query parameter %q", u, param)
		}
	}
	client, err := Client(ctx, gcp.ProjectID(u.Host), o.Conn)
	if err != nil {
		return nil, err
	}
	subPath := path.Join(u.Host, u.Path)
	if subscriptionPathRE.MatchString(subPath) {
		return OpenSubscriptionByPath(client, subPath, &opts)
	}
	// Shortened form?
	subName := strings.TrimPrefix(u.Path, "/")
	return OpenSubscription(client, subName, &opts), nil
}

type topic struct {
	publisher *raw.Publisher
}

// Dial opens a gRPC connection to the GCP Pub Sub API.
//
// The second return value is a function that can be called to clean up
// the connection opened by Dial.
func Dial(ctx context.Context, ts gcp.TokenSource) (*grpc.ClientConn, func(), error) {
	conn, err := grpc.DialContext(ctx, endPoint,
		grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")),
		grpc.WithPerRPCCredentials(oauth.TokenSource{TokenSource: ts}),
		// The default message size limit for gRPC is 4MB, while GCP
		// PubSub supports messages up to 10MB. Aside from the message itself
		// there is also other data in the gRPC response, bringing the maximum
		// response size above 10MB. Tell gRPC to support up to 11MB.
		// https://github.com/googleapis/google-cloud-node/issues/1991
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(1024*1024*11)),
		useragent.GRPCDialOption("pubsub"),
	)
	if err != nil {
		return nil, nil, err
	}
	return conn, func() { conn.Close() }, nil
}

// dialEmulator opens a gRPC connection to the GCP Pub Sub API.
func dialEmulator(ctx context.Context, e string) (*grpc.ClientConn, error) {
	conn, err := grpc.DialContext(ctx, e,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		useragent.GRPCDialOption("pubsub"))
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// Client returns a *raw.Client that can be used in OpenTopic and/or OpenSubscription.
// conn is optional.
func Client(ctx context.Context, projectID gcp.ProjectID, conn *grpc.ClientConn) (*raw.Client, error) {
	if conn == nil {
		return raw.NewClient(ctx, string(projectID))
	}
	return raw.NewClient(ctx, string(projectID), option.WithGRPCConn(conn))
}

// TopicOptions will contain configuration for topics.
type TopicOptions struct {
	// BatcherOptions adds constraints to the default batching done for sends.
	BatcherOptions batcher.Options
}

// OpenTopic returns a *pubsub.Topic backed by an existing GCP PubSub topic
// topicName is the last part of the full topic path, e.g., "foo" from "projects/<projectID>/topic/foo".
// See the package documentation for an example.
func OpenTopic(client *raw.Client, topicName string, opts *TopicOptions) *pubsub.Topic {
	publisher := client.Publisher(topicName)
	if opts == nil {
		opts = &TopicOptions{}
	}
	bo := sendBatcherOpts.NewMergedOptions(&opts.BatcherOptions)
	return pubsub.NewTopic(openTopic(publisher), bo)
}

var topicPathRE = regexp.MustCompile("^projects/.+/topics/(.+)$")

// OpenTopicByPath returns a *pubsub.Topic backed by an existing GCP PubSub
// topic. topicPath must be of the form "projects/<projectID>/topic/<topic>".
// See the package documentation for an example.
func OpenTopicByPath(client *raw.Client, topicPath string, opts *TopicOptions) (*pubsub.Topic, error) {
	matches := topicPathRE.FindStringSubmatch(topicPath)
	if len(matches) != 2 {
		return nil, fmt.Errorf("invalid topicPath %q; must match %v", topicPath, topicPathRE)
	}
	topicName := matches[1]
	return OpenTopic(client, topicName, opts), nil
}

// openTopic returns the driver for OpenTopic. This function exists so the test
// harness can get the driver interface implementation if it needs to.
func openTopic(publisher *raw.Publisher) driver.Topic {
	return &topic{publisher}
}

// SendBatch implements driver.Topic.SendBatch.
func (t *topic) SendBatch(ctx context.Context, dms []*driver.Message) error {
	var results []*raw.PublishResult
	for _, dm := range dms {
		psm := &raw.Message{Data: dm.Body, Attributes: dm.Metadata}
		if dm.BeforeSend != nil {
			asFunc := func(i any) bool {
				if p, ok := i.(**raw.Message); ok {
					*p = psm
					return true
				}
				return false
			}
			if err := dm.BeforeSend(asFunc); err != nil {
				return err
			}
		}
		// Publish always returns immediately; the return value's Get
		// function blocks until there's a result.
		results = append(results, t.publisher.Publish(ctx, psm))
	}
	for n, dm := range dms {
		pr := results[n]
		messageID, err := pr.Get(ctx)
		if err != nil {
			return err
		}
		if dm.AfterSend != nil {
			asFunc := func(i any) bool {
				if p, ok := i.(*string); ok {
					*p = messageID
					return true
				}
				return false
			}
			if err := dm.AfterSend(asFunc); err != nil {
				return err
			}
		}
	}
	return nil
}

// IsRetryable implements driver.Topic.IsRetryable.
func (t *topic) IsRetryable(error) bool {
	// The client handles retries.
	return false
}

// As implements driver.Topic.As.
func (t *topic) As(i any) bool {
	c, ok := i.(**raw.Publisher)
	if !ok {
		return false
	}
	*c = t.publisher
	return true
}

// ErrorAs implements driver.Topic.ErrorAs
func (*topic) ErrorAs(err error, i any) bool {
	return errorAs(err, i)
}

func errorAs(err error, i any) bool {
	s, ok := status.FromError(err)
	if !ok {
		return false
	}
	p, ok := i.(**status.Status)
	if !ok {
		return false
	}
	*p = s
	return true
}

func (*topic) ErrorCode(err error) gcerrors.ErrorCode {
	return gcerr.GRPCCode(err)
}

// Close implements driver.Topic.Close.
func (*topic) Close() error { return nil }

type ackableMsg struct {
	rawm *raw.Message
	ch   chan bool
}

type subscription struct {
	subscriber *raw.Subscriber
	options    *SubscriptionOptions

	// Fields for managing the single Receive RPC called in the background.
	receiving     bool            // true when there's an active RPC
	receiveCtx    context.Context // the single background context used when calling Receive
	receiveCancel func()          // cancel func for receiveCtx; called during Close

	// Fields used for managing buffered messages.
	mu         sync.Mutex
	dms        []*driver.Message            // buffered messages for next ReceiveBatch
	receiveErr error                        // error returned by last Receive, to be returned by next ReceiveBatch
	acks       map[driver.AckID]*ackableMsg // returned messages that haven't yet been acked/nacked
}

// SubscriptionOptions will contain configuration for subscriptions.
type SubscriptionOptions struct {
	// MaxBatchSize caps the maximum batch size used when retrieving messages. It defaults to 1000.
	MaxBatchSize int

	// ReceiveBatcherOptions adds constraints to the default batching done for receives.
	ReceiveBatcherOptions batcher.Options

	// AckBatcherOptions adds constraints to the default batching done for acks.
	AckBatcherOptions batcher.Options
}

// OpenSubscription returns a *pubsub.Subscription backed by an existing GCP
// PubSub subscription subscriptionName. See the package documentation for an example.
func OpenSubscription(client *raw.Client, subscriptionName string, opts *SubscriptionOptions) *pubsub.Subscription {
	subscriber := client.Subscriber(subscriptionName)
	dsub := openSubscription(subscriber, opts)
	recvOpts := *defaultRecvBatcherOpts
	recvOpts.MaxBatchSize = dsub.options.MaxBatchSize
	rbo := recvOpts.NewMergedOptions(&dsub.options.ReceiveBatcherOptions)
	abo := ackBatcherOpts.NewMergedOptions(&dsub.options.AckBatcherOptions)
	return pubsub.NewSubscription(dsub, rbo, abo)
}

var subscriptionPathRE = regexp.MustCompile("^projects/.+/subscriptions/(.+)$")

// OpenSubscriptionByPath returns a *pubsub.Subscription backed by an existing
// GCP PubSub subscription. subscriptionPath must be of the form
// "projects/<projectID>/subscriptions/<subscription>".
// See the package documentation for an example.
func OpenSubscriptionByPath(client *raw.Client, subscriptionPath string, opts *SubscriptionOptions) (*pubsub.Subscription, error) {
	matches := subscriptionPathRE.FindStringSubmatch(subscriptionPath)
	if len(matches) != 2 {
		return nil, fmt.Errorf("invalid subscriptionPath %q; must match %v", subscriptionPath, subscriptionPathRE)
	}
	subscriptionName := matches[1]
	return OpenSubscription(client, subscriptionName, opts), nil
}

// openSubscription returns a driver.Subscription.
func openSubscription(subscriber *raw.Subscriber, opts *SubscriptionOptions) *subscription {
	if opts == nil {
		opts = &SubscriptionOptions{}
	}
	if opts.MaxBatchSize == 0 {
		opts.MaxBatchSize = defaultRecvBatcherOpts.MaxBatchSize
	}
	// Construct a context that's used (repeatedly if necessary) to call Receive.
	// It never expires; the Receive may last forever.
	// The cancel function is used during shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	return &subscription{
		subscriber:    subscriber,
		options:       opts,
		receiving:     false,
		receiveCtx:    ctx,
		receiveCancel: cancel,
		acks:          map[driver.AckID]*ackableMsg{},
	}
}

// ReceiveBatch implements driver.Subscription.ReceiveBatch.
func (s *subscription) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.receiving {
		// Start up the goroutine that calls Receive.
		s.receiving = true
		go func() {
			// Receive blocks until receiveCtx ends, which will only happen during shutdown,
			// or if there's a fatal error.
			// During normal use, this goroutine will sit here in Receive,
			// the underlying library will call the callback as messages arrive,
			// the callback implementation will add them to s.dms, and
			// ReceiveBatch will pull them off.
			//
			// The underlying library *requires* that Ack or Nack be called inside the
			// callback. Therefore, after adding the message to s.dms, the callback
			// (which is running in its own goroutine from the underlying library)
			// creates a channel and waits on it; Ack/Nack will write true/false to that
			// channel, allowing the callback to finally return.
			err := s.subscriber.Receive(s.receiveCtx, func(ctx context.Context, rawm *raw.Message) {
				dm := &driver.Message{
					LoggableID: rawm.ID,
					Body:       rawm.Data,
					Metadata:   rawm.Attributes,
					AckID:      driver.AckID(rawm.ID),
					AsFunc: func(i any) bool {
						rm, ok := i.(**raw.Message)
						if ok {
							*rm = rawm
							return true
						}
						return false
					},
				}
				// As described above, ackCh will be used to wait here until
				// Ack or Nack is called.
				ackCh := make(chan bool)
				s.mu.Lock()
				s.acks[rawm.ID] = &ackableMsg{rawm, ackCh}
				s.dms = append(s.dms, dm)
				s.mu.Unlock()
				isAck := <-ackCh
				if isAck {
					rawm.Ack()
				} else {
					rawm.Nack()
				}
			})
			// Receive returned. Either we're shutting down, or there's a permanent error.
			// Either way, reset so that if ReceiveBatch is called again, we'll retry.
			s.mu.Lock()
			if !errors.Is(err, context.Canceled) {
				s.receiveErr = err
			}
			s.receiving = false
			s.mu.Unlock()
		}()
	}

	// Return up to maxMessages from the buffer.
	var dms []*driver.Message
	if err := s.receiveErr; err != nil {
		s.receiveErr = nil
		return nil, err
	}
	if len(s.dms) <= maxMessages {
		dms = s.dms
		s.dms = nil
	} else {
		dms = s.dms[0:maxMessages]
		s.dms = s.dms[maxMessages:]
	}
	if len(dms) == 0 {
		time.Sleep(100 * time.Millisecond)
		return nil, nil
	}
	return dms, nil
}

// SendAcks implements driver.Subscription.SendAcks.
func (s *subscription) SendAcks(ctx context.Context, ids []driver.AckID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		if ackable := s.acks[id]; ackable != nil {
			ackable.ch <- true
			close(ackable.ch)
			delete(s.acks, id)
		}
	}
	return nil
}

// CanNack implements driver.CanNack.
func (s *subscription) CanNack() bool { return true }

// SendNacks implements driver.Subscription.SendNacks.
func (s *subscription) SendNacks(ctx context.Context, ids []driver.AckID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		if ackable := s.acks[id]; ackable != nil {
			ackable.ch <- false
			close(ackable.ch)
			delete(s.acks, id)
		}
	}
	return nil
}

// IsRetryable implements driver.Subscription.IsRetryable.
func (s *subscription) IsRetryable(err error) bool {
	// The client mostly handles retries, but does not
	// include DeadlineExceeded for some reason.
	return s.ErrorCode(err) == gcerrors.DeadlineExceeded
}

// As implements driver.Subscription.As.
func (s *subscription) As(i any) bool {
	c, ok := i.(**raw.Subscriber)
	if !ok {
		return false
	}
	*c = s.subscriber
	return true
}

// ErrorAs implements driver.Subscription.ErrorAs
func (*subscription) ErrorAs(err error, i any) bool {
	return errorAs(err, i)
}

func (*subscription) ErrorCode(err error) gcerrors.ErrorCode {
	return gcerr.GRPCCode(err)
}

// Close implements driver.Subscription.Close.
func (s *subscription) Close() error {
	s.receiveCancel()
	return nil
}

func queryParameterInt(value []string) (int, error) {
	if len(value) > 1 {
		return 0, fmt.Errorf("expected only one parameter value, got: %v", len(value))
	}

	return strconv.Atoi(value[0])
}
