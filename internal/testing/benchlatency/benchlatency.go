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

// Package benchlatency measures the latency distribution of an operation sent
// at a fixed rate, timing each call from its intended start.
package benchlatency

import (
	"context"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Op is one operation to measure, such as a blob read. It must be safe for
// concurrent use and should return promptly once ctx is done.
type Op func(ctx context.Context) error

// Config controls a Run.
type Config struct {
	// Rate is the target dispatch rate in operations per second. Must be > 0.
	Rate float64

	// Duration is the length of the measured window. Must be > 0.
	Duration time.Duration

	// Warmup precedes the measured window; its operations are sent but not recorded.
	Warmup time.Duration

	// MaxInFlight caps concurrent operations; 0 means no limit. Operations
	// skipped at the cap are not recorded.
	MaxInFlight int
}

// Result is the outcome of a Run. Counts cover the measured window only.
type Result struct {
	Count     int64         // recorded operations
	Errors    int64         // recorded operations that failed
	Throttled int64         // operations skipped due to MaxInFlight
	Rate      float64       // requested rate
	Actual    float64       // recorded operations completed within Duration, per second
	MaxLag    time.Duration // worst delay between an operation's intended and actual dispatch

	sorted []time.Duration
}

// P returns the nearest-rank latency at percentile pct (0 to 100), or 0 if
// nothing was recorded.
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

// Report writes a summary of r to w, labeled with name.
func (r *Result) Report(w io.Writer, name string) {
	_, _ = fmt.Fprintf(w, "%s: n=%d errors=%d throttled=%d lag=%s  actual=%.0f/s (target=%.0f/s)\n",
		name, r.Count, r.Errors, r.Throttled, r.MaxLag.Round(time.Microsecond), r.Actual, r.Rate)
	for _, pct := range []float64{50, 90, 95, 99, 99.9} {
		_, _ = fmt.Fprintf(w, "  p%-4s %10s\n", strconv.FormatFloat(pct, 'f', -1, 64), r.P(pct).Round(time.Microsecond))
	}
	_, _ = fmt.Fprintf(w, "  max   %10s\n", r.Max().Round(time.Microsecond))
}

// Run calls op at cfg.Rate and returns the latency distribution. If ctx is
// canceled, Run waits for in-flight operations and returns ctx.Err().
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
		errs      int64
		throttled int64
		inflight  atomic.Int64
		onTime    int64
		maxLag    time.Duration
	)

	start := time.Now()
	warmupEnd := start.Add(cfg.Warmup)
	end := warmupEnd.Add(cfg.Duration)
	total := float64(cfg.Warmup + cfg.Duration)
	interval := float64(time.Second) / cfg.Rate

	// Compute the schedule; a time.Ticker would drop ticks when the loop falls behind.
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
				throttled++
			}
			continue
		}
		inflight.Add(1)
		wg.Go(func() {
			defer inflight.Add(-1)
			err := op(ctx)
			// Measure from the intended start so time spent waiting counts.
			done := time.Now()
			lat := done.Sub(intended)
			if !measured {
				return
			}
			mu.Lock()
			if err != nil {
				errs++
			}
			samples = append(samples, lat)
			if !done.After(end) {
				onTime++
			}
			mu.Unlock()
		})
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	slices.Sort(samples)
	return &Result{
		Count:     int64(len(samples)),
		Errors:    errs,
		Throttled: throttled,
		Rate:      cfg.Rate,
		Actual:    float64(onTime) / cfg.Duration.Seconds(),
		MaxLag:    maxLag,
		sorted:    samples,
	}, nil
}
