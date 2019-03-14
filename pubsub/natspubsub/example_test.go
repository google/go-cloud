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

package natspubsub_test

import (
	"context"
	"log"

	"github.com/nats-io/go-nats"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/natspubsub"
)

func ExampleCreateTopic() {
	ctx := context.Background()

	// Create a connection to NATS
	// For use with NGS and credentials.
	// nc, err := nats.Connect("connect.ngs/global", nats.UserCredentials("path_to_creds_file")
	nc, err := nats.Connect("nats://demo.nats.io")
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	pt := natspubsub.CreateTopic(nc, "go-cloud.example.send")

	err = pt.Send(ctx, &pubsub.Message{Body: []byte("example message")})
}

func ExampleCreateSubscription() {
	ctx := context.Background()

	// Create a connection to NATS
	// For use with NGS and credentials.
	// nc, err := nats.Connect("connect.ngs/global", nats.UserCredentials("path_to_creds_file")
	nc, err := nats.Connect("nats://demo.nats.io")
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	sub := natspubsub.CreateSubscription(nc, "go-cloud.example.send")

	// Now we can use sub to receive messages.
	msg, err := sub.Receive(ctx)
	if err != nil {
		// Handle error....
	}
	// Handle Message

	// Ack is a no-op with NATS.
	msg.Ack()
}
