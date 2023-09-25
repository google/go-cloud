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
// # URLs
//
// For pubsub.OpenTopic and pubsub.OpenSubscription, kafkapubsub registers
// for the scheme "kafka".
// The default URL opener will connect to a default set of Kafka brokers based
// on the environment variable "KAFKA_BROKERS", expected to be a comma-delimited
// set of server addresses.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// # Escaping
//
// Go CDK supports all UTF-8 strings. No escaping is required for Kafka.
// Message metadata is supported through Kafka Headers, which allow arbitrary
// []byte for both key and value. These are converted to string for use in
// Message.Metadata.
//
// # As
//
// kafkapubsub exposes the following types for As:
//   - Topic: sarama.SyncProducer
//   - Subscription: sarama.ConsumerGroup, sarama.ConsumerGroupSession (may be nil during session renegotiation, and session may go stale at any time)
//   - Message: *sarama.ConsumerMessage
//   - Message.BeforeSend: *sarama.ProducerMessage
//   - Message.AfterSend: None
//   - Error: sarama.ConsumerError, sarama.ConsumerErrors, sarama.ProducerError, sarama.ProducerErrors, sarama.ConfigurationError, sarama.PacketDecodingError, sarama.PacketEncodingError, sarama.KError
package kafkapubsub // import "gocloud.dev/pubsub/kafkapubsub"

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"gocloud.dev/gcerrors"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/batcher"
	"gocloud.dev/pubsub/driver"
)

var sendBatcherOpts = &batcher.Options{
	MaxBatchSize: 100,
	MaxHandlers:  100, // max concurrency for sends
}

var recvBatcherOpts = &batcher.Options{
	// Concurrency doesn't make sense here.
	MaxBatchSize: 1,
	MaxHandlers:  1,
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
// subscribe to. The "offset" parameter is available to subscribers to set
// the Kafka consumer's initial offset. Where "oldest" starts consuming from
// the oldest offset of the consumer group and "newest" starts consuming from
// the most recent offset on the topic.
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
	var topics []string
	for param, value := range u.Query() {
		switch param {
		case "topic":
			topics = value
		case "offset":
			if len(value) == 0 {
				return nil, fmt.Errorf("open subscription %v: invalid query parameter %q", u, param)
			}

			offset := value[0]
			switch offset {
			case "oldest":
				o.Config.Consumer.Offsets.Initial = sarama.OffsetOldest
			case "newest":
				o.Config.Consumer.Offsets.Initial = sarama.OffsetNewest
			default:
				return nil, fmt.Errorf("open subscription %v: invalid query parameter %q", u, offset)
			}
		default:
			return nil, fmt.Errorf("open subscription %v: invalid query parameter %q", u, param)
		}
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

	// BatcherOptions adds constraints to the default batching done for sends.
	BatcherOptions batcher.Options
}

// OpenTopic creates a pubsub.Topic that sends to a Kafka topic.
//
// It uses a sarama.SyncProducer to send messages. Producer options can
// be configured in the Producer section of the sarama.Config:
// https://godoc.org/github.com/IBM/sarama#Config.
//
// Config.Producer.Return.Success must be set to true.
func OpenTopic(brokers []string, config *sarama.Config, topicName string, opts *TopicOptions) (*pubsub.Topic, error) {
	dt, err := openTopic(brokers, config, topicName, opts)
	if err != nil {
		return nil, err
	}
	bo := sendBatcherOpts.NewMergedOptions(&dt.opts.BatcherOptions)
	return pubsub.NewTopic(dt, bo), nil
}

// openTopic returns the driver for OpenTopic. This function exists so the test
// harness can get the driver interface implementation if it needs to.
func openTopic(brokers []string, config *sarama.Config, topicName string, opts *TopicOptions) (*topic, error) {
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
		var kafkaKey sarama.Encoder
		var headers []sarama.RecordHeader
		for k, v := range dm.Metadata {
			if k == t.opts.KeyName {
				// Use this key's value as the Kafka message key instead of adding it
				// to the headers.
				kafkaKey = sarama.ByteEncoder(v)
			} else {
				headers = append(headers, sarama.RecordHeader{Key: []byte(k), Value: []byte(v)})
			}
		}
		pm := &sarama.ProducerMessage{
			Topic:   t.topicName,
			Key:     kafkaKey,
			Value:   sarama.ByteEncoder(dm.Body),
			Headers: headers,
		}
		if dm.BeforeSend != nil {
			asFunc := func(i interface{}) bool {
				if p, ok := i.(**sarama.ProducerMessage); ok {
					*p = pm
					return true
				}
				return false
			}
			if err := dm.BeforeSend(asFunc); err != nil {
				return err
			}
		}
		ms = append(ms, pm)

	}
	err := t.producer.SendMessages(ms)
	if err != nil {
		return err
	}
	for _, dm := range dms {
		if dm.AfterSend != nil {
			asFunc := func(i interface{}) bool { return false }
			if err := dm.AfterSend(asFunc); err != nil {
				return err
			}
		}
	}
	return nil
}

// Close implements io.Closer.
func (t *topic) Close() error {
	return t.producer.Close()
}

// IsRetryable implements driver.Topic.IsRetryable.
func (t *topic) IsRetryable(error) bool {
	return false
}

// As implements driver.Topic.As.
func (t *topic) As(i interface{}) bool {
	if p, ok := i.(*sarama.SyncProducer); ok {
		*p = t.producer
		return true
	}
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
	if pes, ok := err.(sarama.ProducerErrors); ok && len(pes) == 1 {
		return errorCode(pes[0])
	}
	if pe, ok := err.(*sarama.ProducerError); ok {
		return errorCode(pe.Err)
	}
	if err == sarama.ErrUnknownTopicOrPartition {
		return gcerrors.NotFound
	}
	return gcerrors.Unknown
}

type subscription struct {
	opts          SubscriptionOptions
	closeCh       chan struct{} // closed when we've shut down
	joinCh        chan struct{} // closed when we join for the first time
	cancel        func()        // cancels the background consumer
	closeErr      error         // fatal error detected by the background consumer
	consumerGroup sarama.ConsumerGroup

	mu             sync.Mutex
	unacked        []*ackInfo
	sess           sarama.ConsumerGroupSession // current session, if any, used for marking offset updates
	expectedClaims int                         // # of expected claims for the current session, they should be added via ConsumeClaim
	claims         []sarama.ConsumerGroupClaim // claims in the current session
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
// https://godoc.org/github.com/IBM/sarama#Config.
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
		opts:          *opts,
		consumerGroup: consumerGroup,
		closeCh:       make(chan struct{}),
		joinCh:        joinCh,
		cancel:        cancel,
	}
	// Start a background consumer. It should run until ctx is cancelled
	// by Close, or until there's a fatal error (e.g., topic doesn't exist).
	// We're registering ds as our ConsumerGroupHandler, so sarama will
	// call [Setup, ConsumeClaim (possibly more than once), Cleanup]
	// repeatedly as the consumer group is rebalanced.
	// See https://godoc.org/github.com/IBM/sarama#ConsumerGroup.
	go func() {
		for {
			ds.closeErr = consumerGroup.Consume(ctx, topics, ds)
			if ds.closeErr != nil || ctx.Err() != nil {
				consumerGroup.Close()
				close(ds.closeCh)
				break
			}
		}
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
	// Record the current session.
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sess = sess
	s.expectedClaims = 0
	for _, claims := range sess.Claims() {
		s.expectedClaims += len(claims)
	}
	return nil
}

// Cleanup implements sarama.ConsumerGroupHandler.Cleanup.
func (s *subscription) Cleanup(sarama.ConsumerGroupSession) error {
	// Clear the current session.
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sess = nil
	s.expectedClaims = 0
	s.claims = nil
	return nil
}

// ConsumeClaim implements sarama.ConsumerGroupHandler.ConsumeClaim.
// This is where messages are actually delivered, via a channel.
func (s *subscription) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	s.mu.Lock()
	s.claims = append(s.claims, claim)
	// Once all of the expected claims have registered, close joinCh to (possibly) wake up OpenSubscription.
	if s.joinCh != nil && len(s.claims) == s.expectedClaims {
		close(s.joinCh)
		s.joinCh = nil
	}
	s.mu.Unlock()
	<-sess.Context().Done()
	return nil
}

// ReceiveBatch implements driver.Subscription.ReceiveBatch.
func (s *subscription) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	// Try to read maxMessages for up to 100ms before giving up.
	maxWaitCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	for {
		// We'll give up after maxWaitCtx is Done, or if s.closeCh is closed.
		// Otherwise, we want to pull a message from one of the channels in the
		// claim(s) we've been given.
		//
		// Note: we could multiplex this by ranging over each claim.Messages(),
		// writing the messages to a single ch, and then reading from that ch
		// here. However, this results in us reading messages from Kafka and
		// essentially queueing them here; when the session is closed for whatever
		// reason, those messages are lost, which may or may not be an issue
		// depending on the Kafka configuration being used.
		//
		// It seems safer to use reflect.Select to explicitly only get a single
		// message at a time, and hand it directly to the user.
		//
		// reflect.Select is essentially a "select" statement, but allows us to
		// build the cases dynamically. We need that because we need a case for
		// each of the claims in s.claims.
		s.mu.Lock()
		cases := make([]reflect.SelectCase, 0, len(s.claims)+2)
		// Add a case for s.closeCh being closed, at index = 0.
		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(s.closeCh),
		})
		// Add a case for maxWaitCtx being Done, at index = 1.
		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(maxWaitCtx.Done()),
		})
		// Add a case per claim, reading from the claim's Messages channel.
		for _, claim := range s.claims {
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(claim.Messages()),
			})
		}
		s.mu.Unlock()
		i, v, ok := reflect.Select(cases)
		if !ok {
			// The i'th channel was closed.
			switch i {
			case 0: // s.closeCh
				return nil, s.closeErr
			case 1: // maxWaitCtx
				// We've tried for a while to get a message, but didn't get any.
				// Return an empty slice; the portable type will call us back.
				return nil, ctx.Err()
			}
			// Otherwise, if one of the claim channels closed, we're probably ending
			// a session. Just keep trying.
			continue
		}
		msg := v.Interface().(*sarama.ConsumerMessage)

		// We've got a message! It should not be nil.
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
		var loggableID string
		if len(msg.Key) == 0 {
			loggableID = fmt.Sprintf("partition %d offset %d", msg.Partition, msg.Offset)
		} else {
			loggableID = string(msg.Key)
		}
		dm := &driver.Message{
			LoggableID: loggableID,
			Body:       msg.Value,
			Metadata:   md,
			AckID:      ack,
			AsFunc: func(i interface{}) bool {
				if p, ok := i.(**sarama.ConsumerMessage); ok {
					*p = msg
					return true
				}
				return false
			},
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		s.unacked = append(s.unacked, ack)
		return []*driver.Message{dm}, nil
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
	return false
}

// As implements driver.Subscription.As.
func (s *subscription) As(i interface{}) bool {
	if p, ok := i.(*sarama.ConsumerGroup); ok {
		*p = s.consumerGroup
		return true
	}
	if p, ok := i.(*sarama.ConsumerGroupSession); ok {
		s.mu.Lock()
		defer s.mu.Unlock()
		*p = s.sess
		return true
	}
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
	switch terr := err.(type) {
	case sarama.ConsumerError:
		if p, ok := i.(*sarama.ConsumerError); ok {
			*p = terr
			return true
		}
	case sarama.ConsumerErrors:
		if p, ok := i.(*sarama.ConsumerErrors); ok {
			*p = terr
			return true
		}
	case sarama.ProducerError:
		if p, ok := i.(*sarama.ProducerError); ok {
			*p = terr
			return true
		}
	case sarama.ProducerErrors:
		if p, ok := i.(*sarama.ProducerErrors); ok {
			*p = terr
			return true
		}
	case sarama.ConfigurationError:
		if p, ok := i.(*sarama.ConfigurationError); ok {
			*p = terr
			return true
		}
	case sarama.PacketDecodingError:
		if p, ok := i.(*sarama.PacketDecodingError); ok {
			*p = terr
			return true
		}
	case sarama.PacketEncodingError:
		if p, ok := i.(*sarama.PacketEncodingError); ok {
			*p = terr
			return true
		}
	case sarama.KError:
		if p, ok := i.(*sarama.KError); ok {
			*p = terr
			return true
		}
	}
	return false
}
