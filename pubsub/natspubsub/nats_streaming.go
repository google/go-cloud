package natspubsub

import (
	"context"
	"errors"
	"sync"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/stan.go"

	"gocloud.dev/gcerrors"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
)

type stmTopic struct {
	nc   stan.Conn
	subj string
}

type StreamingTopicOptions struct {
}

// OpenStreamingTopic returns a *pubsub.Topic for use with NATS Streaming Server.
// The subject is the NATS Subject; for more info, see
// https://nats.io/documentation/writing_applications/subjects.
func OpenStreamingTopic(nc stan.Conn, subject string, _ *StreamingTopicOptions) (*pubsub.Topic, error) {
	dt, err := openStreamingTopic(nc, subject)
	if err != nil {
		return nil, err
	}
	return pubsub.NewTopic(dt, nil), nil
}

func openStreamingTopic(nc stan.Conn, subject string) (driver.Topic, error) {
	if nc == nil {
		return nil, errors.New("natspubsub: stan.Conn is required")
	}

	return &stmTopic{nc, subject}, nil
}

func (t *stmTopic) SendBatch(ctx context.Context, msgs []*driver.Message) error {
	if t == nil || t.nc == nil {
		return errNotInitialized
	}

	for _, m := range msgs {
		if err := ctx.Err(); err != nil {
			return err
		}

		payload, err := encodeMessage(m)
		if err != nil {
			return err
		}

		if m.BeforeSend != nil {
			asFunc := func(i interface{}) bool { return false }
			if err := m.BeforeSend(asFunc); err != nil {
				return err
			}
		}

		if err := t.nc.Publish(t.subj, payload); err != nil {
			return err
		}
	}

	return nil
}

func (t *stmTopic) IsRetryable(err error) bool {
	return false
}

func (t *stmTopic) As(i interface{}) bool {
	c, ok := i.(*stan.Conn)
	if !ok {
		return false
	}
	*c = t.nc
	return true
}

func (t *stmTopic) ErrorAs(error, interface{}) bool {
	return false
}

func (t *stmTopic) ErrorCode(err error) gcerrors.ErrorCode {
	switch err {
	case nil:
		return gcerrors.OK
	case context.Canceled:
		return gcerrors.Canceled
	case errNotInitialized:
		return gcerrors.NotFound
	case nats.ErrBadSubject, stan.ErrBadConnection:
		return gcerrors.FailedPrecondition
	case nats.ErrAuthorization:
		return gcerrors.PermissionDenied
	case nats.ErrMaxPayload, nats.ErrReconnectBufExceeded:
		return gcerrors.ResourceExhausted
	case stan.ErrTimeout:
		return gcerrors.DeadlineExceeded
	}
	return gcerrors.Unknown
}

func (t *stmTopic) Close() error {
	return nil
}

type stmSub struct {
	nc       stan.Conn
	sub      stan.Subscription
	ack      bool
	messages []*stan.Msg
	sync.Mutex
}

// OpenStreamingSubscription returns a *pubsub.Subscription representing a NATS Streaming subscription.
// The subject is the NATS Subject to subscribe to; for more info, see
// https://nats.io/documentation/writing_applications/subjects.
func OpenStreamingSubscription(nc stan.Conn, subject, queue string, oo ...stan.SubscriptionOption) (*pubsub.Subscription, error) {
	ds, err := openStreamingSubscription(nc, subject, queue, oo)
	if err != nil {
		return nil, err
	}
	return pubsub.NewSubscription(ds, nil, nil), nil
}

func openStreamingSubscription(nc stan.Conn, subject, queue string, oo []stan.SubscriptionOption) (driver.Subscription, error) {
	opt := stan.DefaultSubscriptionOptions
	for _, o := range oo {
		if err := o(&opt); err != nil {
			return nil, err
		}
	}

	s := &stmSub{nc: nc, ack: opt.ManualAcks}
	handler := func(msg *stan.Msg) {
		s.Lock()
		s.messages = append(s.messages, msg)
		s.Unlock()
	}

	var err error
	if queue != "" {
		s.sub, err = nc.QueueSubscribe(subject, queue, handler)
	} else {
		s.sub, err = nc.Subscribe(subject, handler)
	}
	if err != nil {
		return nil, err
	}

	return s, nil
}

func (s *stmSub) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	if s == nil || s.nc == nil {
		return nil, stan.ErrBadSubscription
	}

	mc := maxMessages
	s.Lock()
	if mc > len(s.messages) {
		mc = len(s.messages)
	}
	msgs := s.messages[:mc]
	s.messages = s.messages[mc:]
	s.Unlock()

	dms := make([]*driver.Message, 0, len(msgs))

	for _, m := range msgs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		dm, err := decodeStreaming(m)
		if err != nil {
			return nil, err
		}
		dms = append(dms, dm)
	}

	return dms, nil
}

func (s *stmSub) SendAcks(ctx context.Context, ackIDs []driver.AckID) error {
	if !s.ack { // Auto acknowledge is set
		return nil
	}

	for _, a := range ackIDs {
		if err := a.(*stan.Msg).Ack(); err != nil {
			return err
		}
	}

	return nil
}

func (s *stmSub) CanNack() bool {
	return false
}

func (s *stmSub) SendNacks(ctx context.Context, ackIDs []driver.AckID) error {
	panic("unreachable")
}

func (s *stmSub) IsRetryable(err error) bool {
	return false
}

func (s *stmSub) As(i interface{}) bool {
	c, ok := i.(*stan.Subscription)
	if !ok {
		return false
	}
	*c = s.sub
	return true
}

func (s *stmSub) ErrorAs(error, interface{}) bool {
	return false
}

func (s *stmSub) ErrorCode(err error) gcerrors.ErrorCode {
	switch err {
	case nil:
		return gcerrors.OK
	case context.Canceled:
		return gcerrors.Canceled
	case errNotInitialized, nats.ErrBadSubscription:
		return gcerrors.NotFound
	case nats.ErrBadSubject, nats.ErrTypeSubscription:
		return gcerrors.FailedPrecondition
	case nats.ErrAuthorization:
		return gcerrors.PermissionDenied
	case nats.ErrMaxMessages, nats.ErrSlowConsumer:
		return gcerrors.ResourceExhausted
	case nats.ErrTimeout:
		return gcerrors.DeadlineExceeded
	}
	return gcerrors.Unknown
}

func (s *stmSub) Close() error {
	return s.sub.Close()
}

func decodeStreaming(msg *stan.Msg) (*driver.Message, error) {
	if msg == nil {
		return nil, nats.ErrInvalidMsg
	}
	var dm driver.Message
	if err := decodeMessage(msg.Data, &dm); err != nil {
		return nil, err
	}
	dm.AckID = msg
	dm.AsFunc = streamingMessageAsFunc(msg)
	return &dm, nil
}

func streamingMessageAsFunc(msg *stan.Msg) func(interface{}) bool {
	return func(i interface{}) bool {
		p, ok := i.(**stan.Msg)
		if !ok {
			return false
		}
		*p = msg
		return true
	}
}
