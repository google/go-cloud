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

package benchlatency

import (
	"context"
	"errors"
	"math"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func noop(context.Context) error { return nil }

// TestRunLocal runs the harness against an in-process op with a known latency floor.
func TestRunLocal(t *testing.T) {
	const floor = 2 * time.Millisecond
	op := func(ctx context.Context) error {
		select {
		case <-time.After(floor):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	res, err := Run(context.Background(), Config{
		Rate:     200,
		Duration: 500 * time.Millisecond,
		Warmup:   100 * time.Millisecond,
	}, op)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Count != 100 {
		t.Errorf("Count = %d, want 100", res.Count)
	}
	if p50 := res.P(50); p50 < floor {
		t.Errorf("p50 = %v, want >= %v", p50, floor)
	}
	if res.P(99) < res.P(50) || res.Max() < res.P(99) {
		t.Errorf("percentiles not monotonic: p50=%v p99=%v max=%v", res.P(50), res.P(99), res.Max())
	}
	var sb strings.Builder
	res.Report(&sb, "local")
	for _, want := range []string{"local: n=100 ", "p99.9", "max"} {
		if !strings.Contains(sb.String(), want) {
			t.Errorf("Report missing %q:\n%s", want, sb.String())
		}
	}
}

func TestConfigValidation(t *testing.T) {
	for _, cfg := range []Config{
		{Rate: 0, Duration: time.Second},
		{Rate: -1, Duration: time.Second},
		{Rate: math.NaN(), Duration: time.Second},
		{Rate: math.Inf(1), Duration: time.Second},
		{Rate: 10, Duration: 0},
		{Rate: 10, Duration: time.Second, Warmup: -1},
		{Rate: 10, Duration: time.Second, MaxInFlight: -1},
	} {
		if _, err := Run(context.Background(), cfg, noop); err == nil {
			t.Errorf("Run(%+v) succeeded, want error", cfg)
		}
	}
}

// TestNoDroppedDispatches verifies that no scheduled operation is dropped when
// the dispatcher is starved of CPU.
func TestNoDroppedDispatches(t *testing.T) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	stop := make(chan struct{})
	var spinner sync.WaitGroup
	spinner.Add(1)
	go func() {
		defer spinner.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
		}
	}()
	res, err := Run(context.Background(), Config{Rate: 1000, Duration: 500 * time.Millisecond}, noop)
	close(stop)
	spinner.Wait()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Count != 500 {
		t.Errorf("Count = %d, want 500 (dispatches were dropped)", res.Count)
	}
	if res.MaxLag <= 0 {
		t.Errorf("MaxLag = %v, want it populated", res.MaxLag)
	}
	if res.Max() < res.MaxLag {
		t.Errorf("max latency %v < MaxLag %v", res.Max(), res.MaxLag)
	}
}

// TestWarmupExcluded verifies that operations scheduled during warmup are not recorded.
func TestWarmupExcluded(t *testing.T) {
	slow := func(context.Context) error { time.Sleep(50 * time.Millisecond); return nil }
	res, err := Run(context.Background(), Config{
		Rate:     100,
		Duration: 300 * time.Millisecond,
		Warmup:   200 * time.Millisecond,
	}, slow)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Count != 30 {
		t.Errorf("Count = %d, want 30", res.Count)
	}
}

func TestErrorsCounted(t *testing.T) {
	var n atomic.Int64
	op := func(context.Context) error {
		if n.Add(1)%2 == 0 {
			return errors.New("fail")
		}
		return nil
	}
	res, err := Run(context.Background(), Config{Rate: 1000, Duration: 100 * time.Millisecond}, op)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Count != 100 || res.Errors != 50 {
		t.Errorf("Count, Errors = %d, %d; want 100, 50", res.Count, res.Errors)
	}
}

func TestMaxInFlightThrottles(t *testing.T) {
	slow := func(context.Context) error { time.Sleep(50 * time.Millisecond); return nil }
	res, err := Run(context.Background(), Config{Rate: 100, Duration: 200 * time.Millisecond, MaxInFlight: 1}, slow)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Throttled == 0 {
		t.Error("Throttled = 0, want > 0")
	}
	if got := res.Count + res.Throttled; got != 20 {
		t.Errorf("Count+Throttled = %d, want 20 scheduled", got)
	}
}

// TestContextCanceled verifies that cancellation interrupts the wait for the
// next dispatch and that Run still waits for in-flight operations.
func TestContextCanceled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	var finished atomic.Bool
	op := func(ctx context.Context) error {
		<-ctx.Done()
		time.Sleep(20 * time.Millisecond)
		finished.Store(true)
		return ctx.Err()
	}
	begin := time.Now()
	// The second dispatch is 10s away, so a quick return means the wait was interrupted.
	_, err := Run(ctx, Config{Rate: 0.1, Duration: time.Minute}, op)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want %v", err, context.DeadlineExceeded)
	}
	if d := time.Since(begin); d > 5*time.Second {
		t.Errorf("Run took %v after cancellation", d)
	}
	if !finished.Load() {
		t.Error("Run returned before the in-flight operation finished")
	}
}

// TestCanceledWhileDraining verifies that canceling while operations drain returns ctx.Err().
func TestCanceledWhileDraining(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	op := func(ctx context.Context) error {
		time.Sleep(100 * time.Millisecond)
		cancel()
		return nil
	}
	if _, err := Run(ctx, Config{Rate: 100, Duration: 50 * time.Millisecond}, op); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want %v", err, context.Canceled)
	}
}

func TestActualCountsOnTimeCompletions(t *testing.T) {
	// Every op takes longer than the whole window, so none complete within it.
	slow := func(context.Context) error { time.Sleep(200 * time.Millisecond); return nil }
	res, err := Run(context.Background(), Config{Rate: 100, Duration: 100 * time.Millisecond}, slow)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Count != 10 || res.Actual != 0 {
		t.Errorf("Count, Actual = %d, %v; want 10, 0", res.Count, res.Actual)
	}
	res, err = Run(context.Background(), Config{Rate: 100, Duration: 500 * time.Millisecond}, noop)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Actual < 50 || res.Actual > 100 {
		t.Errorf("Actual = %v, want 50 to 100", res.Actual)
	}
}

func TestTinyRate(t *testing.T) {
	// interval overflows time.Duration; only the op at t=0 fits in the window.
	res, err := Run(context.Background(), Config{Rate: 1e-12, Duration: 10 * time.Millisecond}, noop)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Count != 1 {
		t.Errorf("Count = %d, want 1", res.Count)
	}
}

// TestPercentiles verifies the nearest-rank definition.
func TestPercentiles(t *testing.T) {
	ms := func(n int) []time.Duration {
		s := make([]time.Duration, n)
		for i := range s {
			s[i] = time.Duration(i+1) * time.Millisecond
		}
		return s
	}
	r100 := &Result{sorted: ms(100)}
	r1000 := &Result{sorted: ms(1000)}
	for _, tc := range []struct {
		r    *Result
		pct  float64
		want time.Duration
	}{
		{r100, 0, 1 * time.Millisecond},
		{r100, 50, 50 * time.Millisecond},
		{r100, 90, 90 * time.Millisecond},
		{r100, 99, 99 * time.Millisecond},
		{r100, 100, 100 * time.Millisecond},
		{r1000, 99.9, 999 * time.Millisecond},
		{r1000, 100, 1000 * time.Millisecond},
		{&Result{}, 99, 0},
	} {
		if got := tc.r.P(tc.pct); got != tc.want {
			t.Errorf("n=%d P(%v) = %v, want %v", len(tc.r.sorted), tc.pct, got, tc.want)
		}
	}
}

// TestCoordinatedOmissionCorrection verifies that a stall shared by all
// requests shows up in the tail.
func TestCoordinatedOmissionCorrection(t *testing.T) {
	const freeze = 100 * time.Millisecond
	var mu sync.Mutex
	nextFreeze := time.Now().Add(200 * time.Millisecond)
	op := func(context.Context) error {
		mu.Lock()
		if !time.Now().Before(nextFreeze) {
			time.Sleep(freeze)
			nextFreeze = time.Now().Add(300 * time.Millisecond)
		}
		mu.Unlock()
		time.Sleep(500 * time.Microsecond)
		return nil
	}
	res, err := Run(context.Background(), Config{Rate: 500, Duration: time.Second}, op)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// About a quarter of requests arrive during a freeze, so p90 must show it.
	if p90 := res.P(90); p90 < 10*time.Millisecond {
		t.Errorf("p90 = %v; want >= 10ms to reflect the shared stall", p90)
	}
}
