// Copyright 2019 The Go Cloud Development Kit Authors
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

package mqttpubsub_test

import (
	"context"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/filinvadim/go-cloud/pubsub/mqttpubsub"
	"gocloud.dev/pubsub"
	"log"
)

func ExampleOpenTopic() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	opts := mqtt.NewClientOptions()
	opts = opts.AddBroker("mqtt://mqtt.example.com")
	opts.ClientID = "exampleClient"
	cli := mqtt.NewClient(opts)
	token := cli.Connect()
	if token.Wait() && token.Error() != nil {
		log.Fatal(token.Error())
	}

	defer cli.Disconnect(0)

	topic, err := mqttpubsub.OpenTopic(mqttpubsub.NewPublisher(cli), "example.mysubject", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer topic.Shutdown(ctx)
}

func ExampleOpenSubscription() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	opts := mqtt.NewClientOptions()
	opts = opts.AddBroker("mqtt://mqtt.example.com")
	opts.ClientID = "exampleClient"
	cli := mqtt.NewClient(opts)
	token := cli.Connect()
	if token.Wait() && token.Error() != nil {
		log.Fatal(token.Error())
	}

	defer cli.Disconnect(0)

	subscription, err := mqttpubsub.OpenSubscription(
		mqttpubsub.NewSubscriber(cli),
		"example.mysubject",
		nil)
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)
}

func Example_openTopicFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/pubsub/mqttpubsub"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// pubsub.OpenTopic creates a *pubsub.Topic from a URL.
	// This URL will Dial the MQTT server at the URL in the environment variable
	// MQTT_SERVER_URL and send messages with subject "example.mysubject".
	topic, err := pubsub.OpenTopic(ctx, "mqtt://example.mysubject")
	if err != nil {
		log.Fatal(err)
	}
	defer topic.Shutdown(ctx)
}

func Example_openSubscriptionFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/pubsub/mqttpubsub"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// pubsub.OpenSubscription creates a *pubsub.Subscription from a URL.
	// This URL will Dial the MQTT server at the URL in the environment variable
	// MQTT_SERVER_URL and receive messages with subject "example.mysubject".
	subscription, err := pubsub.OpenSubscription(ctx, "mqtt://example.mysubject")
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)
}
