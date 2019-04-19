// Copyright 2018 The Go Cloud Development Kit Authors
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

package s3blob_test

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"gocloud.dev/blob"
	"gocloud.dev/blob/s3blob"
)

func Example() {
	// Establish an AWS session.
	// See https://docs.aws.amazon.com/sdk-for-go/api/aws/session/ for more info.
	// The region must match the region for "my_bucket".
	sess, err := session.NewSession(&aws.Config{Region: aws.String("us-west-1")})
	if err != nil {
		log.Fatal(err)
	}

	// Create a *blob.Bucket.
	ctx := context.Background()
	b, err := s3blob.OpenBucket(ctx, sess, "my-bucket", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer b.Close()

	// Now we can use b to read or write files to the container.
	data, err := b.ReadAll(ctx, "my-key")
	if err != nil {
		log.Fatal(err)
	}
	_ = data
}

func Example_openBucket() {
	ctx := context.Background()

	// OpenBucket creates a *blob.Bucket from a URL.
	b, err := blob.OpenBucket(ctx, "s3://my-bucket")
	defer b.Close()
	_, _ = b, err
}
