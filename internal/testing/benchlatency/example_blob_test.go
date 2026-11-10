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
	_ "gocloud.dev/blob/memblob"
	"gocloud.dev/internal/testing/benchlatency"
)

func Example_blobRead() {
	ctx := context.Background()

	// Open the bucket to measure. Use a real bucket URL for live numbers.
	bucket, err := blob.OpenBucket(ctx, "mem://")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = bucket.Close() }()

	// Write one object to read.
	const key = "benchlatency-object"
	if err := bucket.WriteAll(ctx, key, bytes.Repeat([]byte("x"), 1024), nil); err != nil {
		fmt.Println(err)
		return
	}

	// Measure reads at 1000 per second for 1s, after a 200ms warmup.
	op := func(ctx context.Context) error {
		_, err := bucket.ReadAll(ctx, key)
		return err
	}

	res, err := benchlatency.Run(ctx, benchlatency.Config{
		Rate:     1000,
		Duration: time.Second,
		Warmup:   200 * time.Millisecond,
	}, op)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Latencies vary between runs, so check them instead of calling res.Report.
	if res.Count > 0 && res.P(99) >= res.P(50) {
		fmt.Println("ok")
	}

	// Output: ok
}
