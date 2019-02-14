// Copyright 2018 The Go Cloud Development Kit Authors
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

// workerpool is a sample application that simulates messages being
// published about video uploads and subscribed workers transcoding
// these videos. In a real application the publish and subscribe
// sides would normally be in separate executables.
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/mempubsub"
	"gocloud.dev/pubsub/psutil"
	"golang.org/x/sync/errgroup"
)

func main() {
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)

	// top is a topic that notifies when a video has been uploaded.
	// sub is a subscription to this topic.
	top := mempubsub.NewTopic()
	sub := mempubsub.NewSubscription(top, time.Hour)
	defer top.Shutdown(ctx)
	defer sub.Shutdown(ctx)

	var eg errgroup.Group
	eg.Go(func() error { return publish(ctx, top) })
	eg.Go(func() error { return subscribe(ctx, sub) })
	if err := eg.Wait(); err != nil {
		log.Printf("workerpool sample: %v", err)
	}
}

// publish simulates users uploading videos.
func publish(ctx context.Context, top *pubsub.Topic) error {
	for {
		videoURL := fmt.Sprintf("acmestorage://videos/%d.mpeg", rand.Int())
		if err := top.Send(ctx, &pubsub.Message{Body: []byte(videoURL)}); err != nil {
			return fmt.Errorf("sending video upload message: %v", err)
		}
		// Simulate some delay between videos being uploaded on an up-and-coming video site.
		time.Sleep(time.Duration(rand.Intn(10)*100) * time.Millisecond)
	}
	return nil
}

// subscribe pulls from the subscription and simulates processing the videos.
func subscribe(ctx context.Context, sub *pubsub.Subscription) error {
	return psutil.ReceiveConcurrently(ctx, sub, 10, func(ctx context.Context, m *pubsub.Message) error {
		videoURL := string(m.Body)
		log.Printf("Processing %v", videoURL)
		// Simulate transcoding the video and storing it in various formats.
		time.Sleep(time.Duration(rand.Intn(10)) * time.Second)
		log.Printf("Done processing %v", videoURL)
		return nil // ack
	})
}
