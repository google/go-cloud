package pubsub_test

import (
	"context"
	"testing"

	"github.com/golang/go/src/pkg/math/rand"
	"github.com/google/go-cloud/pubsub"
	"github.com/google/go-cloud/pubsub/driver"
)

type ackingDriverSub struct {
	q        []*driver.Message
	sendAcks func(context.Context, []driver.AckID) error
}

func (s *ackingDriverSub) ReceiveBatch(ctx context.Context) ([]*driver.Message, error) {
	ms := s.q
	s.q = nil
	return ms, nil
}

func (s *ackingDriverSub) SendAcks(ctx context.Context, ackIDs []driver.AckID) error {
	return s.sendAcks(ctx, ackIDs)
}

func (s *ackingDriverSub) Close() error {
	return nil
}

func TestAckTriggersDriverSendAcks(t *testing.T) {
	ctx := context.Background()
	var sentAcks []driver.AckID
	f := func(ctx context.Context, ackIDs []driver.AckID) error {
		sentAcks = ackIDs
		return nil
	}
	id := rand.Int()
	m := &driver.Message{AckID: id}
	ds := &ackingDriverSub{
		q:        []*driver.Message{m},
		sendAcks: f,
	}
	sub := pubsub.NewSubscription(ds, pubsub.SubscriptionOptions{})
	m2, err := sub.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := m2.Ack(ctx); err != nil {
		t.Fatal(err)
	}
	if len(sentAcks) != 1 {
		t.Errorf("len(sentAcks) = %d, want exactly 1", len(sentAcks))
	}
	if sentAcks[0] != id {
		t.Errorf("sentAcks[0] = %d, want %d", sentAcks[0], id)
	}
}

func TestMsgAckReturnsErrorFromSendAcks(t *testing.T) {

}

func TestAckIsCancellable(t *testing.T) {

}
