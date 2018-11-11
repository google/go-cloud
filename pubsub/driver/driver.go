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

import "context"

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
	// context is cancelled.
	//
	// Only the Body and (optionally) Metadata fields of the Messages in ms
	// should be set by the caller of SendBatch.
	//
	// Only one RPC should be made to send the messages, and the returned
	// error should be based on the result of that RPC. Implementations
	// that send only one message at a time should return a non-nil error
	// if len(ms) != 1. Such implementations should set the batch size
	// to 1 in the call to pubsub.NewTopic from the OpenTopic func for
	// their package.
	//
	// SendBatch is called sequentially for each batch, not concurrently,
	// so it does not have to be safe to call from multiple goroutines.
	//
	// The slice ms should not be retained past the end of the call to
	// SendBatch.
	SendBatch(ctx context.Context, ms []*Message) error

	// Close should disconnect the Topic.
	//
	// If Close is called after a call to SendBatch begins but before it
	// ends, then the call to Close should wait for the SendBatch call to
	// end, and then Close should finish.
	//
	// If Close is called and SendBatch is called before Close finishes,
	// then the call to Close should proceed and the call to SendBatch
	// should fail immediately after Close returns.
	Close() error
}

// Subscription receives published messages.
type Subscription interface {
	// ReceiveBatch should return a batch of messages that have queued up
	// for the subscription on the server. If no messages are available
	// yet, it must block until there is at least one, or the context is
	// done.
	ReceiveBatch(ctx context.Context) ([]*Message, error)

	// SendAcks should acknowledge the messages with the given ackIDs on
	// the server so that they will not be received again for this
	// subscription. This method should return only after all the ackIDs
	// are sent, an error occurs, or the context is cancelled.
	//
	// Only one RPC should be made to send the messages, and the returned
	// error should be based on the result of that RPC.  Implementations
	// that send only one ack at a time should return a non-nil error if
	// len(ackIDs) != 1. Such implementations should set AckBatchSize to
	// 1 in the call to pubsub.NewSubscription in the OpenSubscription
	// func for their package.
	SendAcks(ctx context.Context, ackIDs []AckID) error

	// Close should disconnect the Subscription.
	//
	// If Close is called after a call to ReceiveBatch/SendAcks begins but
	// before it ends, then the call to Close should wait for the other
	// call to end, and then Close should finish.
	//
	// If Close is called and ReceiveBatch/SendAcks is called before Close
	// finishes, then the call to Close should proceed and the other call
	// should fail immediately after Close returns.
	Close() error
}
