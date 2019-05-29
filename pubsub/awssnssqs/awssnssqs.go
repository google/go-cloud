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

// Package awssnssqs provides two implementations of pubsub.Topic, one that
// sends messages to AWS SNS (Simple Notification Service), and one that sends
// messages to SQS (Simple Queuing Service). It also provides an implementation
// of pubsub.Subscription that receives messages from SQS.
//
// URLs
//
// For pubsub.OpenTopic, awssnssqs registers for the scheme "awssns" for
// an SNS topic, and "awssqs" for an SQS topic. For pubsub.OpenSubscription,
// it registers for the scheme "awssqs".
//
// The default URL opener will use an AWS session with the default credentials
// and configuration; see https://docs.aws.amazon.com/sdk-for-go/api/aws/session/
// for more details.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// Message Delivery Semantics
//
// AWS SQS supports at-least-once semantics; applications must call Message.Ack
// after processing a message, or it will be redelivered.
// See https://godoc.org/gocloud.dev/pubsub#hdr-At_most_once_and_At_least_once_Delivery
// for more background.
//
// Escaping
//
// Go CDK supports all UTF-8 strings; to make this work with providers lacking
// full UTF-8 support, strings must be escaped (during writes) and unescaped
// (during reads). The following escapes are required for awssnssqs:
//  - Metadata keys: Characters other than "a-zA-z0-9_-.", and additionally "."
//    when it's at the start of the key or the previous character was ".",
//    are escaped using "__0x<hex>__". These characters were determined by
//    experimentation.
//  - Metadata values: Escaped using URL encoding.
//  - Message body: AWS SNS/SQS only supports UTF-8 strings. See the
//    BodyBase64Encoding enum in TopicOptions for strategies on how to send
//    non-UTF-8 message bodies. By default, non-UTF-8 message bodies are base64
//    encoded.
//
// As
//
// awssnssqs exposes the following types for As:
//  - Topic: *sns.SNS (for OpenSNSTopic) or *sqs.SQS (for OpenSQSTopic)
//  - Subscription: *sqs.SQS
//  - Message: *sqs.Message
//  - Message.BeforeSend: *sns.PublishInput (for OpenSNSTopic) or *sqs.SendMessageInput (for OpenSQSTopic)
//  - Error: awserror.Error
package awssnssqs // import "gocloud.dev/pubsub/awssnssqs"

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/google/wire"
	gcaws "gocloud.dev/aws"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/batcher"
	"gocloud.dev/internal/escape"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
)

const (
	// base64EncodedKey is the Message Attribute key used to flag that the
	// message body is base64 encoded.
	base64EncodedKey = "base64encoded"
	// How long ReceiveBatch should wait if no messages are available; controls
	// the poll interval of requests to SQS.
	noMessagesPollDuration = 250 * time.Millisecond
)

var sendBatcherOpts = &batcher.Options{
	MaxBatchSize: 1,   // Both SNS and SQS SendBatch only support one message at a time
	MaxHandlers:  100, // max concurrency for sends
}

var recvBatcherOpts = &batcher.Options{
	// SQS supports receiving at most 10 messages at a time:
	// https://godoc.org/github.com/aws/aws-sdk-go/service/sqs#SQS.ReceiveMessage
	MaxBatchSize: 10,
	MaxHandlers:  100, // max concurrency for receives
}

var ackBatcherOpts = &batcher.Options{
	// SQS supports deleting/updating at most 10 messages at a time:
	// https://godoc.org/github.com/aws/aws-sdk-go/service/sqs#SQS.DeleteMessageBatch
	// https://godoc.org/github.com/aws/aws-sdk-go/service/sqs#SQS.ChangeMessageVisibilityBatch
	MaxBatchSize: 10,
	MaxHandlers:  100, // max concurrency for acks
}

func init() {
	lazy := new(lazySessionOpener)
	pubsub.DefaultURLMux().RegisterTopic(SNSScheme, lazy)
	pubsub.DefaultURLMux().RegisterTopic(SQSScheme, lazy)
	pubsub.DefaultURLMux().RegisterSubscription(SQSScheme, lazy)
}

// Set holds Wire providers for this package.
var Set = wire.NewSet(
	SubscriptionOptions{},
	TopicOptions{},
	URLOpener{},
)

// lazySessionOpener obtains the AWS session from the environment on the first
// call to OpenXXXURL.
type lazySessionOpener struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

func (o *lazySessionOpener) defaultOpener() (*URLOpener, error) {
	o.init.Do(func() {
		sess, err := gcaws.NewDefaultSession()
		if err != nil {
			o.err = err
			return
		}
		o.opener = &URLOpener{
			ConfigProvider: sess,
		}
	})
	return o.opener, o.err
}

func (o *lazySessionOpener) OpenTopicURL(ctx context.Context, u *url.URL) (*pubsub.Topic, error) {
	opener, err := o.defaultOpener()
	if err != nil {
		return nil, fmt.Errorf("open topic %v: failed to open default session: %v", u, err)
	}
	return opener.OpenTopicURL(ctx, u)
}

func (o *lazySessionOpener) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*pubsub.Subscription, error) {
	opener, err := o.defaultOpener()
	if err != nil {
		return nil, fmt.Errorf("open subscription %v: failed to open default session: %v", u, err)
	}
	return opener.OpenSubscriptionURL(ctx, u)
}

// SNSScheme is the URL scheme for pubsub.OpenTopic (for an SNS topic) that
// awssnssqs registers its URLOpeners under on pubsub.DefaultMux.
const SNSScheme = "awssns"

// SQSScheme is the URL scheme for pubsub.OpenTopic (for an SQS topic) and for
// pubsub.OpenSubscription that awssnssqs registers its URLOpeners under on
// pubsub.DefaultMux.
const SQSScheme = "awssqs"

// URLOpener opens AWS SNS/SQS URLs like "awssns://sns-topic-arn" for
// SNS topics or "awssqs://sqs-queue-url" for SQS topics and subscriptions.
//
// For SNS topics, the URL's host+path is used as the topic Amazon Resource Name
// (ARN).
//
// For SQS topics and subscriptions, the URL's host+path is prefixed with
// "https://" to create the queue URL.
//
// See gocloud.dev/aws/ConfigFromURLParams for supported query parameters
// that affect the default AWS session.
type URLOpener struct {
	// ConfigProvider configures the connection to AWS.
	ConfigProvider client.ConfigProvider

	// TopicOptions specifies the options to pass to OpenTopic.
	TopicOptions TopicOptions
	// SubscriptionOptions specifies the options to pass to OpenSubscription.
	SubscriptionOptions SubscriptionOptions
}

// OpenTopicURL opens a pubsub.Topic based on u.
func (o *URLOpener) OpenTopicURL(ctx context.Context, u *url.URL) (*pubsub.Topic, error) {
	configProvider := &gcaws.ConfigOverrider{
		Base: o.ConfigProvider,
	}
	overrideCfg, err := gcaws.ConfigFromURLParams(u.Query())
	if err != nil {
		return nil, fmt.Errorf("open topic %v: %v", u, err)
	}
	configProvider.Configs = append(configProvider.Configs, overrideCfg)
	switch u.Scheme {
	case SNSScheme:
		topicARN := path.Join(u.Host, u.Path)
		return OpenSNSTopic(ctx, configProvider, topicARN, &o.TopicOptions), nil
	case SQSScheme:
		qURL := "https://" + path.Join(u.Host, u.Path)
		return OpenSQSTopic(ctx, configProvider, qURL, &o.TopicOptions), nil
	default:
		return nil, fmt.Errorf("open topic %v: unsupported scheme", u)
	}
}

// OpenSubscriptionURL opens a pubsub.Subscription based on u.
func (o *URLOpener) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*pubsub.Subscription, error) {
	configProvider := &gcaws.ConfigOverrider{
		Base: o.ConfigProvider,
	}
	overrideCfg, err := gcaws.ConfigFromURLParams(u.Query())
	if err != nil {
		return nil, fmt.Errorf("open subscription %v: %v", u, err)
	}
	configProvider.Configs = append(configProvider.Configs, overrideCfg)
	qURL := "https://" + path.Join(u.Host, u.Path)
	return OpenSubscription(ctx, configProvider, qURL, &o.SubscriptionOptions), nil
}

type snsTopic struct {
	client *sns.SNS
	arn    string
	opts   *TopicOptions
}

// BodyBase64Encoding is an enum of strategies for when to base64 message
// bodies.
type BodyBase64Encoding int

const (
	// NonUTF8Only means that message bodies that are valid UTF-8 encodings are
	// sent as-is. Invalid UTF-8 message bodies are base64 encoded, and a
	// MessageAttribute with key "base64encoded" is added to the message.
	// When receiving messages, the "base64encoded" attribute is used to determine
	// whether to base64 decode, and is then filtered out.
	NonUTF8Only BodyBase64Encoding = 0
	// Always means that all message bodies are base64 encoded.
	// A MessageAttribute with key "base64encoded" is added to the message.
	// When receiving messages, the "base64encoded" attribute is used to determine
	// whether to base64 decode, and is then filtered out.
	Always BodyBase64Encoding = 1
	// Never means that message bodies are never base64 encoded. Non-UTF-8
	// bytes in message bodies may be modified by SNS/SQS.
	Never BodyBase64Encoding = 2
)

func (e BodyBase64Encoding) wantEncode(b []byte) bool {
	switch e {
	case Always:
		return true
	case Never:
		return false
	case NonUTF8Only:
		return !utf8.Valid(b)
	}
	panic("unreachable")
}

// TopicOptions contains configuration options for topics.
type TopicOptions struct {
	// BodyBase64Encoding determines when message bodies are base64 encoded.
	// The default is NonUTF8Only.
	BodyBase64Encoding BodyBase64Encoding
}

// OpenTopic is a shortcut for OpenSNSTopic, provided for backwards compatibility.
func OpenTopic(ctx context.Context, sess client.ConfigProvider, topicARN string, opts *TopicOptions) *pubsub.Topic {
	return OpenSNSTopic(ctx, sess, topicARN, opts)
}

// OpenSNSTopic opens a topic that sends to the SNS topic with the given Amazon
// Resource Name (ARN).
func OpenSNSTopic(ctx context.Context, sess client.ConfigProvider, topicARN string, opts *TopicOptions) *pubsub.Topic {
	return pubsub.NewTopic(openSNSTopic(ctx, sess, topicARN, opts), sendBatcherOpts)
}

// openSNSTopic returns the driver for OpenSNSTopic. This function exists so the test
// harness can get the driver interface implementation if it needs to.
func openSNSTopic(ctx context.Context, sess client.ConfigProvider, topicARN string, opts *TopicOptions) driver.Topic {
	if opts == nil {
		opts = &TopicOptions{}
	}
	return &snsTopic{
		client: sns.New(sess),
		arn:    topicARN,
		opts:   opts,
	}
}

var stringDataType = aws.String("String")

// encodeMetadata encodes the keys and values of md as needed.
func encodeMetadata(md map[string]string) map[string]string {
	retval := map[string]string{}
	for k, v := range md {
		// See the package comments for more details on escaping of metadata
		// keys & values.
		k = escape.HexEscape(k, func(runes []rune, i int) bool {
			c := runes[i]
			switch {
			case escape.IsASCIIAlphanumeric(c):
				return false
			case c == '_' || c == '-':
				return false
			case c == '.' && i != 0 && runes[i-1] != '.':
				return false
			}
			return true
		})
		retval[k] = escape.URLEscape(v)
	}
	return retval
}

// maybeEncodeBody decides whether body should base64-encoded based on opt, and
// returns the (possibly encoded) body as a string, along with a boolean
// indicating whether encoding occurred.
func maybeEncodeBody(body []byte, opt BodyBase64Encoding) (string, bool) {
	if opt.wantEncode(body) {
		return base64.StdEncoding.EncodeToString(body), true
	}
	return string(body), false
}

// SendBatch implements driver.Topic.SendBatch.
func (t *snsTopic) SendBatch(ctx context.Context, dms []*driver.Message) error {
	if len(dms) != 1 {
		panic("snsTopic.SendBatch should only get one message at a time")
	}
	dm := dms[0]

	attrs := map[string]*sns.MessageAttributeValue{}
	for k, v := range encodeMetadata(dm.Metadata) {
		attrs[k] = &sns.MessageAttributeValue{
			DataType:    stringDataType,
			StringValue: aws.String(v),
		}
	}
	body, didEncode := maybeEncodeBody(dm.Body, t.opts.BodyBase64Encoding)
	if didEncode {
		attrs[base64EncodedKey] = &sns.MessageAttributeValue{
			DataType:    stringDataType,
			StringValue: aws.String("true"),
		}
	}
	if len(attrs) == 0 {
		attrs = nil
	}
	input := &sns.PublishInput{
		Message:           aws.String(body),
		MessageAttributes: attrs,
		TopicArn:          &t.arn,
	}
	if dm.BeforeSend != nil {
		asFunc := func(i interface{}) bool {
			if p, ok := i.(**sns.PublishInput); ok {
				*p = input
				return true
			}
			return false
		}
		if err := dm.BeforeSend(asFunc); err != nil {
			return err
		}
	}
	_, err := t.client.PublishWithContext(ctx, input)
	return err
}

// IsRetryable implements driver.Topic.IsRetryable.
func (t *snsTopic) IsRetryable(error) bool {
	// The client handles retries.
	return false
}

// As implements driver.Topic.As.
func (t *snsTopic) As(i interface{}) bool {
	c, ok := i.(**sns.SNS)
	if !ok {
		return false
	}
	*c = t.client
	return true
}

// ErrorAs implements driver.Topic.ErrorAs.
func (t *snsTopic) ErrorAs(err error, i interface{}) bool {
	return errorAs(err, i)
}

// ErrorCode implements driver.Topic.ErrorCode.
func (t *snsTopic) ErrorCode(err error) gcerrors.ErrorCode {
	return errorCode(err)
}

// Close implements driver.Topic.Close.
func (*snsTopic) Close() error { return nil }

type sqsTopic struct {
	client *sqs.SQS
	qURL   string
	opts   *TopicOptions
}

// OpenSQSTopic opens a topic that sends to the SQS topic with the given SQS
// queue URL.
func OpenSQSTopic(ctx context.Context, sess client.ConfigProvider, qURL string, opts *TopicOptions) *pubsub.Topic {
	return pubsub.NewTopic(openSQSTopic(ctx, sess, qURL, opts), sendBatcherOpts)
}

// openSQSTopic returns the driver for OpenSQSTopic. This function exists so the test
// harness can get the driver interface implementation if it needs to.
func openSQSTopic(ctx context.Context, sess client.ConfigProvider, qURL string, opts *TopicOptions) driver.Topic {
	if opts == nil {
		opts = &TopicOptions{}
	}
	return &sqsTopic{client: sqs.New(sess), qURL: qURL, opts: opts}
}

// SendBatch implements driver.Topic.SendBatch.
func (t *sqsTopic) SendBatch(ctx context.Context, dms []*driver.Message) error {
	if len(dms) != 1 {
		panic("sqsTopic.SendBatch should only get one message at a time")
	}
	dm := dms[0]
	attrs := map[string]*sqs.MessageAttributeValue{}
	for k, v := range encodeMetadata(dm.Metadata) {
		attrs[k] = &sqs.MessageAttributeValue{
			DataType:    stringDataType,
			StringValue: aws.String(v),
		}
	}
	body, didEncode := maybeEncodeBody(dm.Body, t.opts.BodyBase64Encoding)
	if didEncode {
		attrs[base64EncodedKey] = &sqs.MessageAttributeValue{
			DataType:    stringDataType,
			StringValue: aws.String("true"),
		}
	}
	if len(attrs) == 0 {
		attrs = nil
	}
	req := &sqs.SendMessageInput{
		QueueUrl:          aws.String(t.qURL),
		MessageAttributes: attrs,
		MessageBody:       aws.String(body),
	}
	if dm.BeforeSend != nil {
		asFunc := func(i interface{}) bool {
			if p, ok := i.(**sqs.SendMessageInput); ok {
				*p = req
				return true
			}
			return false
		}
		if err := dm.BeforeSend(asFunc); err != nil {
			return err
		}
	}
	_, err := t.client.SendMessageWithContext(ctx, req)
	return err
}

// IsRetryable implements driver.Topic.IsRetryable.
func (t *sqsTopic) IsRetryable(error) bool {
	// The client handles retries.
	return false
}

// As implements driver.Topic.As.
func (t *sqsTopic) As(i interface{}) bool {
	c, ok := i.(**sqs.SQS)
	if !ok {
		return false
	}
	*c = t.client
	return true
}

// ErrorAs implements driver.Topic.ErrorAs.
func (t *sqsTopic) ErrorAs(err error, i interface{}) bool {
	return errorAs(err, i)
}

// ErrorCode implements driver.Topic.ErrorCode.
func (t *sqsTopic) ErrorCode(err error) gcerrors.ErrorCode {
	return errorCode(err)
}

// Close implements driver.Topic.Close.
func (*sqsTopic) Close() error { return nil }

func errorCode(err error) gcerrors.ErrorCode {
	ae, ok := err.(awserr.Error)
	if !ok {
		return gcerr.Unknown
	}
	ec, ok := errorCodeMap[ae.Code()]
	if !ok {
		return gcerr.Unknown
	}
	return ec
}

var errorCodeMap = map[string]gcerrors.ErrorCode{
	sns.ErrCodeAuthorizationErrorException:          gcerr.PermissionDenied,
	sns.ErrCodeKMSAccessDeniedException:             gcerr.PermissionDenied,
	sns.ErrCodeKMSDisabledException:                 gcerr.FailedPrecondition,
	sns.ErrCodeKMSInvalidStateException:             gcerr.FailedPrecondition,
	sns.ErrCodeKMSOptInRequired:                     gcerr.FailedPrecondition,
	sqs.ErrCodeMessageNotInflight:                   gcerr.FailedPrecondition,
	sqs.ErrCodePurgeQueueInProgress:                 gcerr.FailedPrecondition,
	sqs.ErrCodeQueueDeletedRecently:                 gcerr.FailedPrecondition,
	sqs.ErrCodeQueueNameExists:                      gcerr.FailedPrecondition,
	sns.ErrCodeInternalErrorException:               gcerr.Internal,
	sns.ErrCodeInvalidParameterException:            gcerr.InvalidArgument,
	sns.ErrCodeInvalidParameterValueException:       gcerr.InvalidArgument,
	sqs.ErrCodeBatchEntryIdsNotDistinct:             gcerr.InvalidArgument,
	sqs.ErrCodeBatchRequestTooLong:                  gcerr.InvalidArgument,
	sqs.ErrCodeEmptyBatchRequest:                    gcerr.InvalidArgument,
	sqs.ErrCodeInvalidAttributeName:                 gcerr.InvalidArgument,
	sqs.ErrCodeInvalidBatchEntryId:                  gcerr.InvalidArgument,
	sqs.ErrCodeInvalidIdFormat:                      gcerr.InvalidArgument,
	sqs.ErrCodeInvalidMessageContents:               gcerr.InvalidArgument,
	sqs.ErrCodeReceiptHandleIsInvalid:               gcerr.InvalidArgument,
	sqs.ErrCodeTooManyEntriesInBatchRequest:         gcerr.InvalidArgument,
	sqs.ErrCodeUnsupportedOperation:                 gcerr.InvalidArgument,
	sns.ErrCodeInvalidSecurityException:             gcerr.PermissionDenied,
	sns.ErrCodeKMSNotFoundException:                 gcerr.NotFound,
	sns.ErrCodeNotFoundException:                    gcerr.NotFound,
	sqs.ErrCodeQueueDoesNotExist:                    gcerr.NotFound,
	sns.ErrCodeFilterPolicyLimitExceededException:   gcerr.ResourceExhausted,
	sns.ErrCodeSubscriptionLimitExceededException:   gcerr.ResourceExhausted,
	sns.ErrCodeTopicLimitExceededException:          gcerr.ResourceExhausted,
	sqs.ErrCodeOverLimit:                            gcerr.ResourceExhausted,
	sns.ErrCodeKMSThrottlingException:               gcerr.ResourceExhausted,
	sns.ErrCodeThrottledException:                   gcerr.ResourceExhausted,
	"RequestCanceled":                               gcerr.Canceled,
	sns.ErrCodeEndpointDisabledException:            gcerr.Unknown,
	sns.ErrCodePlatformApplicationDisabledException: gcerr.Unknown,
}

type subscription struct {
	client *sqs.SQS
	qURL   string
}

// SubscriptionOptions will contain configuration for subscriptions.
type SubscriptionOptions struct{}

// OpenSubscription opens a subscription based on AWS SQS for the given SQS
// queue URL. The queue is assumed to be subscribed to some SNS topic, though
// there is no check for this.
func OpenSubscription(ctx context.Context, sess client.ConfigProvider, qURL string, opts *SubscriptionOptions) *pubsub.Subscription {
	return pubsub.NewSubscription(openSubscription(ctx, sess, qURL), recvBatcherOpts, ackBatcherOpts)
}

// openSubscription returns a driver.Subscription.
func openSubscription(ctx context.Context, sess client.ConfigProvider, qURL string) driver.Subscription {
	return &subscription{client: sqs.New(sess), qURL: qURL}
}

// ReceiveBatch implements driver.Subscription.ReceiveBatch.
func (s *subscription) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	output, err := s.client.ReceiveMessageWithContext(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:              aws.String(s.qURL),
		MaxNumberOfMessages:   aws.Int64(int64(maxMessages)),
		MessageAttributeNames: []*string{aws.String("All")},
	})
	if err != nil {
		return nil, err
	}
	var ms []*driver.Message
	for _, m := range output.Messages {
		// By default, messages from SNS come with everything in a JSON-encoded Body.
		// For example, message attributes are not in m.MessageAttributes, but
		// are rather in a MessageAttributes field in the JSON-encoded body.
		// If RawMessageDelivery is enabled as part of the SQS queue subscription
		// to the SNS topic, then body is in Body and attributes are in
		// MessageAttributes. Same deal if you send directly to the SQS queue.
		//
		// If it looks like the body might be JSON, we try decoding it. If it
		// doesn't look like JSON, or if the JSON decode fails, we use the raw
		// data from m.
		bodyStr := aws.StringValue(m.Body)
		rawAttrs := map[string]string{}
		useJSON := len(m.MessageAttributes) == 0 // if we got attributes, it's raw
		if useJSON {
			var bodyJSON struct {
				Message           string
				MessageAttributes map[string]struct{ Value string }
			}
			if err := json.Unmarshal([]byte(bodyStr), &bodyJSON); err == nil {
				// JSON decode succeeded; get attributes from the decoded struct,
				// and update the body to be the JSON Message field.
				for k, v := range bodyJSON.MessageAttributes {
					rawAttrs[k] = v.Value
				}
				bodyStr = bodyJSON.Message
			} else {
				// JSON decode failed; leave bodyStr alone. There can't be any
				// attributes.
			}
		} else {
			for k, v := range m.MessageAttributes {
				rawAttrs[k] = aws.StringValue(v.StringValue)
			}
		}

		decodeIt := false
		attrs := map[string]string{}
		for k, v := range rawAttrs {
			// See BodyBase64Encoding for details on when we base64 decode message bodies.
			if k == base64EncodedKey {
				decodeIt = true
				continue
			}
			// See the package comments for more details on escaping of metadata
			// keys & values.
			attrs[escape.HexUnescape(k)] = escape.URLUnescape(v)
		}

		var b []byte
		if decodeIt {
			var err error
			b, err = base64.StdEncoding.DecodeString(bodyStr)
			if err != nil {
				// Fall back to using the raw message.
				b = []byte(bodyStr)
			}
		} else {
			b = []byte(bodyStr)
		}

		m2 := &driver.Message{
			Body:     b,
			Metadata: attrs,
			AckID:    m.ReceiptHandle,
			AsFunc: func(i interface{}) bool {
				p, ok := i.(**sqs.Message)
				if !ok {
					return false
				}
				*p = m
				return true
			},
		}
		ms = append(ms, m2)
	}
	if len(ms) == 0 {
		// When we return no messages and no error, the portable type will call
		// ReceiveBatch again immediately. Sleep for a bit to avoid hammering SQS
		// with RPCs.
		time.Sleep(noMessagesPollDuration)
	}
	return ms, nil
}

// SendAcks implements driver.Subscription.SendAcks.
func (s *subscription) SendAcks(ctx context.Context, ids []driver.AckID) error {
	req := &sqs.DeleteMessageBatchInput{QueueUrl: aws.String(s.qURL)}
	for _, id := range ids {
		req.Entries = append(req.Entries, &sqs.DeleteMessageBatchRequestEntry{
			Id:            aws.String(strconv.Itoa(len(req.Entries))),
			ReceiptHandle: id.(*string),
		})
	}
	resp, err := s.client.DeleteMessageBatchWithContext(ctx, req)
	if err != nil {
		return err
	}
	// Note: DeleteMessageBatch doesn't return failures when you try
	// to Delete an id that isn't found.
	if numFailed := len(resp.Failed); numFailed > 0 {
		first := resp.Failed[0]
		return awserr.New(aws.StringValue(first.Code), fmt.Sprintf("sqs.DeleteMessageBatch failed for %d message(s): %s", numFailed, aws.StringValue(first.Message)), nil)
	}
	return nil
}

// CanNack implements driver.CanNack.
func (s *subscription) CanNack() bool { return true }

// SendNacks implements driver.Subscription.SendNacks.
func (s *subscription) SendNacks(ctx context.Context, ids []driver.AckID) error {
	req := &sqs.ChangeMessageVisibilityBatchInput{QueueUrl: aws.String(s.qURL)}
	for _, id := range ids {
		req.Entries = append(req.Entries, &sqs.ChangeMessageVisibilityBatchRequestEntry{
			Id:                aws.String(strconv.Itoa(len(req.Entries))),
			ReceiptHandle:     id.(*string),
			VisibilityTimeout: aws.Int64(0),
		})
	}
	resp, err := s.client.ChangeMessageVisibilityBatchWithContext(ctx, req)
	if err != nil {
		return err
	}
	// Note: ChangeMessageVisibilityBatch returns failures when you try to
	// modify an id that isn't found; drop those.
	var firstFail *sqs.BatchResultErrorEntry
	numFailed := 0
	for _, fail := range resp.Failed {
		if aws.StringValue(fail.Code) == sqs.ErrCodeReceiptHandleIsInvalid {
			continue
		}
		if numFailed == 0 {
			firstFail = fail
		}
		numFailed++
	}
	if numFailed > 0 {
		return awserr.New(aws.StringValue(firstFail.Code), fmt.Sprintf("sqs.ChangeMessageVisibilityBatch failed for %d message(s): %s", numFailed, aws.StringValue(firstFail.Message)), nil)
	}
	return nil
}

// IsRetryable implements driver.Subscription.IsRetryable.
func (*subscription) IsRetryable(error) bool {
	// The client handles retries.
	return false
}

// As implements driver.Subscription.As.
func (s *subscription) As(i interface{}) bool {
	c, ok := i.(**sqs.SQS)
	if !ok {
		return false
	}
	*c = s.client
	return true
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
	e, ok := err.(awserr.Error)
	if !ok {
		return false
	}
	p, ok := i.(*awserr.Error)
	if !ok {
		return false
	}
	*p = e
	return true
}

// Close implements driver.Subscription.Close.
func (*subscription) Close() error { return nil }
