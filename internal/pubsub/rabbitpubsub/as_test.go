package rabbitpubsub_test

import (
	"testing"

	"github.com/google/go-cloud/internal/pubsub/rabbitpubsub"
	"github.com/streadway/amqp"
)

var name = ""
var conn *amqp.Connection

func TestTopicAsCanFail(t *testing.T) {
	top, err := rabbitpubsub.OpenTopic(conn, name)
	if err != nil {
		t.Fatal(err)
	}
	var conn2 amqp.Connection
	if top.As(&conn2) {
		t.Errorf("cast succeeded for %T, want failure", &conn2)
	}
}

func TestTopicAsCanSucceed(t *testing.T) {
	top, err := rabbitpubsub.OpenTopic(conn, name)
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
	sub, err := rabbitpubsub.OpenSubscription(conn, name)
	if err != nil {
		t.Fatal(err)
	}
	var conn2 amqp.Connection
	if sub.As(&conn2) {
		t.Errorf("cast succeeded for %T, want failure", &conn2)
	}
}

func TestSubscriptionAsCanSucceed(t *testing.T) {
	sub, err := rabbitpubsub.OpenSubscription(conn, name)
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
