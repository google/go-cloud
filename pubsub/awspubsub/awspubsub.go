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

// Package awspubsub provides an implementation of pubsub that uses AWS
// SNS (Simple Notification Service) and SQS (Simple Queueing Service).
//
// Escaping
//
// Go CDK supports all UTF-8 strings; to make this work with providers lacking
// full UTF-8 support, strings must be escaped (during writes) and unescaped
// (during reads). The following escapes are required for awspubsub:
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
// awspubsub exposes the following types for As:
//  - Topic: *sns.SNS
//  - Subscription: *sqs.SQS
//  - Message: *sqs.Message
//  - Error: awserror.Error
package awspubsub

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sqs"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/escape"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
	"golang.org/x/sync/errgroup"
)

const (
	// base64EncodedKey is the Message Attribute key used to flag that the
	// message body is base64 encoded.
	base64EncodedKey = "base64encoded"
	// maxPublishConcurrency limits the number of Publish RPCs to SNS in flight
	// at once, per Topic.
	maxPublishConcurrency = 10
)

type topic struct {
	client *sns.SNS
	arn    string
	sem    chan struct{} // limits maximum in-flight Public RPCs across SendBatch
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

// OpenTopic opens the topic on AWS SNS for the given SNS client and topic ARN.
func OpenTopic(ctx context.Context, client *sns.SNS, topicARN string, opts *TopicOptions) *pubsub.Topic {
	return pubsub.NewTopic(openTopic(ctx, client, topicARN, opts), nil)
}

// openTopic returns the driver for OpenTopic. This function exists so the test
// harness can get the driver interface implementation if it needs to.
func openTopic(ctx context.Context, client *sns.SNS, topicARN string, opts *TopicOptions) driver.Topic {
	if opts == nil {
		opts = &TopicOptions{}
	}
	return &topic{
		client: client,
		arn:    topicARN,
		sem:    make(chan struct{}, maxPublishConcurrency),
		opts:   opts,
	}
}

var stringDataType = aws.String("String")

// SendBatch implements driver.Topic.SendBatch.
func (t *topic) SendBatch(ctx context.Context, dms []*driver.Message) error {
	g, ctx := errgroup.WithContext(ctx)
	var ctxErr error

Loop:
	for _, dm := range dms {
		select {
		case <-ctx.Done():
			ctxErr = ctx.Err()
			break Loop
		case t.sem <- struct{}{}:
			dm := dm
			g.Go(func() error {
				defer func() { <-t.sem }()
				return t.sendMessage(ctx, dm)
			})
		}
	}
	if err := g.Wait(); err != nil {
		return err
	}
	return ctxErr
}

// sendMessage sends a single message via RPC to SNS.
func (t *topic) sendMessage(ctx context.Context, dm *driver.Message) error {
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
	_, err := t.client.PublishWithContext(ctx, &sns.PublishInput{
		Message:           aws.String(body),
		MessageAttributes: attrs,
		TopicArn:          &t.arn,
	})
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
func (t *topic) ErrorAs(err error, target interface{}) bool {
	return errorAs(err, target)
}

// ErrorCode implements driver.Topic.ErrorCode.
func (t *topic) ErrorCode(err error) gcerrors.ErrorCode {
	return errorCode(err)
}

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
	sqs.ErrCodeQueueDoesNotExist:                    gcerr.FailedPrecondition,
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
	sns.ErrCodeFilterPolicyLimitExceededException:   gcerr.ResourceExhausted,
	sns.ErrCodeSubscriptionLimitExceededException:   gcerr.ResourceExhausted,
	sns.ErrCodeTopicLimitExceededException:          gcerr.ResourceExhausted,
	sqs.ErrCodeOverLimit:                            gcerr.ResourceExhausted,
	sns.ErrCodeKMSThrottlingException:               gcerr.ResourceExhausted,
	sns.ErrCodeThrottledException:                   gcerr.ResourceExhausted,
	sns.ErrCodeEndpointDisabledException:            gcerr.Unknown,
	sns.ErrCodePlatformApplicationDisabledException: gcerr.Unknown,
}

type subscription struct {
	client *sqs.SQS
	qURL   string
}

// SubscriptionOptions will contain configuration for subscriptions.
type SubscriptionOptions struct{}

// OpenSubscription opens a on AWS SQS for the given SQS client and queue URL.
// The queue is assumed to be subscribed to some SNS topic, though there is no
// check for this.
func OpenSubscription(ctx context.Context, client *sqs.SQS, qURL string, opts *SubscriptionOptions) *pubsub.Subscription {
	return pubsub.NewSubscription(openSubscription(ctx, client, qURL), nil)
}

// openSubscription returns a driver.Subscription.
func openSubscription(ctx context.Context, client *sqs.SQS, qURL string) driver.Subscription {
	return &subscription{client: client, qURL: qURL}
}

// ReceiveBatch implements driver.Subscription.ReceiveBatch.
func (s *subscription) ReceiveBatch(ctx context.Context, maxMessages int) (msgs []*driver.Message, er error) {
	// SQS supports receiving at most 10 messages at a time:
	// https://docs.aws.amazon.com/AWSSimpleQueueService/latest/APIReference/API_ReceiveMessage.html
	max := int64(maxMessages)
	if max > 10 {
		max = 10
	}
	output, err := s.client.ReceiveMessageWithContext(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(s.qURL),
		MaxNumberOfMessages: aws.Int64(max),
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
	return ms, nil
}

// SendAcks implements driver.Subscription.SendAcks.
func (s *subscription) SendAcks(ctx context.Context, ids []driver.AckID) error {
	for _, id := range ids {
		_, err := s.client.DeleteMessage(&sqs.DeleteMessageInput{
			QueueUrl:      &s.qURL,
			ReceiptHandle: id.(*string),
		})
		if err != nil {
			return err
		}
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
func (s *subscription) ErrorAs(err error, target interface{}) bool {
	return errorAs(err, target)
}

// ErrorCode implements driver.Subscription.ErrorCode.
func (*subscription) ErrorCode(err error) gcerrors.ErrorCode {
	return errorCode(err)
}

func errorAs(err error, target interface{}) bool {
	e, ok := err.(awserr.Error)
	if !ok {
		return false
	}
	p, ok := target.(*awserr.Error)
	if !ok {
		return false
	}
	*p = e
	return true
}

// AckFunc implements driver.Subscription.AckFunc.
func (*subscription) AckFunc() func() { return nil }
