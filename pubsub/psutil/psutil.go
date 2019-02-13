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

// Package psutil provides utilities that work with Go CDK pubsub.
package psutil

import (
	"context"
	"fmt"
	"gocloud.dev/internal/workerpool"
	"gocloud.dev/pubsub"
)

// ReceiveConcurrently repeatedly fetches messages from the given subscription
// sub, calling handleMessage on each message with up to max concurrent calls.
// If handleMessage returns nil for its error, then the message is acked. If
// sub.Receive or handleMessage returns a non-nil error then the processing is
// terminated via context cancellation. Cancelling the context causes this
// function to return.
func ReceiveConcurrently(ctx context.Context, sub *pubsub.Subscription, max int, handleMessage func(ctx context.Context, m *pubsub.Message) error) error {
	return workerpool.Run(ctx, max, func(ctx context.Context) (workerpool.Task, error) {
		m, err := sub.Receive(ctx)
		if err != nil {
			return nil, fmt.Errorf("receiving message: %v", err)
		}
		task := func(ctx context.Context) error {
			if err := handleMessage(ctx, m); err != nil {
				return fmt.Errorf("handling message: %v", err)
			}
			m.Ack()
			return nil
		}
		return task, nil
	})
}
