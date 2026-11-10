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

package benchlatency_test

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/memblob" // swap for gcsblob/s3blob to hit a live cloud
	"gocloud.dev/internal/testing/benchlatency"
)

// Example_blobRead shows how to characterize p50/p99/max latency of a blob read
// against a live cloud backend. It uses memblob here so the example is runnable
// in CI; for real tail-latency numbers, open a gs://, s3:// or azblob:// bucket
// instead and run it from an env-gated test (NOT under `go test -bench`).
func Example_blobRead() {
	ctx := context.Background()

	// In production: blob.OpenBucket(ctx, "gs://my-benchmark-bucket")
	bucket, err := blob.OpenBucket(ctx, "mem://")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = bucket.Close() }()

	// Seed one object to read.
	const key = "benchlatency-object"
	if err := bucket.WriteAll(ctx, key, bytes.Repeat([]byte("x"), 1024), nil); err != nil {
		fmt.Println(err)
		return
	}

	op := func(ctx context.Context) error {
		_, err := bucket.ReadAll(ctx, key)
		return err
	}

	res, err := benchlatency.Run(ctx, benchlatency.Config{
		Rate:     1000,                   // 1000 reads/sec, open loop
		Duration: time.Second,            // real runs: 30s-5m
		Warmup:   200 * time.Millisecond, // discard connection warmup
	}, op)
	if err != nil {
		fmt.Println(err)
		return
	}

	// In a real run, call res.Report(os.Stdout, "blob.ReadAll"). We suppress
	// the nondeterministic latency numbers here so the example output is stable.
	if res.Count > 0 && res.P(99) >= res.P(50) {
		fmt.Println("ok")
	}

	// Output: ok
}
