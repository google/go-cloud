// Copyright 2018 Google LLC
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

package gcsblob_test

import (
	"context"
	"flag"
	"testing"

	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/gcsblob"
	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/internal/testing/setup"
)

var testBucket = flag.String("gcs-bucket", "pledged-solved-practically", "GCS bucket name used for testing")

func TestReadOfNonExistentFile(t *testing.T) {
	if !*setup.Record {
		// TODO(shantuo): use replay once
		// https://github.com/google/go-cloud/issues/49 is fixed.
		t.Skip("Skipping integration test in short mode")
	}
	ctx := context.Background()
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		t.Fatal(err)
	}
	c, err := gcp.NewHTTPClient(gcp.DefaultTransport(), gcp.CredentialsTokenSource(creds))
	if err != nil {
		t.Fatal(err)
	}
	bucket, err := gcsblob.OpenBucket(ctx, *testBucket, c)
	if err != nil {
		t.Fatal("error getting bucket:", err)
	}
	t.Run("ObjectNotExist", func(t *testing.T) {
		if _, err := bucket.NewReader(ctx, "test_notexist"); err == nil || !blob.IsNotExist(err) {
			t.Errorf("NewReader: got %#v, want not exist error", err)
		}
	})
}

func TestDeleteNonExistentFile(t *testing.T) {
	if !*setup.Record {
		// TODO(shantuo): use replay once
		// https://github.com/google/go-cloud/issues/49 is fixed.
		t.Skip("Skipping integration test in short mode")
	}
	ctx := context.Background()
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		t.Fatal(err)
	}
	c, err := gcp.NewHTTPClient(gcp.DefaultTransport(), gcp.CredentialsTokenSource(creds))
	if err != nil {
		t.Fatal(err)
	}
	bucket, err := gcsblob.OpenBucket(ctx, *testBucket, c)
	if err != nil {
		t.Fatal("error getting bucket:", err)
	}
	t.Run("ObjectNotExist", func(t *testing.T) {
		if err := bucket.Delete(ctx, "test_notexist"); err == nil || !blob.IsNotExist(err) {
			t.Errorf("Delete: got %#v, want not exist error", err)
		}
	})
}
