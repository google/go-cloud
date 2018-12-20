// Copyright 2018 The Go Cloud Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"github.com/streadway/amqp"
	"gocloud.dev/internal/pubsub"
	"gocloud.dev/internal/pubsub/rabbitpubsub"
)

// openRabbitTopic returns the RabbitMQ topic for the given topic ID.
func openRabbitTopic(topicID, serverURL string) (*pubsub.Topic, func(), error) {
	conn, err := amqp.Dial(serverURL)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { conn.Close() }
	topic := rabbitpubsub.OpenTopic(conn, topicID)
	return topic, cleanup, nil
}

// openRabbitTopic returns the RabbitMQ topic for the given topic ID.
func openRabbitSubscription(subID, serverURL string) (*pubsub.Subscription, func(), error) {
	conn, err := amqp.Dial(serverURL)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { conn.Close() }
	sub := rabbitpubsub.OpenSubscription(conn, subID)
	return sub, cleanup, nil
}
