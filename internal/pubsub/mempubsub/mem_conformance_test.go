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

package mempubsub_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cloud/internal/pubsub"
	"github.com/google/go-cloud/internal/pubsub/drivertest"
	"github.com/google/go-cloud/internal/pubsub/mempubsub"
)

type harness struct{}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	return &harness{}, nil
}

func (h *harness) MakePair(ctx context.Context) (*pubsub.Topic, *pubsub.Subscription, error) {
	dt := mempubsub.OpenTopic()
	ds := mempubsub.OpenSubscription(t, time.Second)
	t := pubsub.NewTopic(ctx, t)
	s := pubsub.NewSubscription(ctx, s)
	return t, s, nil
}

func (h *harness) Close() {
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness)
}
