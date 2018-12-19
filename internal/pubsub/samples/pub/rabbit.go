package main

import (
	"github.com/streadway/amqp"
	"gocloud.dev/internal/pubsub"
	"gocloud.dev/internal/pubsub/rabbitpubsub"
)

const rabbitURL = "amqp://guest:guest@localhost:5672/"

// openRabbitTopic returns the RabbitMQ topic for the given topic ID.
func openRabbitTopic(topicID string) (*pubsub.Topic, func(), error) {
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { conn.Close() }
	topic := rabbitpubsub.OpenTopic(conn, topicID)
	return topic, cleanup, nil
}
