package rabbitpubsub

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/google/go-cloud/internal/pubsub/driver"
	"github.com/streadway/amqp"
)

func TestTopicAsCanFail(t *testing.T) {
	conn := mustDialRabbit(t)
	top, err := OpenTopic(conn, "")
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
	top, err := OpenTopic(conn, "")
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
	sub, err := makeSubscription(conn)
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
	sub, err := makeSubscription(conn)
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

// atomic. Unique ID, so tests don't interact with each other.
var uid int32

func newName(prefix string) string {
	return fmt.Sprintf("%s%d", prefix, atomic.AddInt32(&uid, 1))
}

func makeSubscription(conn *amqp.Connection) (driver.Subscription, error) {
	exchange := newName("t")
	if err := declareExchange(conn, exchange); err != nil {
		return nil, err
	}
	queue := newName("s")
	if err := bindQueue(conn, queue, exchange); err != nil {
		return nil, err
	}
	return newSubscription(conn, queue)
}
