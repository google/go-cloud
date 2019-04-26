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

// Package awssnssqs provides an implementation of pubsub that uses AWS
// SNS (Simple Notification Service) and SQS (Simple Queueing Service).
//
// URLs
//
// For pubsub.OpenTopic and pubsub.OpenSubscription, awssnssqs registers
// for the schemes "awssns" and "awssqs" respectively.
// The default URL opener will use an AWS session with the default credentials
// and configuration; see https://docs.aws.amazon.com/sdk-for-go/api/aws/session/
// for more details.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://godoc.org/gocloud.dev#hdr-URLs for background information.
//
// Message Delivery Semantics
//
// AWS SNS and SQS combine to support at-least-once semantics; applications must
// call Message.Ack after processing a message, or it will be redelivered.
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
//  - Topic: *sns.SNS
//  - Subscription: *sqs.SQS
//  - Message: *sqs.Message
//  - Message.BeforeSend: *sns.PublishInput
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
	"github.com/aws/aws-sdk-go/aws/session"
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
	MaxBatchSize: 1,   // SendBatch only supports one message at a time
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
		sess, err := session.NewSessionWithOptions(session.Options{SharedConfigState: session.SharedConfigEnable})
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

// SNSScheme is the URL scheme for pubsub.OpenTopic awssnssqs registers its URLOpeners under on pubsub.DefaultMux.
const SNSScheme = "awssns"

// SQSScheme is the URL scheme for pubsub.OpenSubscription awssnssqs registers its URLOpeners under on pubsub.DefaultMux.
const SQSScheme = "awssqs"

// URLOpener opens AWS SNS/SQS URLs like "awssns://sns-topic-arn" for
// topics or "awssqs://sqs-queue-url" for subscriptions.
//
// For topics, the URL's host+path is used as the topic Amazon Resource Name
// (ARN).
//
// For subscriptions, the URL's host+path is prefixed with "https://" to create
// the queue URL.
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
	topicARN := path.Join(u.Host, u.Path)
	return OpenTopic(ctx, configProvider, topicARN, &o.TopicOptions), nil
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

type topic struct {
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

// OpenTopic opens a topic that sends to the SNS topic with the given Amazon
// Resource Name (ARN).
func OpenTopic(ctx context.Context, sess client.ConfigProvider, topicARN string, opts *TopicOptions) *pubsub.Topic {
	return pubsub.NewTopic(openTopic(ctx, sess, topicARN, opts), sendBatcherOpts)
}

// openTopic returns the driver for OpenTopic. This function exists so the test
// harness can get the driver interface implementation if it needs to.
func openTopic(ctx context.Context, sess client.ConfigProvider, topicARN string, opts *TopicOptions) driver.Topic {
	if opts == nil {
		opts = &TopicOptions{}
	}

	return &topic{
		client: sns.New(sess),
		arn:    topicARN,
		opts:   opts,
	}
}

var stringDataType = aws.String("String")

// SendBatch implements driver.Topic.SendBatch.
func (t *topic) SendBatch(ctx context.Context, dms []*driver.Message) error {
	if len(dms) != 1 {
		panic("awssnssqs.SendBatch should only get one message at a time")
	}
	dm := dms[0]
	attrs := map[string]*sns.MessageAttributeValue{}
	for k, v := range dm.Metadata {
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
		attrs[k] = &sns.MessageAttributeValue{
			DataType:    stringDataType,
			StringValue: aws.String(escape.URLEscape(v)),
		}
	}
	var body string
	if t.opts.BodyBase64Encoding.wantEncode(dm.Body) {
		body = base64.StdEncoding.EncodeToString(dm.Body)
		attrs[base64EncodedKey] = &sns.MessageAttributeValue{
			DataType:    stringDataType,
			StringValue: aws.String("true"),
		}
	} else {
		body = string(dm.Body)
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
func (t *topic) IsRetryable(error) bool {
	// The client handles retries.
	return false
}

// As implements driver.Topic.As.
func (t *topic) As(i interface{}) bool {
	c, ok := i.(**sns.SNS)
	if !ok {
		return false
	}
	*c = t.client
	return true
}

// ErrorAs implements driver.Topic.ErrorAs.
func (t *topic) ErrorAs(err error, i interface{}) bool {
	return errorAs(err, i)
}

// ErrorCode implements driver.Topic.ErrorCode.
func (t *topic) ErrorCode(err error) gcerrors.ErrorCode {
	return errorCode(err)
}

// Close implements driver.Topic.Close.
func (*topic) Close() error { return nil }

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
		QueueUrl:            aws.String(s.qURL),
		MaxNumberOfMessages: aws.Int64(int64(maxMessages)),
	})
	if err != nil {
		return nil, err
	}
	var ms []*driver.Message
	for _, m := range output.Messages {
		type MsgBody struct {
			Message           string
			MessageAttributes map[string]struct{ Value string }
		}
		var body MsgBody
		if err := json.Unmarshal([]byte(*m.Body), &body); err != nil {
			return nil, err
		}
		// See BodyBase64Encoding for details on when we base64 decode message bodies.
		decodeIt := false
		attrs := map[string]string{}
		for k, v := range body.MessageAttributes {
			if k == base64EncodedKey {
				decodeIt = true
				continue
			}
			// See the package comments for more details on escaping of metadata
			// keys & values.
			attrs[escape.HexUnescape(k)] = escape.URLUnescape(v.Value)
		}

		var b []byte
		if decodeIt {
			var err error
			b, err = base64.StdEncoding.DecodeString(body.Message)
			if err != nil {
				// Fall back to using the raw message.
				b = []byte(body.Message)
			}
		} else {
			b = []byte(body.Message)
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

// AckFunc implements driver.Subscription.AckFunc.
func (*subscription) AckFunc() func() { return nil }

// Close implements driver.Subscription.Close.
func (*subscription) Close() error { return nil }
