package pubsub_test

import (
	"context"
	"testing"

	"github.com/google/go-cloud/pubsub"
)

func TestPubSubHappyPath(t *testing.T) {
	ctx := context.Background()
	topic := &pubsub.Topic{}
	sub := &pubsub.Subscription{}
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
