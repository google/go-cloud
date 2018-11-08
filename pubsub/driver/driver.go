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

type Message struct {
	// Body contains the content of the message.
	Body []byte

	// Attributes has key/value metadata for the message.
	Attributes map[string]string

	// AckID identifies the message on the server. It can be used to ack
	// the message after it has been received.
	AckID AckID

	// ErrChan relays back an error or nil as the result of sending the
	// batch that includes this message.
	ErrChan chan error
}

// Topic publishes messages.
type Topic interface {
	// SendBatch publishes all the messages in ms.
	SendBatch(ctx context.Context, ms []*Message) error

	// Close disconnects the Topic.
	Close() error
}

// Subscription receives published messages.
type Subscription interface {
	// ReceiveBatch returns a batch of messages that have queued up for the
	// subscription on the server.
	ReceiveBatch(ctx context.Context) ([]*Message, error)

	// SendAcks acknowledges the messages with the given ackIDs on the
	// server so that they will not be received again for this
	// subscription. This method returns only after all the ackIDs are
	// sent.
	SendAcks(ctx context.Context, ackIDs []interface{}) error

	// Close disconnects the Subscription.
	Close() error
}
