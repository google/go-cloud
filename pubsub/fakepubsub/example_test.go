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

package fakepubsub_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/go-cloud/pubsub"
	"github.com/google/go-cloud/pubsub/fakepubsub"
)

func Example() {
	ctx := context.Background()
	dt := fakepubsub.OpenTopic()
	topic := pubsub.NewTopic(ctx, dt, nil)
	ds := fakepubsub.OpenSubscription(dt, 3*time.Second)
	sub := pubsub.NewSubscription(ctx, ds, nil)

	body := []byte("hello, world")
	if err := topic.Send(ctx, &pubsub.Message{Body: body}); err != nil {
		log.Fatal(err)
	}
	msg, err := sub.Receive(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(msg.Body))
	if err := msg.Ack(ctx); err != nil {
		log.Fatal(err)
	}
	// Output: hello, world
}
