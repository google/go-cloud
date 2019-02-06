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
// It exposes the following types for As:
// Topic: *sns.SNS
// Subscription: *sqs.SQS
// Error: awserror.Error
package awspubsub

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sqs"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
)

type topic struct {
	client *sns.SNS
	arn    string
}

// TopicOptions will contain configuration for topics.
type TopicOptions struct{}

// OpenTopic opens the topic on AWS SNS for the given SNS client and topic ARN.
func OpenTopic(ctx context.Context, client *sns.SNS, topicARN string, opts *TopicOptions) *pubsub.Topic {
	return pubsub.NewTopic(openTopic(ctx, client, topicARN))
}

// openTopic returns the driver for OpenTopic. This function exists so the test
// harness can get the driver interface implementation if it needs to.
func openTopic(ctx context.Context, client *sns.SNS, topicARN string) driver.Topic {
	return &topic{client: client, arn: topicARN}
}

var stringDataType = aws.String("String")

// SendBatch implements driver.Topic.SendBatch.
func (t *topic) SendBatch(ctx context.Context, dms []*driver.Message) error {
	for _, dm := range dms {
		attrs := map[string]*sns.MessageAttributeValue{}
		for k, v := range dm.Metadata {
			attrs[k] = &sns.MessageAttributeValue{
				DataType:    stringDataType,
				StringValue: aws.String(v),
			}
		}
		_, err := t.client.Publish(&sns.PublishInput{
			Message:           aws.String(string(dm.Body)),
			MessageAttributes: attrs,
			TopicArn:          &t.arn,
		})
		if err != nil {
			return err
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
	sns.ErrCodeAuthorizationErrorException:          gcerr.AuthorizationError,
	sns.ErrCodeKMSAccessDeniedException:             gcerr.AuthorizationError,
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
	sns.ErrCodeInvalidSecurityException:             gcerr.InvalidCredentials,
	sns.ErrCodeKMSNotFoundException:                 gcerr.NotFound,
	sns.ErrCodeNotFoundException:                    gcerr.NotFound,
	sns.ErrCodeFilterPolicyLimitExceededException:   gcerr.ResourceExhausted,
	sns.ErrCodeSubscriptionLimitExceededException:   gcerr.ResourceExhausted,
	sns.ErrCodeTopicLimitExceededException:          gcerr.ResourceExhausted,
	sqs.ErrCodeOverLimit:                            gcerr.ResourceExhausted,
	sns.ErrCodeKMSThrottlingException:               gcerr.Throttled,
	sns.ErrCodeThrottledException:                   gcerr.Throttled,
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
func (s *subscription) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	output, err := s.client.ReceiveMessage(&sqs.ReceiveMessageInput{
		QueueUrl: &s.qURL,
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
		attrs := map[string]string{}
		for k, v := range body.MessageAttributes {
			attrs[k] = v.Value
		}
		m2 := &driver.Message{
			Body:     []byte(body.Message),
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
func (s *subscription) IsRetryable(error) bool {
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
func (t *subscription) ErrorCode(err error) gcerrors.ErrorCode {
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
