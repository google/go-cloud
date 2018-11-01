package pubsub

import (
	"context"
	"github.com/google/go-cloud/pubsub/driver"
)

// Message contains data to be published.
type Message struct {
	// Body contains the content of the message.
	Body []byte

	// Attributes contains key/value pairs with metadata about the message.
	Attributes map[string]string

	// ackID is an ID for the message on the server, used for acking.
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
	// Send the ack ID back to the subscriber for batching.
	m.sub.ackChan <- m.ackID
	select {
	case err := <-m.sub.ackErrChan:
		return err
	case <-ctx.Done():
		return nil
	}
}

// Topic publishes messages to all its subscribers.
type Topic struct {
	driver   driver.Topic
	mcChan   chan msgCtx
	doneChan chan struct{}
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

// msgCtx pairs a Message with the Context of its Send call.
type msgCtx struct {
	msg *Message
	ctx context.Context
}

// Send publishes a message. It only returns after the message has been
// sent, or failed to be sent. Send can be called from multiple goroutines
// at once.
func (t *Topic) Send(ctx context.Context, m *Message) error {
	t.mcChan <- msgCtx{m, ctx}
	// Wait for the batch including this message to be sent to the server.
	return <-m.errChan
}

// Close disconnects the Topic.
func (t *Topic) Close() error {
	close(t.doneChan)
	return t.driver.Close()
}

// NewTopic makes a pubsub.Topic from a driver.Topic and opts to
// tune how messages are sent. Behind the scenes, NewTopic spins up a goroutine
// to bundle messages into batches and send them to the server.
func NewTopic(d driver.Topic, opts TopicOptions) *Topic {
	t := &Topic{
		driver:   d,
		mcChan:   make(chan msgCtx),
		doneChan: make(chan struct{}),
	}
	go func() {
		// Pull messages from t.mcChan and put them in batches. Send the current
		// batch whenever it is large enough or enough time has elapsed since
		// the last send.
		for {
			batch := make([]*driver.Message, 0, batchSize)
			timeout := time.After(opts.SendDelay)
		Loop:
			for i := 0; i < opts.BatchSize; i++ {
				select {
				case <-timeout:
					// Time to send the batch, even if it isn't full.
					break Loop
				case mc := <-t.mcChan:
					select {
					case <-mc.ctx.Done():
						// This message's Send call was cancelled, so just skip
						// over it.
					default:
						dm := &driver.Message{
							Body:       m.msg.Body,
							Attributes: m.msg.Attributes,
							AckID:      m.msg.AckID,
						}
						batch = append(batch, dm)
					}
				case <-t.doneChan:
					return
				}
			}
			if len(batch) > 0 {
				err := t.driver.SendBatch(ctx, batch)
				for _, m := range batch {
					m.errChan <- err
				}
			}
		}
	}()
	return t
}

// Subscription receives published messages.
type Subscription struct {
	driver driver.Subscription

	// ackChan conveys ackIDs from Message.Ack to the ack batcher goroutine.
	ackChan chan AckID

	// ackErrChan reports errors back to Message.Ack.
	ackErrChan chan error

	// doneChan tells the goroutine from startAckBatcher to finish.
	doneChan chan struct{}

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
// blocking if none are available. This method can be called concurrently from
// multiple goroutines. On systems that support acks, the Ack() method of the
// returned Message has to be called once the message has been processed, to
// prevent it from being received again.
func (s *Subscription) Receive(ctx context.Context) (*Message, error) {
	if len(s.q) == 0 {
		// Get the next batch of messages from the server.
	Loop:
		for {
			msgs, err := s.driver.ReceiveBatch(ctx)
			if err != nil {
				return nil, err
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
				if len(msgs) > 0 {
					s.q = make([]*Message, len(msgs))
					for _, m := range msgs {
						s.q[i] = &Message{
							Body:       m.Body,
							Attributes: m.Attributes,
							ackID:      m.AckID,
							sub:        s,
						}
					}
					break Loop
				}
			}
		}
	}
	m := s.q[0]
	s.q = s.q[1:]
	return m, nil
}

// Close disconnects the Subscription.
func (s *Subscription) Close() error {
	close(s.doneChan)
	return s.driver.Close()
}

// NewSubscription creates a Subscription from a driver.Subscription and opts to
// tune sending and receiving of acks and messages. Behind the scenes,
// NewSubscription spins up a goroutine to gather acks into batches and
// periodically send them to the server.
func NewSubscription(s driver.Subscription, opts SubscriptionOptions) *Subscription {
	// Details similar to the body of NewTopic should go here.
}
