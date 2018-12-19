package main

import (
	"github.com/streadway/amqp"
	"gocloud.dev/internal/pubsub"
	"gocloud.dev/internal/pubsub/rabbitpubsub"
)

const rabbitURL = "amqp://guest:guest@localhost:5672/"

// openRabbitTopic returns the RabbitMQ topic for the given topic ID.
func openRabbitSubscription(subID string) (*pubsub.Subscription, func(), error) {
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { conn.Close() }
	sub := rabbitpubsub.OpenSubscription(conn, subID)
	return sub, cleanup, nil
}
