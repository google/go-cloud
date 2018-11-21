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
package pubsub_test

import (
	"context"
	"testing"

	"github.com/google/go-cloud/internal/pubsub"
	"github.com/google/go-cloud/internal/pubsub/driver"
)

// emptyDriverSub is an intentionally buggy subscription driver. Such drivers should
// wait until some messages are available and then return a non-empty batch. This
// driver mischeviously always returns an empty batch.
type emptyDriverSub struct{}

func (s *emptyDriverSub) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	return nil, nil
}

func (s *emptyDriverSub) SendAcks(ctx context.Context, ackIDs []driver.AckID) error {
	return nil
}

func (s *emptyDriverSub) Close() error {
	return nil
}

func (s *emptyDriverSub) IsRetryable(error) bool { return false }

func TestReceiveErrorIfEmptyBatchReturnedFromDriver(t *testing.T) {
	ctx := context.Background()
	ds := &emptyDriverSub{}
	sub := pubsub.NewSubscription(ds)
	defer sub.Close()
	_, err := sub.Receive(ctx)
	if err == nil {
		t.Error("error expected for Receive with buggy driver")
	}
}
