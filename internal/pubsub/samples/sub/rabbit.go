package main

import (
	"github.com/streadway/amqp"
	"gocloud.dev/internal/pubsub"
	"gocloud.dev/internal/pubsub/rabbitpubsub"
)

const rabbitURL = "amqp://guest:guest@localhost:5672/"

// makeRabbitSubscription creates a subscription with the given ID within a
// local rabbit server.
func makeRabbitSubscription(topicID, subID string) error {
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
	_, err = ch.QueueDeclare(subID,
		false, // durable
		false, // delete when unused
		false, // exclusive
		false, // wait
		nil)   // args
	if err != nil {
		return err
	}
	return ch.QueueBind(subID, subID, topicID, false, nil)
}

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

// deleteRabbitSubscription deletes a subscription from the rabbit server.
func deleteRabbitSubscription(subID string) error {
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
	_, err = ch.QueueDelete(subID,
		false, // ifUnused
		false, // ifEmpty
		false) // noWait
	return err
}
