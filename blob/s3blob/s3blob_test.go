// Copyright 2018 The Go Cloud Authors
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

package s3blob

import (
	"context"
	"testing"

	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/drivertest"
	"github.com/google/go-cloud/internal/testing/setup"
)

// bucketName records the bucket used for the last --record.
// If you want to -use --record mode,
// 1. Create a bucket in your AWS project from the S3 management console.
//    https://s3.console.aws.amazon.com/s3/home.
// 2. Update this constant to your bucket name.
// TODO(issue #300): Use Terraform to provision a bucket, and get the bucket
//    name from the Terraform output instead (saving a copy of it for replay).
const (
	bucketName = "go-cloud-bucket"
	region     = "us-east-2"
)

// makeBucket creates a *blob.Bucket and a function to close it after the test
// is done. It fails the test if the creation fails.
func makeBucket(t *testing.T) (*blob.Bucket, func()) {
	ctx := context.Background()
	sess, done := setup.NewAWSSession(t, region)
	b, err := OpenBucket(ctx, sess, bucketName)
	if err != nil {
		t.Fatal(err)
	}
	return b, done
}
func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, makeBucket, "../testdata")
}
