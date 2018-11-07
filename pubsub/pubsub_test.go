package pubsub_test

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/google/go-cloud/pubsub"
	"github.com/google/go-cloud/pubsub/driver"
)

type driverTopic struct {
	subs []*driverSub
}

func (t *driverTopic) SendBatch(ctx context.Context, ms []*driver.Message) error {
	log.Printf("driverTopic.SendBatch: %v\n", ms)
	for _, s := range t.subs {
		s.q = append(s.q, ms...)
	}
	return nil
}

func (t *driverTopic) Close() error {
	return nil
}

type driverSub struct {
	// Normally this queue would live on a separate server in the cloud.
	q []*driver.Message
}

func (s *driverSub) ReceiveBatch(ctx context.Context) ([]*driver.Message, error) {
	ms := s.q
	s.q = nil
	return ms, nil
}

func (s *driverSub) SendAcks(ctx context.Context, ackIDs []interface{}) error {
	return nil
}

func (s *driverSub) Close() error {
	return nil
}

func TestSendReceive(t *testing.T) {
	ctx := context.Background()
	topicOpts := pubsub.TopicOptions{SendDelay: time.Millisecond, BatchSize: 10}
	ds := &driverSub{}
	dt := &driverTopic{
		subs: []*driverSub{ds},
	}
	topic := pubsub.NewTopic(ctx, dt, topicOpts)
	m := &pubsub.Message{Body: []byte("user signed up")}
	if err := topic.Send(ctx, m); err != nil {
		t.Fatal(err)
	}

	subOpts := pubsub.SubscriptionOptions{}
	sub := pubsub.NewSubscription(ds, subOpts)
	m2, err := sub.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if string(m2.Body) != string(m.Body) {
		t.Fatalf("received message has body %q, want %q", m2.Body, m.Body)
	}
}

func TestAckTriggersDriverSendAcks(t *testing.T) {
	ctx := context.Background()
	ds := &emptyDriverSub{}
	sub := pubsub.NewSubscription(ds, pubsub.SubscriptionOptions{})
	_, err := sub.Receive(ctx)
	if err == nil {
		t.Error("error expected for Receive with buggy driver")
	}
}
