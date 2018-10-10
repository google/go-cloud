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

package gcsblob

import (
	"context"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/drivertest"
	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/internal/testing/setup"
	"google.golang.org/api/googleapi"
)

const (
	// These constants capture values that were used during the last -record.
	//
	// If you want to use --record mode,
	// 1. Create a bucket in your GCP project:
	//    https://console.cloud.google.com/storage/browser, then "Create Bucket".
	// 2. Update the bucketName constant to your bucket name.
	// 3. Create a service account in your GCP project and update the
	//    serviceAccountID constant to it.
	// 4. Download a private key to a .pem file as described here:
	//    https://godoc.org/cloud.google.com/go/storage#SignedURLOptions
	//    and update the pathToPrivateKey constant with a path to the .pem file.
	// TODO(issue #300): Use Terraform to provision a bucket, and get the bucket
	//    name from the Terraform output instead (saving a copy of it for replay).
	bucketName       = "go-cloud-blob-test-bucket"
	serviceAccountID = "storage-viewer@go-cloud-test-216917.iam.gserviceaccount.com"
	pathToPrivateKey = "/usr/local/google/home/rvangent/Downloads/storage-viewer.pem"
)

type harness struct {
	client     *gcp.HTTPClient
	privateKey []byte
	rt         http.RoundTripper
	closer     func()
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	pk, err := ioutil.ReadFile(pathToPrivateKey)
	if err != nil {
		t.Fatalf("Couldn't find private key at %v: %v", pathToPrivateKey, err)
	}
	client, rt, done := setup.NewGCPClient(ctx, t)
	return &harness{client: client, privateKey: pk, rt: rt, closer: done}, nil
}

func (h *harness) HTTPClient() *http.Client {
	return &http.Client{Transport: h.rt}
}

func (h *harness) MakeBucket(ctx context.Context) (*blob.Bucket, error) {
	return OpenBucket(ctx, bucketName, h.client, &Options{
		GoogleAccessID: serviceAccountID,
		PrivateKey:     h.privateKey,
	})
}

func (h *harness) Close() {
	h.closer()
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, "../testdata")
}

// GCS-specific unit tests.
func TestBufferSize(t *testing.T) {
	tests := []struct {
		size int
		want int
	}{
		{
			size: 5 * 1024 * 1024,
			want: 5 * 1024 * 1024,
		},
		{
			size: 0,
			want: googleapi.DefaultUploadChunkSize,
		},
		{
			size: -1024,
			want: 0,
		},
	}
	for i, test := range tests {
		got := bufferSize(test.size)
		if got != test.want {
			t.Errorf("%d) got buffer size %d, want %d", i, got, test.want)
		}
	}
}
