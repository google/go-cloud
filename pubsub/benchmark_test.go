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

	"gocloud.dev/pubsub/driver"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
)

const (
	// How long to run the test.
	runFor = 25 * time.Second
	// How long the "warmup period" is, during which we report more frequently.
	reportWarmup = 500 * time.Millisecond
	// Minimum frequency for reporting throughput, during warmup and after that.
	reportPeriodWarmup = 50 * time.Millisecond
	reportPeriod       = 1 * time.Second
	// Number of output lines per test. We set this to a constant so that it's
	// easy to copy/paste the output into a Google Sheet with pre-created graphs.
	// Should be above runFor / reportPeriod + reportWarmup / reportPeriodWarmup.
	numLinesPerTest = 50
	// Number of data points to smooth msgs/sec and RPCs/sec over.
	smoothing = 5
)

type fakeSub struct {
	driver.Subscription
	start   time.Time
	profile func(bool, int) (int, time.Duration)
	msgs    []*driver.Message
}

func (s *fakeSub) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	n, delay := s.profile(s.inMiddleThird(), maxMessages)
	if delay > 0 {
		time.Sleep(delay)
	}
	return s.msgs[:n], nil
}

// inMiddleThird returns true if this test is in the middle third of the running
// time; used for burstiness tests.
func (s *fakeSub) inMiddleThird() bool {
	elapsed := time.Since(s.start)
	return elapsed > runFor/3 && elapsed < runFor*2/3
}

// TestReceivePerformance enables characterization of Receive under various
// situations, characterized in "tests" below.
func TestReceivePerformance(t *testing.T) {
	t.Skip("Skipped by default")

	const defaultNumGoRoutines = 100
	defaultReceiveProfile := func(_ bool, maxMessages int) (int, time.Duration) { return maxMessages, 0 }
	defaultProcessProfile := func(bool) time.Duration { return 0 }

	tests := []struct {
		description string
		// See the defaults above.
		numGoRoutines  int
		receiveProfile func(bool, int) (int, time.Duration)
		processProfile func(bool) time.Duration
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
			receiveProfile: func(_ bool, maxMessages int) (int, time.Duration) { return maxMessages, 100 * time.Millisecond },
		},
		{
			description:    "receive 1s",
			receiveProfile: func(_ bool, maxMessages int) (int, time.Duration) { return maxMessages, 1 * time.Second },
		},
		{
			description:    "process 100ms",
			processProfile: func(bool) time.Duration { return 100 * time.Millisecond },
		},
		{
			description:    "process 1s",
			processProfile: func(bool) time.Duration { return 1 * time.Second },
		},
		{
			description: "receive 1s process 70ms",
			receiveProfile: func(_ bool, maxMessages int) (int, time.Duration) {
				return maxMessages, 1 * time.Second
			},
			processProfile: func(bool) time.Duration { return 70 * time.Millisecond },
		},
		{
			description: "receive 250ms+stddev 150ms, process 10ms + stddev 5ms",
			receiveProfile: func(_ bool, maxMessages int) (int, time.Duration) {
				return maxMessages, time.Duration(rand.NormFloat64()*150+250) * time.Millisecond
			},
			processProfile: func(bool) time.Duration { return time.Duration(rand.NormFloat64()*5+10) * time.Millisecond },
		},
		{
			description: "bursty message arrival",
			receiveProfile: func(inMiddleThird bool, maxMessages int) (int, time.Duration) {
				// When in the middle third of the running time, return 0 messages.
				n := maxMessages
				if inMiddleThird {
					n = 0
				}
				return n, time.Duration(rand.NormFloat64()*25+100) * time.Millisecond
			},
			processProfile: func(bool) time.Duration { return time.Duration(rand.NormFloat64()*5+10) * time.Millisecond },
		},
		{
			description: "bursty receive time",
			receiveProfile: func(inMiddleThird bool, maxMessages int) (int, time.Duration) {
				// When in the middle third of the running time, 10x the RPC time.
				d := time.Duration(rand.NormFloat64()*25+100) * time.Millisecond
				if inMiddleThird {
					d *= 10
				}
				return maxMessages, d
			},
			processProfile: func(bool) time.Duration { return time.Duration(rand.NormFloat64()*5+10) * time.Millisecond },
		},
		{
			description: "bursty process time",
			receiveProfile: func(_ bool, maxMessages int) (int, time.Duration) {
				return maxMessages, time.Duration(rand.NormFloat64()*25+100) * time.Millisecond
			},
			processProfile: func(inMiddleThird bool) time.Duration {
				d := time.Duration(rand.NormFloat64()*5+10) * time.Millisecond
				if inMiddleThird {
					d *= 100
				}
				return d
			},
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

func runBenchmark(t *testing.T, description string, numGoRoutines int, receiveProfile func(bool, int) (int, time.Duration), processProfile func(bool) time.Duration, noAck bool) {
	msgs := make([]*driver.Message, maxBatchSize)
	for i := range msgs {
		msgs[i] = &driver.Message{}
	}

	fake := &fakeSub{msgs: msgs, profile: receiveProfile, start: time.Now()}
	sub := newSubscription(fake, nil, nil)
	defer sub.Shutdown(context.Background())

	// Header row.
	fmt.Printf("%s\tmsgs/sec\tRPCs/sec\tbatchsize\n", description)

	var mu sync.Mutex
	start := time.Now()
	var lastReport time.Time
	numMsgs := 0
	var prevMsgsPerSec, prevRPCsPerSec []float64     // last <smoothing> datapoints
	var runningMsgsPerSec, runningRPCsPerSec float64 // sum of values in above slices
	numRPCs := 0
	lastMaxMessages := 0
	nLines := 1 // header

	// mu must be locked when called.
	reportLine := func(now time.Time) {
		elapsed := now.Sub(start)
		elapsedSinceReport := now.Sub(lastReport)

		// Smooth msgsPerSec over the last <smoothing> datapoints.
		msgsPerSec := float64(numMsgs) / elapsedSinceReport.Seconds()
		prevMsgsPerSec = append(prevMsgsPerSec, msgsPerSec)
		runningMsgsPerSec += msgsPerSec
		if len(prevMsgsPerSec) > smoothing {
			runningMsgsPerSec -= prevMsgsPerSec[0]
			if runningMsgsPerSec < 0 {
				runningMsgsPerSec = 0
			}
			prevMsgsPerSec = prevMsgsPerSec[1:]
		}

		// Smooth rpcsPerSec over the last <smoothing> datapoints.
		rpcsPerSec := float64(numRPCs) / elapsedSinceReport.Seconds()
		prevRPCsPerSec = append(prevRPCsPerSec, rpcsPerSec)
		runningRPCsPerSec += rpcsPerSec
		if len(prevRPCsPerSec) > smoothing {
			runningRPCsPerSec -= prevRPCsPerSec[0]
			if runningRPCsPerSec < 0 {
				runningRPCsPerSec = 0
			}
			prevRPCsPerSec = prevRPCsPerSec[1:]
		}

		fmt.Printf("%f\t%f\t%f\t%d\n", elapsed.Seconds(), runningMsgsPerSec/float64(len(prevMsgsPerSec)), runningRPCsPerSec/float64(len(prevRPCsPerSec)), lastMaxMessages)
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

	ctx, cancel := context.WithTimeout(context.Background(), runFor)
	defer cancel()
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
				if xerrors.Is(err, context.DeadlineExceeded) {
					return nil
				}
				if err != nil {
					return err
				}
				mu.Lock()
				numMsgs++
				mu.Unlock()
				delay := processProfile(fake.inMiddleThird())
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
