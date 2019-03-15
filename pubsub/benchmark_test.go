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

package pubsub

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gocloud.dev/internal/retry"
	"gocloud.dev/pubsub/driver"
	"golang.org/x/sync/errgroup"
)

const (
	// Number of goroutines to use for Receive.
	nGoRoutines = 100
	// How long to run the test.
	runFor = 10 * time.Second
	// Minimum frequency for reporting throughput.
	reportPeriod = 1 * time.Second
)

var (
	// receiveBatchProfile should return the number of message for ReceiveBatch
	// to return (<= maxMessages), and the simulated time it took for ReceiveBatch
	// to run.
	receiveBatchProfile = func(maxMessages int) (int, time.Duration) {
		return maxMessages, 0
	}
	// processMessageProfile should return whether Message.Ack should be called,
	// and the simulated time it took to process the message.
	processMessageProfile = func() (bool, time.Duration) {
		return true, 0
	}
)

type fakeSub struct {
	driver.Subscription
	msgs []*driver.Message
}

func (s *fakeSub) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	n, delay := receiveBatchProfile(maxMessages)
	if delay > 0 {
		time.Sleep(delay)
	}
	return s.msgs[:n], nil
}

func (s *fakeSub) AckFunc() func() { return func() {} }

// TestReceivePerformance enables characterization of Receive under various
// situations. Tune the const and var parameters above, disable the Skip,
// and the test will output a series of timepoints with throughput and the
// batch size being sent to ReceiveBatch.
func TestReceivePerformance(t *testing.T) {
	t.Skip("Skipping by default")

	msgs := make([]*driver.Message, 1000)
	for i := 0; i < 1000; i++ {
		msgs[i] = &driver.Message{}
	}
	sub := newSubscription(&fakeSub{msgs: msgs}, nil)

	// Header row.
	fmt.Println("Elapsed (sec)\tMsgs/sec\tMax Messages")

	// Configure our output in a hook called whenever ReceiveBatch returns.
	start := time.Now()
	lastReport := start
	numMsgs := 0
	lastMaxMessages := 0
	sub.onReceiveBatchHook = func(numMessages, maxMessages int) {
		numMsgs += numMessages

		// Emit a timepoint if maxMessages has changed, or if reportPeriod has
		// elapsed since our last timepoint.
		now := time.Now()
		elapsed := now.Sub(lastReport)
		if lastMaxMessages != maxMessages || elapsed > reportPeriod {
			secsSinceStart := now.Sub(start).Seconds()
			msgsPerSec := float64(numMsgs) / elapsed.Seconds()
			fmt.Printf("%f\t%f\t%d\n", secsSinceStart, msgsPerSec, maxMessages)

			lastReport = now
			numMsgs = 0
			lastMaxMessages = maxMessages
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(runFor, cancel)
	var grp errgroup.Group
	for i := 0; i < nGoRoutines; i++ {
		grp.Go(func() error {
			// Each goroutine loops until ctx is canceled.
			for {
				m, err := sub.Receive(ctx)
				if err == context.Canceled {
					return nil
				}
				// TODO(rvangent): This is annoying. Maybe retry should return
				// context.Canceled instead of wrapping if it never retried?
				if cerr, ok := err.(*retry.ContextError); ok && cerr.CtxErr == context.Canceled {
					return nil
				}
				if err != nil {
					return err
				}
				callAck, delay := processMessageProfile()
				if delay > 0 {
					time.Sleep(delay)
				}
				if callAck {
					m.Ack()
				}
			}
		})
	}
	if err := grp.Wait(); err != nil {
		t.Error(err)
	}
}
