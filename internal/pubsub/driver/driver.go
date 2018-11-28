// Copyright 2018 The Go Cloud Authors
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

// Package driver defines a set of interfaces that the pubsub package uses to
// interact with the underlying pubsub services.
package driver

import (
	"context"
)

// Batcher should gather items into batches to be sent to the pubsub service.
type Batcher interface {
	// Add should add an item to the batcher.
	Add(ctx context.Context, item interface{}) error

	// AddNoWait should add an item to the batcher without blocking.
	AddNoWait(item interface{}) <-chan error

	// Shutdown should wait for all active calls to Add to finish, then
	// return. After Shutdown is called, all calls to Add should fail.
	Shutdown()
}

// AckID is the identifier of a message for purposes of acknowledgement.
type AckID interface{}

// Message is data to be published (sent) to a topic and later received from
// subscriptions on that topic.
type Message struct {
	// Body contains the content of the message.
	Body []byte

	// Metadata has key/value pairs describing the message.
	Metadata map[string]string

	// AckID should be set to something identifying the message on the
	// server. It may be passed to Subscription.SendAcks() to acknowledge
	// the message. This field should only be set by methods implementing
	// Subscription.ReceiveBatch.
	AckID AckID
}

// Topic publishes messages.
type Topic interface {
	// SendBatch publishes all the messages in ms. This method should
	// return only after all the messages are sent, an error occurs, or the
	// context is done.
	//
	// Only the Body and (optionally) Metadata fields of the Messages in ms
	// should be set by the caller of SendBatch.
	//
	// Only one RPC should be made to send the messages, and the returned
	// error should be based on the result of that RPC. Implementations
	// that send only one message at a time should return a non-nil error
	// if len(ms) != 1.
	//
	// If there is a transient failure, this method should not retry but should
	// return an error. The concrete API will take care of retry logic.
	//
	// The slice ms should not be retained past the end of the call to
	// SendBatch.
	//
	// SendBatch should be safe for concurrent access from multiple goroutines.
	SendBatch(ctx context.Context, ms []*Message) error

	// IsRetryable should report whether err can be retried.
	// err will always be a non-nil error returned from SendBatch.
	IsRetryable(err error) bool
}

// Subscription receives published messages.
type Subscription interface {
	// ReceiveBatch should return a batch of messages that have queued up
	// for the subscription on the server, up to maxMessages.
	//
	// If there is a transient failure, this method should not retry but
	// should return a nil slice and an error. The concrete API will take
	// care of retry logic.
	//
	// If the service returns no messages for some other reason, this
	// method should return the empty slice of messages and not attempt to
	// retry.
	//
	// Implementations of ReceiveBatch should request that the underlying
	// service wait some non-zero amount of time before returning, if there
	// are no messages yet.
	//
	// ReceiveBatch should be safe for concurrent access from multiple goroutines.
	ReceiveBatch(ctx context.Context, maxMessages int) ([]*Message, error)

	// SendAcks should acknowledge the messages with the given ackIDs on
	// the server so that they will not be received again for this
	// subscription if the server gets the acks before their deadlines.
	// This method should return only after all the ackIDs are sent, an
	// error occurs, or the context is done.
	//
	// Only one RPC should be made to send the messages, and the returned
	// error should be based on the result of that RPC.  Implementations
	// that send only one ack at a time should return a non-nil error if
	// len(ackIDs) != 1.
	//
	// SendAcks should be safe for concurrent access from multiple goroutines.
	SendAcks(ctx context.Context, ackIDs []AckID) error

	// IsRetryable should report whether err can be retried.
	// err will always be a non-nil error returned from ReceiveBatch or SendAcks.
	IsRetryable(err error) bool
}
