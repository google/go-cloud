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

package mempubsub

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cloud/internal/pubsub/driver"
	"github.com/google/go-cloud/internal/pubsub/drivertest"
)

type harness struct {
	b *Broker
}

func (h *harness) MakeTopic(context.Context) (driver.Topic, error) {
	return h.b.topic("t"), nil
}

func (h *harness) MakeSubscription(_ context.Context, dt driver.Topic) (driver.Subscription, error) {
	return newSubscription(dt.(*topic), time.Second), nil
}

func (h *harness) Close() {}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, func(context.Context, *testing.T) (drivertest.Harness, error) {
		return &harness{b: NewBroker([]string{"t"})}, nil
	})
}
