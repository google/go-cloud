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

// Package driver defines interfaces to be implemented by pubsub drivers, which
// will be used by the pubsub package to interact with the underlying services.
// Application code should use package pubsub.
package driver // import "gocloud.dev/pubsub/driver"

import (
	"context"

	"gocloud.dev/gcerrors"
)

// AckID is the identifier of a message for purposes of acknowledgement.
type AckID any

// AckInfo represents an action on an AckID.
type AckInfo struct {
	// AckID is the AckID the action is for.
	AckID AckID
	// IsAck is true if the AckID should be acked, false if it should be nacked.
	IsAck bool
}

// Message is data to be published (sent) to a topic and later received from
// subscriptions on that topic.
type Message struct {
	// LoggableID should be set to an opaque message identifer for
	// received messages.
	LoggableID string

	// Body contains the content of the message.
	Body []byte

	// Metadata has key/value pairs describing the message.
	Metadata map[string]string

	// AckID should be set to something identifying the message on the
	// server. It may be passed to Subscription.SendAcks to acknowledge
	// the message, or to Subscription.SendNacks. This field should only
	// be set by methods implementing Subscription.ReceiveBatch.
	AckID AckID

	// AsFunc allows drivers to expose driver-specific types;
	// see Topic.As for more details.
	// AsFunc must be populated on messages returned from ReceiveBatch.
	AsFunc func(any) bool

	// BeforeSend is a callback used when sending a message. It should remain
	// nil on messages returned from ReceiveBatch.
	//
	// The callback must be called exactly once, before the message is sent.
	//
	// asFunc converts its argument to driver-specific types.
	// See https://gocloud.dev/concepts/as/ for background information.
	BeforeSend func(asFunc func(any) bool) error

	// AfterSend is a callback used when sending a message. It should remain
	// nil on messages returned from ReceiveBatch.
	//
	// The callback must be called at most once, after the message is sent.
	// If Send returns an error, AfterSend will not be called.
	//
	// asFunc converts its argument to driver-specific types.
	// See https://gocloud.dev/concepts/as/ for background information.
	AfterSend func(asFunc func(any) bool) error
}

// ByteSize estimates the size in bytes of the message for the purpose of restricting batch sizes.
func (m *Message) ByteSize() int {
	return len(m.Body)
}

// Topic publishes messages.
// Drivers may optionally also implement io.Closer; Close will be called
// when the pubsub.Topic is Shutdown.
type Topic interface {
	// SendBatch should publish all the messages in ms. It should
	// return only after all the messages are sent, an error occurs, or the
	// context is done.
	//
	// Only the Body and (optionally) Metadata fields of the Messages in ms
	// will be set by the caller of SendBatch.
	//
	// If any message in the batch fails to send, SendBatch should return an
	// error.
	//
	// If there is a transient failure, this method should not retry but
	// should return an error for which IsRetryable returns true. The
	// concrete API takes care of retry logic.
	//
	// The slice ms should not be retained past the end of the call to
	// SendBatch.
	//
	// SendBatch may be called concurrently from multiple goroutines.
	//
	// Drivers can control the number of messages sent in a single batch
	// and the concurrency of calls to SendBatch via a batcher.Options
	// passed to pubsub.NewTopic.
	SendBatch(ctx context.Context, ms []*Message) error

	// IsRetryable should report whether err can be retried.
	// err will always be a non-nil error returned from SendBatch.
	IsRetryable(err error) bool

	// As allows drivers to expose driver-specific types.
	// See https://gocloud.dev/concepts/as/ for background information.
	As(i any) bool

	// ErrorAs allows drivers to expose driver-specific types for errors.
	// See https://gocloud.dev/concepts/as/ for background information.
	ErrorAs(error, any) bool

	// ErrorCode should return a code that describes the error, which was returned by
	// one of the other methods in this interface.
	ErrorCode(error) gcerrors.ErrorCode

	// Close cleans up any resources used by the Topic. Once Close is called,
	// there will be no method calls to the Topic other than As, ErrorAs, and
	// ErrorCode.
	Close() error
}

// Subscription receives published messages.
// Drivers may optionally also implement io.Closer; Close will be called
// when the pubsub.Subscription is Shutdown.
type Subscription interface {
	// ReceiveBatch should return a batch of messages that have queued up
	// for the subscription on the server, up to maxMessages.
	//
	// If there is a transient failure, this method should not retry but
	// should return a nil slice and an error. The concrete API will take
	// care of retry logic.
	//
	// If no messages are currently available, this method should block for
	// no more than about 1 second. It can return an empty
	// slice of messages and no error. ReceiveBatch will be called again
	// immediately, so implementations should try to wait for messages for some
	// non-zero amount of time before returning zero messages. If the underlying
	// service doesn't support waiting, then a time.Sleep can be used.
	//
	// ReceiveBatch may be called concurrently from multiple goroutines.
	//
	// Drivers can control the maximum value of maxMessages and the concurrency
	// of calls to ReceiveBatch via a batcher.Options passed to
	// pubsub.NewSubscription.
	ReceiveBatch(ctx context.Context, maxMessages int) ([]*Message, error)

	// SendAcks should acknowledge the messages with the given ackIDs on
	// the server so that they will not be received again for this
	// subscription if the server gets the acks before their deadlines.
	// This method should return only after all the ackIDs are sent, an
	// error occurs, or the context is done.
	//
	// It is acceptable for SendAcks to be a no-op for drivers that don't
	// support message acknowledgement.
	//
	// Drivers should suppress errors caused by double-acking a message.
	//
	// SendAcks may be called concurrently from multiple goroutines.
	//
	// Drivers can control the maximum size of ackIDs and the concurrency
	// of calls to SendAcks/SendNacks via a batcher.Options passed to
	// pubsub.NewSubscription.
	SendAcks(ctx context.Context, ackIDs []AckID) error

	// CanNack must return true iff the driver supports Nacking messages.
	//
	// If CanNack returns false, SendNacks will never be called, and Nack will
	// panic if called.
	CanNack() bool

	// SendNacks should notify the server that the messages with the given ackIDs
	// are not being processed by this client, so that they will be received
	// again later, potentially by another subscription.
	// This method should return only after all the ackIDs are sent, an
	// error occurs, or the context is done.
	//
	// If the service does not suppport nacking of messages, return false from
	// CanNack, and SendNacks will never be called.
	//
	// SendNacks may be called concurrently from multiple goroutines.
	//
	// Drivers can control the maximum size of ackIDs and the concurrency
	// of calls to SendAcks/Nacks via a batcher.Options passed to
	// pubsub.NewSubscription.
	SendNacks(ctx context.Context, ackIDs []AckID) error

	// IsRetryable should report whether err can be retried.
	// err will always be a non-nil error returned from ReceiveBatch or SendAcks.
	IsRetryable(err error) bool

	// As converts i to driver-specific types.
	// See https://gocloud.dev/concepts/as/ for background information.
	As(i any) bool

	// ErrorAs allows drivers to expose driver-specific types for errors.
	// See https://gocloud.dev/concepts/as/ for background information.
	ErrorAs(error, any) bool

	// ErrorCode should return a code that describes the error, which was returned by
	// one of the other methods in this interface.
	ErrorCode(error) gcerrors.ErrorCode

	// Close cleans up any resources used by the Topic. Once Close is called,
	// there will be no method calls to the Topic other than As, ErrorAs, and
	// ErrorCode.
	Close() error
}
