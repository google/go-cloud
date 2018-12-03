package rabbitpubsub_test

import (
	"testing"

	"github.com/google/go-cloud/internal/pubsub/rabbitpubsub"
	"github.com/streadway/amqp"
)

func TestTopicAsCanFail(t *testing.T) {
	conn := mustDialRabbit(t)
	top, err := rabbitpubsub.OpenTopic(conn, "")
	if err != nil {
		t.Fatal(err)
	}
	var conn2 amqp.Connection
	if top.As(&conn2) {
		t.Errorf("cast succeeded for %T, want failure", &conn2)
	}
}

func TestTopicAsCanSucceed(t *testing.T) {
	conn := mustDialRabbit(t)
	top, err := rabbitpubsub.OpenTopic(conn, "")
	if err != nil {
		t.Fatal(err)
	}
	var conn2 *amqp.Connection
	if !top.As(&conn2) {
		t.Fatalf("cast failed for %T", &conn2)
	}
	if conn2 != conn {
		t.Errorf("got %p, want %p", conn2, conn)
	}
}

func TestSubscriptionAsCanFail(t *testing.T) {
	conn := mustDialRabbit(t)
	sub, err := rabbitpubsub.OpenSubscription(conn, "")
	if err != nil {
		t.Fatal(err)
	}
	var conn2 amqp.Connection
	if sub.As(&conn2) {
		t.Errorf("cast succeeded for %T, want failure", &conn2)
	}
}

func TestSubscriptionAsCanSucceed(t *testing.T) {
	conn := mustDialRabbit(t)
	sub, err := rabbitpubsub.OpenSubscription(conn, "")
	if err != nil {
		t.Fatal(err)
	}
	var conn2 *amqp.Connection
	if !sub.As(&conn2) {
		t.Fatalf("cast failed for %T", &conn2)
	}
	if conn2 != conn {
		t.Errorf("got %p, want %p", conn2, conn)
	}
}

func mustDialRabbit(t *testing.T) *amqp.Connection {
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		t.Skipf("skipping because the RabbitMQ server is not up (dial error: %v)", err)
	}
	return conn
}
