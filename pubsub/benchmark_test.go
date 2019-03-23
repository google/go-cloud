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
	"math/rand"
	"sync"
	"testing"
	"time"

	"gocloud.dev/internal/retry"
	"gocloud.dev/pubsub/driver"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
)

const (
	// How long to run the test.
	runFor = 10 * time.Second
	// How long the "warmup period" is, during which we report more frequently.
	reportWarmup = 500 * time.Millisecond
	// Minimum frequency for reporting throughput, during warmup and after that.
	reportPeriodWarmup = 25 * time.Millisecond
	reportPeriod       = 500 * time.Millisecond
	// Number of output lines per test. We set this to a constant so that it's
	// easy to copy/paste the output into a Google Sheet with pre-created graphs.
	// Should be above runFor / reportPeriod.
	numLinesPerTest = 50
)

type fakeSub struct {
	driver.Subscription
	profile func(int) (int, time.Duration)
	msgs    []*driver.Message
}

func (s *fakeSub) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	n, delay := s.profile(maxMessages)
	if delay > 0 {
		time.Sleep(delay)
	}
	return s.msgs[:n], nil
}

func (s *fakeSub) AckFunc() func() { return func() {} }

// TestReceivePerformance enables characterization of Receive under various
// situations, characterized in "tests" below.
func TestReceivePerformance(t *testing.T) {
	t.Skip("Skipped by default")

	const defaultNumGoRoutines = 100
	defaultReceiveProfile := func(maxMessages int) (int, time.Duration) { return maxMessages, 0 }
	defaultProcessProfile := func() time.Duration { return 0 }

	tests := []struct {
		description string
		// See the defaults above.
		numGoRoutines  int
		receiveProfile func(int) (int, time.Duration)
		processProfile func() time.Duration
		noAck          bool
	}{
		{
			description: "baseline",
		},
		{
			description:   "1 goroutine",
			numGoRoutines: 1,
		},
		{
			description: "no ack",
			noAck:       true,
		},
		{
			description:    "receive 100ms",
			receiveProfile: func(maxMessages int) (int, time.Duration) { return maxMessages, 100 * time.Millisecond },
		},
		{
			description:    "receive 1s",
			receiveProfile: func(maxMessages int) (int, time.Duration) { return maxMessages, 1 * time.Second },
		},
		{
			description:    "process 100ms",
			processProfile: func() time.Duration { return 100 * time.Millisecond },
		},
		{
			description:    "process 1s",
			processProfile: func() time.Duration { return 1 * time.Second },
		},
		{
			description: "receive 250ms+stddev 150ms, process 10ms + stddev 5ms",
			receiveProfile: func(maxMessages int) (int, time.Duration) {
				return maxMessages, time.Duration(rand.NormFloat64()*150+250) * time.Millisecond
			},
			processProfile: func() time.Duration { return time.Duration(rand.NormFloat64()*5+10) * time.Millisecond },
		},
	}

	for _, test := range tests {
		if test.numGoRoutines == 0 {
			test.numGoRoutines = defaultNumGoRoutines
		}
		if test.receiveProfile == nil {
			test.receiveProfile = defaultReceiveProfile
		}
		if test.processProfile == nil {
			test.processProfile = defaultProcessProfile
		}
		t.Run(test.description, func(t *testing.T) {
			runBenchmark(t, test.description, test.numGoRoutines, test.receiveProfile, test.processProfile, test.noAck)
		})
	}
}

func runBenchmark(t *testing.T, description string, numGoRoutines int, receiveProfile func(int) (int, time.Duration), processProfile func() time.Duration, noAck bool) {
	msgs := make([]*driver.Message, 1000)
	for i := range msgs {
		msgs[i] = &driver.Message{}
	}

	fake := &fakeSub{msgs: msgs, profile: receiveProfile}
	sub := newSubscription(fake, 0, nil)

	// Configure our output in a hook called whenever ReceiveBatch returns.

	// Header row.
	fmt.Printf("%s\tmsgs/sec\tRPCs/sec\tbatchsize\n", description)

	var mu sync.Mutex
	start := time.Now()
	var lastReport time.Time
	numMsgs := 0
	numRPCs := 0
	lastMaxMessages := 0
	nLines := 1 // header

	// mu must be locked when called.
	reportLine := func(now time.Time) {
		elapsed := now.Sub(start)
		elapsedSinceReport := now.Sub(lastReport)
		msgsPerSec := float64(numMsgs) / elapsedSinceReport.Seconds()
		rpcsPerSec := float64(numRPCs) / elapsedSinceReport.Seconds()
		fmt.Printf("%f\t%f\t%f\t%d\n", elapsed.Seconds(), msgsPerSec, rpcsPerSec, lastMaxMessages)
		nLines++

		lastReport = now
		numMsgs = 0
		numRPCs = 0
	}

	sub.preReceiveBatchHook = func(maxMessages int) {
		mu.Lock()
		defer mu.Unlock()
		lastMaxMessages = maxMessages
		numRPCs++
		if lastReport.IsZero() {
			reportLine(time.Now())
		}
	}
	sub.postReceiveBatchHook = func(numMessages int) {
		mu.Lock()
		defer mu.Unlock()
		numMsgs += numMessages
	}

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(runFor, cancel)
	done := make(chan struct{})
	go func() {
		period := reportPeriodWarmup
		for {
			select {
			case now := <-time.After(period):
				mu.Lock()
				reportLine(now)
				mu.Unlock()
				if now.Sub(start) > reportWarmup {
					period = reportPeriod
				}
			case <-ctx.Done():
				close(done)
				return
			}
		}
	}()

	var grp errgroup.Group
	for i := 0; i < numGoRoutines; i++ {
		grp.Go(func() error {
			// Each goroutine loops until ctx is canceled.
			for {
				m, err := sub.Receive(ctx)
				if xerrors.Is(err, context.Canceled) {
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
				delay := processProfile()
				if delay > 0 {
					time.Sleep(delay)
				}
				if !noAck {
					m.Ack()
				}
			}
		})
	}
	if err := grp.Wait(); err != nil {
		t.Errorf("%s: %v", description, err)
	}
	<-done
	if nLines > numLinesPerTest {
		t.Errorf("produced too many lines (%d)", nLines)
	}
	for n := nLines; n < numLinesPerTest; n++ {
		fmt.Println()
	}
}
