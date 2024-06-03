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

// Package gcppubsub provides a pubsub implementation that uses GCP
// PubSub. Use OpenTopic to construct a *pubsub.Topic, and/or OpenSubscription
// to construct a *pubsub.Subscription.
//
// # URLs
//
// For pubsub.OpenTopic and pubsub.OpenSubscription, gcppubsub registers
// for the scheme "gcppubsub".
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
// gcppubsub exposes the following types for As:
//   - Topic: *raw.PublisherClient
//   - Subscription: *raw.SubscriberClient
//   - Message.BeforeSend: *pb.PubsubMessage
//   - Message.AfterSend: *string for the pb.PublishResponse.MessageIds entry corresponding to the message.
//   - Message: *pb.PubsubMessage, *pb.ReceivedMessage
//   - Error: *google.golang.org/grpc/status.Status
package gcppubsub // import "gocloud.dev/pubsub/gcppubsub"

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	raw "cloud.google.com/go/pubsub/apiv1"
	pb "cloud.google.com/go/pubsub/apiv1/pubsubpb"
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
	MaxBatchSize: 1000, // The PubSub service limits the number of messages in a single Publish RPC
	MaxHandlers:  2,
	// The PubSub service limits the size of the request body in a single Publish RPC.
	// The limit is currently documented as "10MB (total size)" and "10MB (data field)" per message.
	// We are enforcing 9MiB to give ourselves some headroom for message attributes since those
	// are currently not considered when computing the byte size of a message.
	MaxBatchByteSize: 9 * 1024 * 1024,
}

var defaultRecvBatcherOpts = &batcher.Options{
	// GCP Pub/Sub returns at most 1000 messages per RPC.
	MaxBatchSize: 1000,
	MaxHandlers:  10,
}

var ackBatcherOpts = &batcher.Options{
	// The PubSub service limits the size of Acknowledge/ModifyAckDeadline RPCs.
	// (E.g., "Request payload size exceeds the limit: 524288 bytes.").
	MaxBatchSize: 1000,
	MaxHandlers:  2,
}

func init() {
	o := new(lazyCredsOpener)
	pubsub.DefaultURLMux().RegisterTopic(Scheme, o)
	pubsub.DefaultURLMux().RegisterSubscription(Scheme, o)
}

// Set holds Wire providers for this package.
var Set = wire.NewSet(
	Dial,
	PublisherClient,
	SubscriberClient,
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

// Scheme is the URL scheme gcppubsub registers its URLOpeners under on pubsub.DefaultMux.
const Scheme = "gcppubsub"

// URLOpener opens GCP Pub/Sub URLs like "gcppubsub://projects/myproject/topics/mytopic" for
// topics or "gcppubsub://projects/myproject/subscriptions/mysub" for subscriptions.
//
// The shortened forms "gcppubsub://myproject/mytopic" for topics or
// "gcppubsub://myproject/mysub" for subscriptions are also supported.
//
// The following query parameters are supported:
//
//   - max_recv_batch_size: sets SubscriptionOptions.MaxBatchSize.
//   - max_send_batch_size: sets TopicOptions.BatcherOptions.MaxBatchSize.
//   - nacklazy: sets SubscriberOptions.NackLazy. The value must be parseable by `strconv.ParseBool`.
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
	pc, err := PublisherClient(ctx, o.Conn)
	if err != nil {
		return nil, err
	}
	topicPath := path.Join(u.Host, u.Path)
	if topicPathRE.MatchString(topicPath) {
		return OpenTopicByPath(pc, topicPath, &opts)
	}
	// Shortened form?
	topicName := strings.TrimPrefix(u.Path, "/")
	return OpenTopic(pc, gcp.ProjectID(u.Host), topicName, &opts), nil
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
		case "nacklazy":
			var err error
			nackLazy, err := queryParameterBool(value)
			if err != nil {
				return nil, fmt.Errorf("open subscription %v: invalid query parameter %q: %v", u, param, err)
			}
			opts.NackLazy = nackLazy
		default:
			return nil, fmt.Errorf("open subscription %v: invalid query parameter %q", u, param)
		}
	}
	sc, err := SubscriberClient(ctx, o.Conn)
	if err != nil {
		return nil, err
	}
	subPath := path.Join(u.Host, u.Path)
	if subscriptionPathRE.MatchString(subPath) {
		return OpenSubscriptionByPath(sc, subPath, &opts)
	}
	// Shortened form?
	subName := strings.TrimPrefix(u.Path, "/")
	return OpenSubscription(sc, gcp.ProjectID(u.Host), subName, &opts), nil
}

type topic struct {
	path   string
	client *raw.PublisherClient
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

// PublisherClient returns a *raw.PublisherClient that can be used in OpenTopic.
func PublisherClient(ctx context.Context, conn *grpc.ClientConn) (*raw.PublisherClient, error) {
	return raw.NewPublisherClient(ctx, option.WithGRPCConn(conn))
}

// SubscriberClient returns a *raw.SubscriberClient that can be used in OpenSubscription.
func SubscriberClient(ctx context.Context, conn *grpc.ClientConn) (*raw.SubscriberClient, error) {
	return raw.NewSubscriberClient(ctx, option.WithGRPCConn(conn))
}

// TopicOptions will contain configuration for topics.
type TopicOptions struct {
	// BatcherOptions adds constraints to the default batching done for sends.
	BatcherOptions batcher.Options
}

// OpenTopic returns a *pubsub.Topic backed by an existing GCP PubSub topic
// in the given projectID. topicName is the last part of the full topic
// path, e.g., "foo" from "projects/<projectID>/topic/foo".
// See the package documentation for an example.
func OpenTopic(client *raw.PublisherClient, projectID gcp.ProjectID, topicName string, opts *TopicOptions) *pubsub.Topic {
	topicPath := fmt.Sprintf("projects/%s/topics/%s", projectID, topicName)
	if opts == nil {
		opts = &TopicOptions{}
	}
	bo := sendBatcherOpts.NewMergedOptions(&opts.BatcherOptions)
	return pubsub.NewTopic(openTopic(client, topicPath), bo)
}

var topicPathRE = regexp.MustCompile("^projects/.+/topics/.+$")

// OpenTopicByPath returns a *pubsub.Topic backed by an existing GCP PubSub
// topic. topicPath must be of the form "projects/<projectID>/topic/<topic>".
// See the package documentation for an example.
func OpenTopicByPath(client *raw.PublisherClient, topicPath string, opts *TopicOptions) (*pubsub.Topic, error) {
	if !topicPathRE.MatchString(topicPath) {
		return nil, fmt.Errorf("invalid topicPath %q; must match %v", topicPath, topicPathRE)
	}
	if opts == nil {
		opts = &TopicOptions{}
	}
	bo := sendBatcherOpts.NewMergedOptions(&opts.BatcherOptions)
	return pubsub.NewTopic(openTopic(client, topicPath), bo), nil
}

// openTopic returns the driver for OpenTopic. This function exists so the test
// harness can get the driver interface implementation if it needs to.
func openTopic(client *raw.PublisherClient, topicPath string) driver.Topic {
	return &topic{topicPath, client}
}

// SendBatch implements driver.Topic.SendBatch.
func (t *topic) SendBatch(ctx context.Context, dms []*driver.Message) error {
	var ms []*pb.PubsubMessage
	for _, dm := range dms {
		psm := &pb.PubsubMessage{Data: dm.Body, Attributes: dm.Metadata}
		if dm.BeforeSend != nil {
			asFunc := func(i interface{}) bool {
				if p, ok := i.(**pb.PubsubMessage); ok {
					*p = psm
					return true
				}
				return false
			}
			if err := dm.BeforeSend(asFunc); err != nil {
				return err
			}
		}
		ms = append(ms, psm)
	}
	req := &pb.PublishRequest{Topic: t.path, Messages: ms}
	pr, err := t.client.Publish(ctx, req)
	if err != nil {
		return err
	}
	if len(pr.MessageIds) == len(dms) {
		for n, dm := range dms {
			if dm.AfterSend != nil {
				asFunc := func(i interface{}) bool {
					if p, ok := i.(*string); ok {
						*p = pr.MessageIds[n]
						return true
					}
					return false
				}
				if err := dm.AfterSend(asFunc); err != nil {
					return err
				}
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
func (t *topic) As(i interface{}) bool {
	c, ok := i.(**raw.PublisherClient)
	if !ok {
		return false
	}
	*c = t.client
	return true
}

// ErrorAs implements driver.Topic.ErrorAs
func (*topic) ErrorAs(err error, i interface{}) bool {
	return errorAs(err, i)
}

func errorAs(err error, i interface{}) bool {
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

type subscription struct {
	client  *raw.SubscriberClient
	path    string
	options *SubscriptionOptions
}

// SubscriptionOptions will contain configuration for subscriptions.
type SubscriptionOptions struct {
	// MaxBatchSize caps the maximum batch size used when retrieving messages. It defaults to 1000.
	MaxBatchSize int

	// NackLazy determines what Nack does.
	//
	// By default, Nack uses ModifyAckDeadline to set the ack deadline
	// for the nacked message to 0, so that it will be redelivered immediately.
	// Set NackLazy to true to bypass this behavior; Nack will do nothing,
	// and the message will be redelivered after the existing ack deadline
	// expires.
	NackLazy bool

	// ReceiveBatcherOptions adds constraints to the default batching done for receives.
	ReceiveBatcherOptions batcher.Options

	// AckBatcherOptions adds constraints to the default batching done for acks.
	AckBatcherOptions batcher.Options
}

// OpenSubscription returns a *pubsub.Subscription backed by an existing GCP
// PubSub subscription subscriptionName in the given projectID. See the package
// documentation for an example.
func OpenSubscription(client *raw.SubscriberClient, projectID gcp.ProjectID, subscriptionName string, opts *SubscriptionOptions) *pubsub.Subscription {
	path := fmt.Sprintf("projects/%s/subscriptions/%s", projectID, subscriptionName)

	dsub := openSubscription(client, path, opts)
	recvOpts := *defaultRecvBatcherOpts
	recvOpts.MaxBatchSize = dsub.options.MaxBatchSize
	rbo := recvOpts.NewMergedOptions(&dsub.options.ReceiveBatcherOptions)
	abo := ackBatcherOpts.NewMergedOptions(&dsub.options.AckBatcherOptions)
	return pubsub.NewSubscription(dsub, rbo, abo)
}

var subscriptionPathRE = regexp.MustCompile("^projects/.+/subscriptions/.+$")

// OpenSubscriptionByPath returns a *pubsub.Subscription backed by an existing
// GCP PubSub subscription. subscriptionPath must be of the form
// "projects/<projectID>/subscriptions/<subscription>".
// See the package documentation for an example.
func OpenSubscriptionByPath(client *raw.SubscriberClient, subscriptionPath string, opts *SubscriptionOptions) (*pubsub.Subscription, error) {
	if !subscriptionPathRE.MatchString(subscriptionPath) {
		return nil, fmt.Errorf("invalid subscriptionPath %q; must match %v", subscriptionPath, subscriptionPathRE)
	}

	dsub := openSubscription(client, subscriptionPath, opts)
	recvOpts := *defaultRecvBatcherOpts
	recvOpts.MaxBatchSize = dsub.options.MaxBatchSize
	rbo := recvOpts.NewMergedOptions(&dsub.options.ReceiveBatcherOptions)
	abo := ackBatcherOpts.NewMergedOptions(&dsub.options.AckBatcherOptions)
	return pubsub.NewSubscription(dsub, rbo, abo), nil
}

// openSubscription returns a driver.Subscription.
func openSubscription(client *raw.SubscriberClient, subscriptionPath string, opts *SubscriptionOptions) *subscription {
	if opts == nil {
		opts = &SubscriptionOptions{}
	}
	if opts.MaxBatchSize == 0 {
		opts.MaxBatchSize = defaultRecvBatcherOpts.MaxBatchSize
	}
	return &subscription{client, subscriptionPath, opts}
}

// ReceiveBatch implements driver.Subscription.ReceiveBatch.
func (s *subscription) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	// Whether to ask Pull to return immediately, or wait for some messages to
	// arrive. If we're making multiple RPCs, we don't want any of them to wait;
	// we might have gotten messages from one of the other RPCs.
	// maxMessages will only be high enough to set this to true in high-throughput
	// situations, so the likelihood of getting 0 messages is small anyway.
	returnImmediately := maxMessages == s.options.MaxBatchSize

	req := &pb.PullRequest{
		Subscription:      s.path,
		ReturnImmediately: returnImmediately,
		MaxMessages:       int32(maxMessages),
	}
	resp, err := s.client.Pull(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(resp.ReceivedMessages) == 0 {
		// If we did happen to get 0 messages, and we didn't ask the server to wait
		// for messages, sleep a bit to avoid spinning.
		if returnImmediately {
			time.Sleep(100 * time.Millisecond)
		}
		return nil, nil
	}

	ms := make([]*driver.Message, 0, len(resp.ReceivedMessages))
	for _, rm := range resp.ReceivedMessages {
		rm := rm
		rmm := rm.Message
		m := &driver.Message{
			LoggableID: rmm.MessageId,
			Body:       rmm.Data,
			Metadata:   rmm.Attributes,
			AckID:      rm.AckId,
			AsFunc:     messageAsFunc(rmm, rm),
		}
		ms = append(ms, m)
	}
	return ms, nil
}

func messageAsFunc(pm *pb.PubsubMessage, rm *pb.ReceivedMessage) func(interface{}) bool {
	return func(i interface{}) bool {
		ip, ok := i.(**pb.PubsubMessage)
		if ok {
			*ip = pm
			return true
		}
		rp, ok := i.(**pb.ReceivedMessage)
		if ok {
			*rp = rm
			return true
		}
		return false
	}
}

// SendAcks implements driver.Subscription.SendAcks.
func (s *subscription) SendAcks(ctx context.Context, ids []driver.AckID) error {
	ids2 := make([]string, 0, len(ids))
	for _, id := range ids {
		ids2 = append(ids2, id.(string))
	}
	return s.client.Acknowledge(ctx, &pb.AcknowledgeRequest{Subscription: s.path, AckIds: ids2})
}

// CanNack implements driver.CanNack.
func (s *subscription) CanNack() bool { return true }

// SendNacks implements driver.Subscription.SendNacks.
func (s *subscription) SendNacks(ctx context.Context, ids []driver.AckID) error {
	if s.options.NackLazy {
		return nil
	}
	ids2 := make([]string, 0, len(ids))
	for _, id := range ids {
		ids2 = append(ids2, id.(string))
	}
	return s.client.ModifyAckDeadline(ctx, &pb.ModifyAckDeadlineRequest{
		Subscription:       s.path,
		AckIds:             ids2,
		AckDeadlineSeconds: 0,
	})
}

// IsRetryable implements driver.Subscription.IsRetryable.
func (s *subscription) IsRetryable(err error) bool {
	// The client mostly handles retries, but does not
	// include DeadlineExceeded for some reason.
	return s.ErrorCode(err) == gcerrors.DeadlineExceeded
}

// As implements driver.Subscription.As.
func (s *subscription) As(i interface{}) bool {
	c, ok := i.(**raw.SubscriberClient)
	if !ok {
		return false
	}
	*c = s.client
	return true
}

// ErrorAs implements driver.Subscription.ErrorAs
func (*subscription) ErrorAs(err error, i interface{}) bool {
	return errorAs(err, i)
}

func (*subscription) ErrorCode(err error) gcerrors.ErrorCode {
	return gcerr.GRPCCode(err)
}

// Close implements driver.Subscription.Close.
func (*subscription) Close() error { return nil }

func queryParameterInt(value []string) (int, error) {
	if len(value) > 1 {
		return 0, fmt.Errorf("expected only one parameter value, got: %v", len(value))
	}

	return strconv.Atoi(value[0])
}

func queryParameterBool(value []string) (bool, error) {
	if len(value) > 1 {
		return false, fmt.Errorf("expected only one parameter value, got: %v", len(value))
	}

	return strconv.ParseBool(value[0])
}
