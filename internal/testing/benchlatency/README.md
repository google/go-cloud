# benchlatency

An open-loop, tail-latency (p50/p90/p99/max) harness for Go CDK data-plane
operations. It is the Go equivalent of JMH's `SampleTime` mode, without the
coordinated omission that timing ops inside a `testing.B` loop suffers from.

## Why not just use `testing.B`?

`testing.B` reports **mean `ns/op`** — total elapsed time ÷ `b.N`. It has no
built-in notion of a latency distribution.

The tempting fix — time each op inside the `b.N` loop and build a histogram — is
a **closed-loop** measurement and is wrong for tail latency:

```go
for pb.Next() {
    start := time.Now()
    call()                  // <-- next iteration can't start until this returns
    hist.Record(time.Since(start))
}
```

When the backend stalls, the loop stalls with it and never issues the requests
that *would* have queued during the stall. Those missing samples are exactly the
slow ones. This is **coordinated omission** (Gil Tene): naive closed-loop timing
reports optimistic p99/p999 precisely when the system is unhealthy.

The canonical reference is Gil Tene's [*How NOT to Measure
Latency*](https://www.infoq.com/presentations/latency-response-time/);
[wrk2](https://github.com/giltene/wrk2) is the reference open-loop
implementation.

## What this does instead

Open-loop dispatch: operations are launched on a fixed wall-clock schedule
(`Config.Rate` ops/sec) **regardless of whether prior ops have returned**, and
latency is measured from each op's *intended* start time. That folds wait time
back into the distribution, so p99 reflects `Response Time = Wait + Service`.

## Usage

```go
res, err := benchlatency.Run(ctx, benchlatency.Config{
    Rate:     1000,             // target ops/sec (open loop)
    Duration: 60 * time.Second,
    Warmup:   5 * time.Second,
}, func(ctx context.Context) error {
    _, err := bucket.ReadAll(ctx, key)
    return err
})
res.Report(os.Stdout, "blob.ReadAll")
```

Example output (illustrative values):

```
blob.ReadAll: n=60000 errors=0 throttled=0 lag=212µs  actual=998/s (target=1000/s)
  p50        1.1ms
  p90        2.4ms
  p95        3.9ms
  p99       18.2ms
  p99.9     47.6ms
  max       61.0ms
```

Run it from a `TestXxxLatency` gated behind an env var (or a `main` inside this
module, since the package is internal), NOT under `go test -bench`. It hits live
cloud resources and takes minutes.

## Watch the diagnostics

- `actual` (operations completed within `Duration`, per second) far below
  `target` ⇒ the backend can't sustain `Rate`; the numbers describe an
  overloaded system.
- `throttled > 0` ⇒ `MaxInFlight` was hit and the skipped operations, which
  would have been the slowest, are missing from the distribution. Lower `Rate`
  or raise the cap.
- `errors > 0` ⇒ failed operations are included in the percentiles (a slow
  failure still counts against the tail, but a fast one can flatter it).
- `lag` large relative to p50 ⇒ the client, not the backend, fell behind its
  schedule (GC, CPU starvation). Late operations are still sent and the delay
  is charged to them, but the run is measuring the client too.

## Recording

Raw samples are kept and percentiles are exact nearest-rank, using only the
standard library. Memory is O(samples), which is fine for the minute-scale runs
this harness targets; hour-scale soak tests would want a fixed-footprint
histogram such as
[HdrHistogram](https://github.com/HdrHistogram/hdrhistogram-go) instead.
