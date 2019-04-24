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

// Package kafkapubsub provides an implementation of pubsub for Kafka.
// It requires a minimum Kafka version of 0.11.x for Header support.
// Some functionality may work with earlier versions of Kafka.
//
// See https://kafka.apache.org/documentation.html#semantics for a discussion
// of message semantics in Kafka. sarama.Config exposes many knobs that
// can affect performance and semantics, so review and set them carefully.
//
// kafkapubsub does not support Message.Nack; Message.Nackable will return
// false, and Message.Nack will panic if called.
//
// URLs
//
// For pubsub.OpenTopic and pubsub.OpenSubscription, kafkapubsub registers
// for the scheme "kafka".
// The default URL opener will connect to a default set of Kafka brokers based
// on the environment variable "KAFKA_BROKERS", expected to be a comma-delimited
// set of server addresses.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://godoc.org/gocloud.dev#hdr-URLs for background information.
//
// Escaping
//
// Go CDK supports all UTF-8 strings. No escaping is required for Kafka.
// Message metadata is supported through Kafka Headers, which allow arbitrary
// []byte for both key and value. These are converted to string for use in
// Message.Metadata.
//
// As
//
// kafkapubsub exposes the following types for As:
//  - Topic: TODO(rvangent)
//  - Subscription: TODO(rvangent)
//  - Message: TODO(rvangent)
//  - Error: TODO(rvangent)
package kafkapubsub // import "gocloud.dev/pubsub/kafkapubsub"

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

	"github.com/Shopify/sarama"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/batcher"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
)

var sendBatcherOpts = &batcher.Options{
	MaxBatchSize: 100,
	MaxHandlers:  2,
}

var recvBatcherOpts = &batcher.Options{
	MaxBatchSize: 100,
	MaxHandlers:  2,
}

func init() {
	opener := new(defaultOpener)
	pubsub.DefaultURLMux().RegisterTopic(Scheme, opener)
	pubsub.DefaultURLMux().RegisterSubscription(Scheme, opener)
}

// defaultOpener create a default opener.
type defaultOpener struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

func (o *defaultOpener) defaultOpener() (*URLOpener, error) {
	o.init.Do(func() {
		brokerList := os.Getenv("KAFKA_BROKERS")
		if brokerList == "" {
			o.err = errors.New("KAFKA_BROKERS environment variable not set")
			return
		}
		brokers := strings.Split(brokerList, ",")
		for i, b := range brokers {
			brokers[i] = strings.TrimSpace(b)
		}
		o.opener = &URLOpener{
			Brokers: brokers,
			Config:  MinimalConfig(),
		}
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

// Scheme is the URL scheme that kafkapubsub registers its URLOpeners under on pubsub.DefaultMux.
const Scheme = "kafka"

// URLOpener opens Kafka URLs like "kafka://mytopic" for topics and
// "kafka://group?topic=mytopic" for subscriptions.
//
// For topics, the URL's host+path is used as the topic name.
//
// For subscriptions, the URL's host+path is used as the group name,
// and the "topic" query parameter(s) are used as the set of topics to
// subscribe to.
type URLOpener struct {
	// Brokers is the slice of brokers in the Kafka cluster.
	Brokers []string
	// Config is the Sarama Config.
	// Config.Producer.Return.Success must be set to true.
	Config *sarama.Config

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
	topicName := path.Join(u.Host, u.Path)
	return OpenTopic(o.Brokers, o.Config, topicName, &o.TopicOptions)
}

// OpenSubscriptionURL opens a pubsub.Subscription based on u.
func (o *URLOpener) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*pubsub.Subscription, error) {
	q := u.Query()
	topics := q["topic"]
	q.Del("topic")
	for param := range q {
		return nil, fmt.Errorf("open subscription %v: invalid query parameter %q", u, param)
	}
	group := path.Join(u.Host, u.Path)
	return OpenSubscription(o.Brokers, o.Config, group, topics, &o.SubscriptionOptions)
}

// MinimalConfig returns a minimal sarama.Config.
func MinimalConfig() *sarama.Config {
	config := sarama.NewConfig()
	config.Version = sarama.V0_11_0_0       // required for Headers
	config.Producer.Return.Successes = true // required for SyncProducer
	return config
}

type topic struct {
	producer  sarama.SyncProducer
	topicName string
	opts      TopicOptions
}

// TopicOptions contains configuration options for topics.
type TopicOptions struct {
	// KeyName optionally sets the Message.Metadata key to use as the optional
	// Kafka message key. If set, and if a matching Message.Metadata key is found,
	// the value for that key will be used as the message key when sending to
	// Kafka, instead of being added to the message headers.
	KeyName string
}

// OpenTopic creates a pubsub.Topic that sends to a Kafka topic.
//
// It uses a sarama.SyncProducer to send messages. Producer options can
// be configured in the Producer section of the sarama.Config:
// https://godoc.org/github.com/Shopify/sarama#Config.
//
// Config.Producer.Return.Success must be set to true.
func OpenTopic(brokers []string, config *sarama.Config, topicName string, opts *TopicOptions) (*pubsub.Topic, error) {
	dt, err := openTopic(brokers, config, topicName, opts)
	if err != nil {
		return nil, err
	}
	return pubsub.NewTopic(dt, sendBatcherOpts), nil
}

// openTopic returns the driver for OpenTopic. This function exists so the test
// harness can get the driver interface implementation if it needs to.
func openTopic(brokers []string, config *sarama.Config, topicName string, opts *TopicOptions) (driver.Topic, error) {
	if opts == nil {
		opts = &TopicOptions{}
	}
	producer, err := sarama.NewSyncProducer(brokers, config)
	if err != nil {
		return nil, err
	}
	return &topic{producer: producer, topicName: topicName, opts: *opts}, nil
}

// SendBatch implements driver.Topic.SendBatch.
func (t *topic) SendBatch(ctx context.Context, dms []*driver.Message) error {
	// Convert the messages to a slice of sarama.ProducerMessage.
	ms := make([]*sarama.ProducerMessage, 0, len(dms))
	for _, dm := range dms {
		var kafkaKey []byte
		var headers []sarama.RecordHeader
		for k, v := range dm.Metadata {
			if k == t.opts.KeyName {
				// Use this key's value as the Kafka message key instead of adding it
				// to the headers.
				kafkaKey = []byte(v)
			} else {
				headers = append(headers, sarama.RecordHeader{Key: []byte(k), Value: []byte(v)})
			}
		}
		ms = append(ms, &sarama.ProducerMessage{
			Topic:   t.topicName,
			Key:     sarama.ByteEncoder(kafkaKey),
			Value:   sarama.ByteEncoder(dm.Body),
			Headers: headers,
		})
		// TODO(rvangent): Call dm.AsFunc.
	}
	return t.producer.SendMessages(ms)
}

// Close implements io.Closer.
func (t *topic) Close() error {
	return t.producer.Close()
}

// IsRetryable implements driver.Topic.IsRetryable.
func (t *topic) IsRetryable(error) bool {
	// TODO(rvangent): Investigate.
	return false
}

// As implements driver.Topic.As.
func (t *topic) As(i interface{}) bool {
	// TODO(rvangent): Implement.
	return false
}

// ErrorAs implements driver.Topic.ErrorAs.
func (t *topic) ErrorAs(err error, i interface{}) bool {
	return errorAs(err, i)
}

// ErrorCode implements driver.Topic.ErrorCode.
func (t *topic) ErrorCode(err error) gcerrors.ErrorCode {
	return errorCode(err)
}

func errorCode(err error) gcerrors.ErrorCode {
	// TODO(rvangent): I suspect it will be easier to use errorAs here.
	if pes, ok := err.(sarama.ProducerErrors); ok && len(pes) == 1 {
		return errorCode(pes[0])
	}
	if pe, ok := err.(*sarama.ProducerError); ok {
		return errorCode(pe.Err)
	}
	if err == sarama.ErrUnknownTopicOrPartition {
		return gcerr.NotFound
	}
	return gcerr.Unknown
}

type subscription struct {
	opts     SubscriptionOptions
	ch       chan *sarama.ConsumerMessage // for received messages
	closeCh  chan struct{}                // closed when we've shut down
	joinCh   chan struct{}                // closed when we join for the first time
	cancel   func()                       // cancels the background consumer
	closeErr error                        // fatal error detected by the background consumer

	mu      sync.Mutex
	unacked []*ackInfo
	sess    sarama.ConsumerGroupSession // current session, if any, used for marking offset updates
}

// ackInfo stores info about a message and whether it has been acked.
// It is used as the driver.AckID.
type ackInfo struct {
	msg   *sarama.ConsumerMessage
	acked bool
}

// SubscriptionOptions contains configuration for subscriptions.
type SubscriptionOptions struct {
	// KeyName optionally sets the Message.Metadata key in which to store the
	// Kafka message key. If set, and if the Kafka message key is non-empty,
	// the key value will be stored in Message.Metadata under KeyName.
	KeyName string

	// WaitForJoin causes OpenSubscription to wait for up to WaitForJoin
	// to allow the client to join the consumer group.
	// Messages sent to the topic before the client joins the group
	// may not be received by this subscription.
	// OpenSubscription will succeed even if WaitForJoin elapses and
	// the subscription still hasn't been joined successfully.
	WaitForJoin time.Duration
}

// OpenSubscription creates a pubsub.Subscription that joins group, receiving
// messages from topics.
//
// It uses a sarama.ConsumerGroup to receive messages. Consumer options can
// be configured in the Consumer section of the sarama.Config:
// https://godoc.org/github.com/Shopify/sarama#Config.
func OpenSubscription(brokers []string, config *sarama.Config, group string, topics []string, opts *SubscriptionOptions) (*pubsub.Subscription, error) {
	ds, err := openSubscription(brokers, config, group, topics, opts)
	if err != nil {
		return nil, err
	}
	return pubsub.NewSubscription(ds, recvBatcherOpts, nil), nil
}

// openSubscription returns the driver for OpenSubscription. This function
// exists so the test harness can get the driver interface implementation if it
// needs to.
func openSubscription(brokers []string, config *sarama.Config, group string, topics []string, opts *SubscriptionOptions) (driver.Subscription, error) {
	if opts == nil {
		opts = &SubscriptionOptions{}
	}
	consumerGroup, err := sarama.NewConsumerGroup(brokers, group, config)
	if err != nil {
		return nil, err
	}
	// Create a cancelable context for the background goroutine that
	// consumes messages.
	ctx, cancel := context.WithCancel(context.Background())
	joinCh := make(chan struct{})
	ds := &subscription{
		opts:    *opts,
		ch:      make(chan *sarama.ConsumerMessage),
		closeCh: make(chan struct{}),
		joinCh:  joinCh,
		cancel:  cancel,
	}
	// Start a background consumer. It should run until ctx is cancelled
	// by Close, or until there's a fatal error (e.g., topic doesn't exist).
	// We're registering ds as our ConsumerGroupHandler, so sarama will
	// call [Setup, ConsumeClaim (possibly more than once), Cleanup]
	// repeatedly as the consumer group is rebalanced.
	// See https://godoc.org/github.com/Shopify/sarama#ConsumerGroup.
	go func() {
		ds.closeErr = consumerGroup.Consume(ctx, topics, ds)
		consumerGroup.Close()
		close(ds.closeCh)
	}()
	if opts.WaitForJoin > 0 {
		// Best effort wait for first consumer group session.
		select {
		case <-joinCh:
		case <-ds.closeCh:
		case <-time.After(opts.WaitForJoin):
		}
	}
	return ds, nil
}

// Setup implements sarama.ConsumerGroupHandler.Setup. It is called whenever
// a new session with the broker is starting.
func (s *subscription) Setup(sess sarama.ConsumerGroupSession) error {
	// The first time, close joinCh to (possibly) wake up OpenSubscription.
	if s.joinCh != nil {
		close(s.joinCh)
		s.joinCh = nil
	}
	// Record the current session.
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sess = sess
	return nil
}

// Cleanup implements sarama.ConsumerGroupHandler.Cleanup.
func (s *subscription) Cleanup(sarama.ConsumerGroupSession) error {
	// Clear the current session.
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sess = nil
	return nil
}

// ConsumeClaim implements sarama.ConsumerGroupHandler.ConsumeClaim.
// This is where messages are actually delivered, via a channel.
func (s *subscription) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		// We've received a message. Send it over the channel to ReceiveBatch.
		// s.ch has no buffer, so the send blocks until ReceiveBatch is
		// ready to receive them.
		select {
		case s.ch <- msg:
		case <-sess.Context().Done():
			// This session is over, we must return. We'll end up dropping msg, but
			// that's OK.
			// TODO(rvangent): See if we can avoid that.
			// TODO(rvangent): Also consider setting ReceiveBatch to maxMessages=1
			//                 and maxHandlers=1 similar to NATS.
		}
	}
	return nil
}

// ReceiveBatch implements driver.Subscription.ReceiveBatch.
func (s *subscription) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	// Try to read maxMessages for up to 100ms before giving up.
	waitForMaxCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	var dms []*driver.Message
	for {
		select {
		case msg := <-s.ch:
			// Read the metadata from msg.Headers.
			md := map[string]string{}
			for _, h := range msg.Headers {
				md[string(h.Key)] = string(h.Value)
			}
			// Add a metadata entry for the message key if appropriate.
			if len(msg.Key) > 0 && s.opts.KeyName != "" {
				md[s.opts.KeyName] = string(msg.Key)
			}
			ack := &ackInfo{msg: msg}
			dm := &driver.Message{
				Body:     msg.Value,
				Metadata: md,
				AckID:    ack,
				AsFunc: func(i interface{}) bool {
					// TODO(rvangent): Implement.
					return false
				},
			}
			dms = append(dms, dm)

			s.mu.Lock()
			s.unacked = append(s.unacked, ack)
			s.mu.Unlock()

			if len(dms) == maxMessages {
				return dms, nil
			}
		case <-s.closeCh:
			// Fatal error, probably the topic doesn't exist.
			return nil, s.closeErr
		case <-waitForMaxCtx.Done():
			// We've tried for a while to get maxMessages, but didn't get enough.
			// Return whatever we have (may be empty).
			return dms, ctx.Err()
		}
	}
}

// SendAcks implements driver.Subscription.SendAcks.
func (s *subscription) SendAcks(ctx context.Context, ids []driver.AckID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Mark them all acked.
	for _, id := range ids {
		id.(*ackInfo).acked = true
	}
	if s.sess == nil {
		// We don't have a current session, so we can't send offset updates.
		// We'll just wait until next time and retry.
		return nil
	}
	// Mark all of the acked messages at the head of the slice. Since Kafka only
	// stores a single offset, we can't mark messages that aren't at the head; that
	// would move the offset past other as-yet-unacked messages.
	for len(s.unacked) > 0 && s.unacked[0].acked {
		s.sess.MarkMessage(s.unacked[0].msg, "")
		s.unacked = s.unacked[1:]
	}
	return nil
}

// CanNack implements driver.CanNack.
func (s *subscription) CanNack() bool {
	// Nacking a single message doesn't make sense with the way Kafka maintains
	// offsets.
	return false
}

// SendNacks implements driver.Subscription.SendNacks.
func (s *subscription) SendNacks(ctx context.Context, ids []driver.AckID) error {
	panic("unreachable")
}

// Close implements io.Closer.
func (s *subscription) Close() error {
	// Cancel the ctx for the background goroutine and wait until it's done.
	s.cancel()
	<-s.closeCh
	return nil
}

// IsRetryable implements driver.Subscription.IsRetryable.
func (*subscription) IsRetryable(error) bool {
	// TODO(rvangent): Investigate.
	return false
}

// As implements driver.Subscription.As.
func (s *subscription) As(i interface{}) bool {
	// TODO(rvangent): Implement.
	return false
}

// ErrorAs implements driver.Subscription.ErrorAs.
func (s *subscription) ErrorAs(err error, i interface{}) bool {
	return errorAs(err, i)
}

// ErrorCode implements driver.Subscription.ErrorCode.
func (*subscription) ErrorCode(err error) gcerrors.ErrorCode {
	return errorCode(err)
}

func errorAs(err error, i interface{}) bool {
	// TODO(rvangent): Implement.
	return false
}

// AckFunc implements driver.Subscription.AckFunc.
func (*subscription) AckFunc() func() { return nil }
