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
// URLs
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
// Message Delivery Semantics
//
// GCP Pub/Sub supports at-least-once semantics; applications must
// call Message.Ack after processing a message, or it will be redelivered.
// See https://godoc.org/gocloud.dev/pubsub#hdr-At_most_once_and_At_least_once_Delivery
// for more background.
//
// As
//
// gcppubsub exposes the following types for As:
//  - Topic: *raw.PublisherClient
//  - Subscription: *raw.SubscriberClient
//  - Message.BeforeSend: *pb.PubsubMessage
//  - Message: *pb.PubsubMessage
//  - Error: *google.golang.org/grpc/status.Status
package gcppubsub // import "gocloud.dev/pubsub/gcppubsub"

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	raw "cloud.google.com/go/pubsub/apiv1"
	"github.com/google/wire"
	"gocloud.dev/gcerrors"
	"gocloud.dev/gcp"
	"gocloud.dev/internal/batcher"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/internal/useragent"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
	"google.golang.org/api/option"
	pb "google.golang.org/genproto/googleapis/pubsub/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
	"google.golang.org/grpc/status"
)

const endPoint = "pubsub.googleapis.com:443"

var sendBatcherOpts = &batcher.Options{
	MaxBatchSize: 1000, // The PubSub service limits the number of messages in a single Publish RPC
	MaxHandlers:  2,
}

var recvBatcherOpts = &batcher.Options{
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
	SubscriptionOptions{},
	TopicOptions{},
	URLOpener{},
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
		creds, err := gcp.DefaultCredentials(ctx)
		if err != nil {
			o.err = err
			return
		}
		conn, _, err := Dial(ctx, creds.TokenSource)
		if err != nil {
			o.err = err
			return
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

// URLOpener opens GCP Pub/Sub URLs like "gcppubsub://myproject/mytopic" for
// topics or "gcppubsub://myproject/mysub" for subscriptions.
//
// The URL's host is used as the projectID, and the URL's path (with the
// leading "/" trimmed) is used as the topic or subscription name.
//
// No URL parameters are supported.
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
	for param := range u.Query() {
		return nil, fmt.Errorf("open topic %v: invalid query parameter %q", u, param)
	}
	pc, err := PublisherClient(ctx, o.Conn)
	if err != nil {
		return nil, err
	}
	topicName := strings.TrimPrefix(u.Path, "/")
	return OpenTopic(pc, gcp.ProjectID(u.Host), topicName, &o.TopicOptions), nil
}

// OpenSubscriptionURL opens a pubsub.Subscription based on u.
func (o *URLOpener) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*pubsub.Subscription, error) {
	for param := range u.Query() {
		return nil, fmt.Errorf("open subscription %v: invalid query parameter %q", u, param)
	}
	sc, err := SubscriberClient(ctx, o.Conn)
	if err != nil {
		return nil, err
	}
	subName := strings.TrimPrefix(u.Path, "/")
	return OpenSubscription(sc, gcp.ProjectID(u.Host), subName, &o.SubscriptionOptions), nil
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
		useragent.GRPCDialOption("pubsub"),
	)
	if err != nil {
		return nil, nil, err
	}
	return conn, func() { conn.Close() }, nil
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
type TopicOptions struct{}

// OpenTopic returns a *pubsub.Topic backed by an existing GCP PubSub topic
// topicName in the given projectID. See the package documentation for an
// example.
func OpenTopic(client *raw.PublisherClient, proj gcp.ProjectID, topicName string, opts *TopicOptions) *pubsub.Topic {
	return pubsub.NewTopic(openTopic(client, proj, topicName), sendBatcherOpts)
}

// openTopic returns the driver for OpenTopic. This function exists so the test
// harness can get the driver interface implementation if it needs to.
func openTopic(client *raw.PublisherClient, proj gcp.ProjectID, topicName string) driver.Topic {
	path := fmt.Sprintf("projects/%s/topics/%s", proj, topicName)
	return &topic{path, client}
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
	if _, err := t.client.Publish(ctx, req); err != nil {
		return err
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
	client *raw.SubscriberClient
	path   string
}

// SubscriptionOptions will contain configuration for subscriptions.
type SubscriptionOptions struct{}

// OpenSubscription returns a *pubsub.Subscription backed by an existing GCP
// PubSub subscription subscriptionName in the given projectID. See the package
// documentation for an example.
func OpenSubscription(client *raw.SubscriberClient, proj gcp.ProjectID, subscriptionName string, opts *SubscriptionOptions) *pubsub.Subscription {
	ds := openSubscription(client, proj, subscriptionName)
	return pubsub.NewSubscription(ds, nil, ackBatcherOpts)
}

// openSubscription returns a driver.Subscription.
func openSubscription(client *raw.SubscriberClient, projectID gcp.ProjectID, subscriptionName string) driver.Subscription {
	path := fmt.Sprintf("projects/%s/subscriptions/%s", projectID, subscriptionName)
	return &subscription{client, path}
}

// ReceiveBatch implements driver.Subscription.ReceiveBatch.
func (s *subscription) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	// Whether to ask Pull to return immediately, or wait for some messages to
	// arrive. If we're making multiple RPCs, we don't want any of them to wait;
	// we might have gotten messages from one of the other RPCs.
	// maxMessages will only be high enough to set this to true in high-throughput
	// situations, so the likelihood of getting 0 messages is small anyway.
	returnImmediately := maxMessages == recvBatcherOpts.MaxBatchSize

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
		rmm := rm.Message
		m := &driver.Message{
			Body:     rmm.Data,
			Metadata: rmm.Attributes,
			AckID:    rm.AckId,
			AsFunc:   messageAsFunc(rmm),
		}
		ms = append(ms, m)
	}
	return ms, nil
}

func messageAsFunc(pm *pb.PubsubMessage) func(interface{}) bool {
	return func(i interface{}) bool {
		p, ok := i.(**pb.PubsubMessage)
		if !ok {
			return false
		}
		*p = pm
		return true
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
func (s *subscription) IsRetryable(error) bool {
	// The client handles retries.
	return false
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
