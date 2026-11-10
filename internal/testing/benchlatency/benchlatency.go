// Copyright 2026 The Go Cloud Development Kit Authors
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

// Package benchlatency is a standalone, open-loop latency-distribution harness
// for Go CDK data-plane operations (blob/pubsub/docstore calls that hit a live
// cloud backend). It exists to capture p50/p90/p99/max tail latency -- the
// equivalent of JMH's SampleTime mode -- without the coordinated omission that
// per-op timing inside a testing.B loop suffers from.
//
// Why this is NOT a testing.B benchmark:
//
// Timing every operation inside a `for pb.Next()` loop is a closed-loop
// measurement: the next request is not issued until the previous one returns.
// When the backend stalls, the loop stalls with it and simply fails to record
// the requests that *would* have queued during the stall. This is "coordinated
// omission" (Gil Tene): it systematically under-reports tail latency exactly
// when the system is unhealthy, producing optimistic p99/p999 numbers. See the
// package README for the full rationale.
//
// This harness instead uses OPEN-LOOP dispatch: operation i is scheduled at
// start + i/Rate on the wall clock, regardless of whether earlier operations
// have completed, and its latency is recorded as (completion time - INTENDED
// start time). If the dispatcher itself falls behind (GC pause, CPU-starved
// client), the late operations are still sent, and the delay is charged to
// them rather than silently dropped. Result.MaxLag reports how far behind the
// dispatcher fell, so a run where the client was the bottleneck is visible.
//
// Recording keeps raw samples and computes exact nearest-rank percentiles with
// only the standard library, which suits the minute-scale runs this harness
// targets. Memory is O(samples); hour-scale soak tests would want a
// fixed-footprint histogram (e.g. HdrHistogram) instead.
package benchlatency

import (
	"context"
	"fmt"
	"io"
	"math"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// Op is a single unit of work to measure -- e.g. one blob read, one pubsub
// send, one docstore Get. It must be safe to call concurrently. Returning an
// error records the sample as a failure (still timed, so a slow error still
// counts against the tail).
type Op func(ctx context.Context) error

// Config controls an open-loop run.
type Config struct {
	// Rate is the target dispatch rate in operations per second. This is the
	// defining knob of open-loop measurement: operations are launched on this
	// schedule regardless of whether prior operations have completed. Must be
	// > 0.
	Rate float64

	// Duration is the wall-clock length of the measured window. Must be > 0.
	Duration time.Duration

	// Warmup is run before the measured window (connection setup, TLS
	// handshakes, cache warm-up). Operations scheduled during warmup are
	// dispatched but not recorded. Zero disables warmup.
	Warmup time.Duration

	// MaxInFlight caps concurrent in-flight operations to bound resource use
	// against a runaway backend. Zero means unlimited. Skipped dispatches are
	// omitted from the distribution entirely, and they would have been the
	// slowest, so a nonzero Throttled count means the tail is understated and
	// Rate exceeds what the backend sustains.
	MaxInFlight int
}

// Result is the outcome of a run. Counts cover the measured window only.
type Result struct {
	Count     int64         // recorded samples
	Errors    int64         // recorded operations that returned an error
	Throttled int64         // scheduled operations skipped due to MaxInFlight
	Rate      float64       // target rate that was requested
	Actual    float64       // recorded operations completed within Duration, per second
	MaxLag    time.Duration // worst delay between an op's intended and actual dispatch

	sorted []time.Duration
}

// P returns the latency at the given percentile (0..100), e.g. P(99), using
// the nearest-rank method. It returns 0 if nothing was recorded.
func (r *Result) P(pct float64) time.Duration {
	n := len(r.sorted)
	if n == 0 {
		return 0
	}
	i := int(math.Ceil(pct*float64(n)/100)) - 1
	return r.sorted[min(max(i, 0), n-1)]
}

// Max returns the maximum recorded latency.
func (r *Result) Max() time.Duration { return r.P(100) }

// Report writes a human-readable summary (fixed percentile ladder) to w.
func (r *Result) Report(w io.Writer, name string) {
	_, _ = fmt.Fprintf(w, "%s: n=%d errors=%d throttled=%d lag=%s  actual=%.0f/s (target=%.0f/s)\n",
		name, r.Count, r.Errors, r.Throttled, r.MaxLag.Round(time.Microsecond), r.Actual, r.Rate)
	for _, pct := range []float64{50, 90, 95, 99, 99.9} {
		_, _ = fmt.Fprintf(w, "  p%-4s %10s\n", trimPct(pct), r.P(pct).Round(time.Microsecond))
	}
	_, _ = fmt.Fprintf(w, "  max   %10s\n", r.Max().Round(time.Microsecond))
}

// Run executes op in open loop under cfg and returns the latency distribution.
// If ctx is canceled before Run finishes, it waits for in-flight operations and
// returns ctx.Err().
func Run(ctx context.Context, cfg Config, op Op) (*Result, error) {
	if !(cfg.Rate > 0) || math.IsInf(cfg.Rate, 0) {
		return nil, fmt.Errorf("benchlatency: Rate must be finite and > 0, got %v", cfg.Rate)
	}
	if cfg.Duration <= 0 {
		return nil, fmt.Errorf("benchlatency: Duration must be > 0, got %v", cfg.Duration)
	}
	if cfg.Warmup < 0 {
		return nil, fmt.Errorf("benchlatency: Warmup must be >= 0, got %v", cfg.Warmup)
	}
	if cfg.MaxInFlight < 0 {
		return nil, fmt.Errorf("benchlatency: MaxInFlight must be >= 0, got %v", cfg.MaxInFlight)
	}

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		samples   []time.Duration
		errs      atomic.Int64
		throttled atomic.Int64
		inflight  atomic.Int64
		onTime    int64
		maxLag    time.Duration
	)

	start := time.Now()
	warmupEnd := start.Add(cfg.Warmup)
	end := warmupEnd.Add(cfg.Duration)
	total := float64(cfg.Warmup + cfg.Duration)
	interval := float64(time.Second) / cfg.Rate

	// The schedule is computed, not ticked: a time.Ticker drops ticks for a
	// slow receiver, which would silently skip exactly the operations a stalled
	// client should have sent.
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	for i := 0; ; i++ {
		// Compare in float so a tiny Rate can't overflow time.Duration.
		off := float64(i) * interval
		if off >= total {
			break
		}
		intended := start.Add(time.Duration(off))
		if d := time.Until(intended); d > 0 {
			timer.Reset(d)
			select {
			case <-ctx.Done():
			case <-timer.C:
			}
		}
		if ctx.Err() != nil {
			wg.Wait()
			return nil, ctx.Err()
		}
		measured := !intended.Before(warmupEnd)
		if measured {
			maxLag = max(maxLag, time.Since(intended))
		}
		if cfg.MaxInFlight > 0 && inflight.Load() >= int64(cfg.MaxInFlight) {
			if measured {
				throttled.Add(1)
			}
			continue
		}
		inflight.Add(1)
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer inflight.Add(-1)
			err := op(ctx)
			// Latency is measured from the INTENDED start, folding in any
			// wait time between schedule and completion.
			done := time.Now()
			lat := done.Sub(intended)
			if !measured {
				return
			}
			if err != nil {
				errs.Add(1)
			}
			mu.Lock()
			samples = append(samples, lat)
			if !done.After(end) {
				onTime++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	slices.Sort(samples)
	return &Result{
		Count:     int64(len(samples)),
		Errors:    errs.Load(),
		Throttled: throttled.Load(),
		Rate:      cfg.Rate,
		Actual:    float64(onTime) / cfg.Duration.Seconds(),
		MaxLag:    maxLag,
		sorted:    samples,
	}, nil
}

func trimPct(p float64) string {
	if p == math.Trunc(p) {
		return fmt.Sprintf("%d", int64(p))
	}
	return fmt.Sprintf("%.1f", p)
}
