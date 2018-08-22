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

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/drivertest"
	"github.com/google/go-cloud/internal/testing/setup"
)

// These constants record the region & bucket used for the last --record.
// If you want to use --record mode,
// 1. Create a bucket in your AWS project from the S3 management console.
//    https://s3.console.aws.amazon.com/s3/home.
// 2. Update this constant to your bucket name.
// TODO(issue #300): Use Terraform to provision a bucket, and get the bucket
//    name from the Terraform output instead (saving a copy of it for replay).
const (
	bucketName = "go-cloud-bucket"
	region     = "us-east-2"
)

type harness struct {
	session *session.Session
	closer  func()
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	sess, done := setup.NewAWSSession(t, region)
	return &harness{session: sess, closer: done}, nil
}

func (h *harness) MakeBucket(ctx context.Context) (*blob.Bucket, error) {
	return OpenBucket(ctx, h.session, bucketName)
}

func (h *harness) Close() {
	h.closer()
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, "../testdata")
}
