package main

import (
	"github.com/streadway/amqp"
	"gocloud.dev/internal/pubsub"
	"gocloud.dev/internal/pubsub/rabbitpubsub"
)

const rabbitURL = "amqp://guest:guest@localhost:5672/"

// makeRabbitTopic creates a topic with the given ID within a local rabbit
// server.
func makeRabbitTopic(topicID string) error {
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		return err
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	return ch.ExchangeDeclare(topicID,
		"fanout", // kind
		false,    // durable
		false,    // delete when unused
		false,    // internal
		false,    // wait
		nil)      // args
}

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

func deleteRabbitTopic(topicID string) error {
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		return err
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	return ch.ExchangeDelete(topicID,
		false, // ifUnused
		false) // noWait
}
