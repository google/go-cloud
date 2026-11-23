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

package huaweiblob_test

import (
	"context"
	"fmt"
	"log"

	"gocloud.dev/blob"
	"gocloud.dev/blob/huaweiblob"
)

func ExampleOpenBucket() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments
	// like these are ignored by the go command, but are used by the
	// documentation generator.

	ctx := context.Background()

	// Create an *http.Client, using credentials from the environment or
	// from your application's configuration, then open the bucket.
	//   client, err := obs.New(...)
	//   ...
	bucket, err := huaweiblob.OpenBucket(ctx, &huaweiblob.Options{
		Endpoint:    "https://obs.cn-north-4.myhuaweicloud.com",
		BucketName:  "my-bucket",
		AccessKeyID: "your-access-key",
		SecretKey:   "your-secret-key",
		Region:      "cn-north-4",
	})
	if err != nil {
		log.Fatal(err)
	}
	defer bucket.Close()

	// Now we can use the bucket, e.g., to read or write an object.
	data, err := bucket.ReadAll(ctx, "my-key")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(data))
}

func Example_openBucketFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments
	// like these are ignored by the go command, but are used by the
	// documentation generator.

	ctx := context.Background()

	// blob.OpenBucket creates a *blob.Bucket from a URL.
	// Use the URL Query parameters to set the required options:
	//   - endpoint (repeatable), access_key, secret_key, region.
	bucket, err := blob.OpenBucket(ctx, "obs://my-bucket?endpoint=https%3A%2F%2Fobs.cn-north-4.myhuaweicloud.com&access_key=your-access-key&secret_key=your-secret-key&region=cn-north-4")
	if err != nil {
		log.Fatal(err)
	}
	defer bucket.Close()

	// Now we can use the bucket, e.g., to read or write an object.
	data, err := bucket.ReadAll(ctx, "my-key")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(data))
}