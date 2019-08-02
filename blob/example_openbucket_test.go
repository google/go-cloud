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

package blob_test

import (
	"context"
	"fmt"
	"log"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/memblob"
)

func Example_openFromURL() {
	ctx := context.Background()

	// Connect to a bucket using a URL.
	// This example uses "memblob", the in-memory implementation.
	// We need to add a blank import line to register the memblob driver's
	// URLOpener, which implements blob.BucketURLOpener:
	// import _ "gocloud.dev/blob/memblob"
	// memblob registers for the "mem" scheme.
	// All blob.OpenBucket URLs also work with "blob+" or "blob+bucket+" prefixes,
	// e.g., "blob+mem://" or "blob+bucket+mem://".
	b, err := blob.OpenBucket(ctx, "mem://")
	if err != nil {
		log.Fatal(err)
	}
	defer b.Close()

	// Now we can use b to read or write to blobs in the bucket.
	if err := b.WriteAll(ctx, "my-key", []byte("hello world"), nil); err != nil {
		log.Fatal(err)
	}
	data, err := b.ReadAll(ctx, "my-key")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(data))
	// Output:
	// hello world
}

func Example_openFromURLWithPrefix() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// Connect to a bucket using a URL, using the "prefix" query parameter to
	// target a subfolder in the bucket.
	// The prefix should end with "/", so that the resulting bucket operates
	// in a subfolder.
	b, err := blob.OpenBucket(ctx, "mem://?prefix=a/subfolder/")
	if err != nil {
		log.Fatal(err)
	}
	defer b.Close()

	// Bucket operations on <key> will be translated to "a/subfolder/<key>".
}
