package pubsub_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cloud/pubsub"
	"github.com/google/go-cloud/pubsub/driver"
)

type driverTopic struct {
	subs []*driverSub
}

func (t *driverTopic) SendBatch(ctx context.Context, ms []*driver.Message) error {
	return nil
}

func (t *driverTopic) Close() error {
	return nil
}

type driverSub struct {
	q []*driver.Message
}

func (s *driverSub) ReceiveBatch(ctx context.Context) ([]*driver.Message, error) {
	return nil, nil
}

func (s *driverSub) SendAcks(ctx context.Context, ackIDs []interface{}) error {
	return nil
}

func (s *driverSub) Close() error {
	return nil
}

func TestPubSubHappyPath(t *testing.T) {
	ctx := context.Background()
	topicOpts := pubsub.TopicOptions{SendDelay: time.Millisecond, BatchSize: 10}
	ds := &driverSub{}
	dt := &driverTopic{
		subs: []*driverSub{ds},
	}
	topic := pubsub.NewTopic(ctx, dt, topicOpts)
	subOpts := pubsub.SubscriptionOptions{
		AckDelay:     time.Millisecond,
		AckBatchSize: 10,
		AckDeadline:  time.Millisecond,
	}
	sub := pubsub.NewSubscription(ds, subOpts)
	m := &pubsub.Message{Body: []byte("user signed up")}
	if err := topic.Send(ctx, m); err != nil {
		t.Fatal(err)
	}
	m2, err := sub.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := m2.Ack(ctx); err != nil {
		t.Fatal(err)
	}
	if string(m2.Body) != string(m.Body) {
		t.Fatalf("received message has body %q, want %q", m2.Body, m.Body)
	}
}
