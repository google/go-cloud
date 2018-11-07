package pubsub_test

import (
	"context"
	"testing"

	"github.com/google/go-cloud/pubsub"
	"github.com/google/go-cloud/pubsub/driver"
)

// emptyDriverSub is an intentionally buggy subscription driver. Such drivers should
// wait until some messages are available and then return a non-empty batch. This
// driver mischeviously always returns an empty batch.
type emptyDriverSub struct{}

func (s *emptyDriverSub) ReceiveBatch(ctx context.Context) ([]*driver.Message, error) {
	return nil, nil
}

func (s *emptyDriverSub) SendAcks(ctx context.Context, ackIDs []interface{}) error {
	return nil
}

func (s *emptyDriverSub) Close() error {
	return nil
}

func TestReceiveErrorIfEmptyBatchReturnedFromDriver(t *testing.T) {
	ctx := context.Background()
	ds := &emptyDriverSub{}
	sub := pubsub.NewSubscription(ds, pubsub.SubscriptionOptions{})
	_, err := sub.Receive(ctx)
	if err == nil {
		t.Error("error expected for Receive with buggy driver")
	}
}
