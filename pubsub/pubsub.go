package pubsub

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/go-cloud/pubsub/driver"
	"google.golang.org/api/support/bundler"
)

// Message contains data to be published.
type Message struct {
	// Body contains the content of the message.
	Body []byte

	// Metadata has key/value metadata for the message.
	Metadata map[string]string

	// AckID identifies the message on the server.
	// It can be used to ack the message after it has been received.
	ackID AckID

	// sub is the Subscription this message was received from.
	sub *Subscription
}

type AckID interface{}

// Ack acknowledges the message, telling the server that it does not need to
// be sent again to the associated Subscription. This method blocks until
// the message has been confirmed as acknowledged on the server, or failure
// occurs.
func (m *Message) Ack(ctx context.Context) error {
	// Send the message back to the subscriber for ack batching.
	mec := msgErrChan{
		msg:     m,
		errChan: make(chan error),
	}
	// FIXME: Figure out the real size of the ackIDs.
	size := 8
	if err := m.sub.ackBatcher.AddWait(ctx, mec, size); err != nil {
		return err
	}
	return <-mec.errChan
}

type msgErrChan struct {
	msg     *Message
	errChan chan error
}

// Topic publishes messages to all its subscribers.
type Topic struct {
	driver  driver.Topic
	batcher *bundler.Bundler
}

// TopicOptions contains configuration for Topics.
type TopicOptions struct {
	// SendDelay tells the max duration to wait before sending the next batch of
	// messages to the server.
	SendDelay time.Duration

	// BatchSize specifies the maximum number of messages that can go in a batch
	// for sending.
	BatchSize int
}

// Send publishes a message. It only returns after the message has been
// sent, or failed to be sent. Send can be called from multiple goroutines
// at once.
func (t *Topic) Send(ctx context.Context, m *Message) error {
	mec := msgErrChan{
		msg:     m,
		errChan: make(chan error),
	}
	// FIXME: add sizes of Metadata
	size := len(m.Body)
	if err := t.batcher.AddWait(ctx, mec, size); err != nil {
		return err
	}
	return <-mec.errChan
}

// Close disconnects the Topic.
func (t *Topic) Close() error {
	t.batcher.Flush()
	return t.driver.Close()
}

// NewTopic makes a pubsub.Topic from a driver.Topic and opts to
// tune how messages are sent. Behind the scenes, NewTopic spins up a goroutine
// to bundle messages into batches and send them to the server.
func NewTopic(ctx context.Context, d driver.Topic, opts TopicOptions) *Topic {
	handler := func(item interface{}) {
		mecs, ok := item.([]msgErrChan)
		if !ok {
			panic("failed conversion to []msgErrChan in bundler handler")
		}
		dms := make([]*driver.Message, len(mecs))
		for _, mec := range mecs {
			m := mec.msg
			dm := &driver.Message{
				Body:     m.Body,
				Metadata: m.Metadata,
				AckID:    m.ackID,
			}
			dms = append(dms, dm)
		}
		log.Printf("sending off %d messages", len(dms))
		err := d.SendBatch(ctx, dms)
		for _, mec := range mecs {
			mec.errChan <- err
		}
	}
	t := &Topic{
		driver:  d,
		batcher: bundler.NewBundler(msgErrChan{}, handler),
	}
	return t
}

// Subscription receives published messages.
type Subscription struct {
	driver driver.Subscription

	// ackBatcher makes batches of acks and sends them to the server.
	ackBatcher *bundler.Bundler

	// q is the local queue of messages downloaded from the server.
	q []*Message
}

// SubscriptionOptions contains configuration for Subscriptions.
type SubscriptionOptions struct {
	// AckDelay tells the max duration to wait before sending the next batch
	// of acknowledgements back to the server.
	AckDelay time.Duration

	// AckBatchSize is the maximum number of acks that should be sent to
	// the server in a batch.
	AckBatchSize int

	// AckDeadline tells how long the server should wait before assuming a
	// received message has failed to be processed.
	AckDeadline time.Duration
}

// Receive receives and returns the next message from the Subscription's queue,
// blocking and polling if none are available. This method can be called
// concurrently from multiple goroutines. On systems that support acks, the
// Ack() method of the returned Message has to be called once the message has
// been processed, to prevent it from being received again.
func (s *Subscription) Receive(ctx context.Context) (*Message, error) {
	if len(s.q) == 0 {
		if err := s.getNextBatch(ctx); err != nil {
			return nil, err
		}
	}
	m := s.q[0]
	s.q = s.q[1:]
	return m, nil
}

// getNextBatch gets the next batch of messages from the server.
func (s *Subscription) getNextBatch(ctx context.Context) error {
	msgs, err := s.driver.ReceiveBatch(ctx)
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		if len(msgs) == 0 {
			return errors.New("subscription driver bug: received empty batch")
		}
		s.q = make([]*Message, len(msgs))
		for i, m := range msgs {
			log.Printf("putting message %+v on the queue", m)
			s.q[i] = &Message{
				Body:     m.Body,
				Metadata: m.Metadata,
				ackID:    m.AckID,
				sub:      s,
			}
		}
	}
	return nil
}

// Close disconnects the Subscription.
func (s *Subscription) Close() error {
	s.ackBatcher.Flush()
	return s.driver.Close()
}

// NewSubscription creates a Subscription from a driver.Subscription and opts to
// tune sending and receiving of acks and messages. Behind the scenes,
// NewSubscription spins up a goroutine to gather acks into batches and
// periodically send them to the server.
func NewSubscription(ctx context.Context, d driver.Subscription, opts SubscriptionOptions) *Subscription {
	// Fill in defaults for zeros in the opts.
	if opts.AckDelay == 0 {
		opts.AckDelay = time.Millisecond
	}
	if opts.AckBatchSize == 0 {
		opts.AckBatchSize = 100
	}

	handler := func(item interface{}) {
		mecs, ok := item.([]msgErrChan)
		if !ok {
			panic("failed conversion to []msgErrChan in bundler handler")
		}
		ids := make([]driver.AckID, len(mecs))
		for _, mec := range mecs {
			m := mec.msg
			id := m.ackID
			ids = append(ids, id)
		}
		log.Printf("sending off %d ack ids", len(ids))
		err := d.SendAcks(ctx, ids)
		for _, mec := range mecs {
			mec.errChan <- err
		}
	}

	// Details similar to the body of NewTopic should go here.
	s := &Subscription{
		driver:     d,
		ackBatcher: bundler.NewBundler(msgErrChan{}, handler),
	}
	return s
}
