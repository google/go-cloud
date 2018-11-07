package driver

import "context"

type AckID interface{}

type Message struct {
	// Body contains the content of the message.
	Body []byte

	// Attributes has key/value metadata for the message.
	Attributes map[string]string

	// AckID identifies the message on the server.
	// It can be used to ack the message after it has been received.
	AckID AckID

	// errChan relays back an error or nil as the result of sending the
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
	// server so that they
	// will not be received again for this subscription. This method
	// returns only after all the ackIDs are sent.
	SendAcks(ctx context.Context, ackIDs []AckID) error

	// Close disconnects the Subscription.
	Close() error
}
