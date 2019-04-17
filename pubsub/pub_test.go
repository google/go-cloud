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

package pubsub_test

import (
	"context"
	"testing"
	"time"

	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
)

type funcTopic struct {
	driver.Topic
	sendBatch func(ctx context.Context, ms []*driver.Message) error
	closed    bool
}

func (t *funcTopic) SendBatch(ctx context.Context, ms []*driver.Message) error {
	return t.sendBatch(ctx, ms)
}

func (t *funcTopic) IsRetryable(error) bool { return false }
func (t *funcTopic) Close() error {
	t.closed = true
	return nil
}

func TestTopicShutdownCanBeCanceledEvenWithHangingSend(t *testing.T) {
	dt := &funcTopic{
		sendBatch: func(ctx context.Context, ms []*driver.Message) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}
	topic := pubsub.NewTopic(dt, nil)

	go func() {
		m := &pubsub.Message{}
		if err := topic.Send(context.Background(), m); err == nil {
			t.Fatal("nil err from Send, expected context cancellation error")
		}
	}()

	done := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	go func() {
		topic.Shutdown(ctx)
		close(done)
	}()

	// Now cancel the context being used by topic.Shutdown.
	cancel()

	// It shouldn't take too long before topic.Shutdown stops.
	tooLong := 5 * time.Second
	select {
	case <-done:
	case <-time.After(tooLong):
		t.Fatalf("waited too long(%v) for Shutdown(ctx) to run", tooLong)
	}
}

func TestTopicCloseIsCalled(t *testing.T) {
	ctx := context.Background()
	dt := &funcTopic{}
	topic := pubsub.NewTopic(dt, nil)
	topic.Shutdown(ctx)
	if !dt.closed {
		t.Error("want Topic.Close to have been called")
	}
}
