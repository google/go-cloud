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
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/driver"
	"github.com/google/go-cloud/blob/drivertest"
	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/internal/testing/setup"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
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
	//    and pass a path to it via the --privatekey flag.
	// TODO(issue #300): Use Terraform to provision a bucket, and get the bucket
	//    name from the Terraform output instead (saving a copy of it for replay).
	bucketName       = "go-cloud-blob-test-bucket"
	serviceAccountID = "storage-viewer@go-cloud-test-216917.iam.gserviceaccount.com"
)

var pathToPrivateKey = flag.String("privatekey", "", "path to .pem file containing private key (required for --record)")

type harness struct {
	client *gcp.HTTPClient
	opts   *Options
	rt     http.RoundTripper
	closer func()
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	opts := &Options{GoogleAccessID: serviceAccountID}
	if *setup.Record {
		if *pathToPrivateKey == "" {
			t.Fatalf("--privatekey is required in --record mode.")
		}
		// Use a real private key for signing URLs during -record.
		pk, err := ioutil.ReadFile(*pathToPrivateKey)
		if err != nil {
			t.Fatalf("Couldn't find private key at %v: %v", *pathToPrivateKey, err)
		}
		opts.PrivateKey = pk
	} else {
		// Use a dummy signer in replay mode.
		opts.SignBytes = func(b []byte) ([]byte, error) { return []byte("signed!"), nil }
	}
	client, rt, done := setup.NewGCPClient(ctx, t)
	return &harness{client: client, opts: opts, rt: rt, closer: done}, nil
}

func (h *harness) HTTPClient() *http.Client {
	return &http.Client{Transport: h.rt}
}

func (h *harness) MakeDriver(ctx context.Context) (driver.Bucket, error) {
	c, err := storage.NewClient(ctx, option.WithHTTPClient(&h.client.Client))
	if err != nil {
		return nil, err
	}
	return &bucket{name: bucketName, client: c, opts: h.opts}, nil
}

func (h *harness) Close() {
	h.closer()
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyContentLanguage{}})
}

const language = "nl"

// verifyContentLanguage uses As to access the underlying GCS types and
// read/write the ContentLanguage field.
type verifyContentLanguage struct{}

func (verifyContentLanguage) Name() string {
	return "verify ContentLanguage can be written and read through As"
}

func (verifyContentLanguage) BucketCheck(b *blob.Bucket) error {
	var client *storage.Client
	if !b.As(&client) {
		return errors.New("Bucket.As failed")
	}
	return nil
}

func (verifyContentLanguage) ErrorCheck(err error) error {
	// Can't verify this one because the storage library returns a sentinel
	// error, storage.ErrObjectNotExist, for "not exists" instead of the
	// supported As type googleapi.Error.
	return nil
}

func (verifyContentLanguage) BeforeWrite(as func(interface{}) bool) error {
	var sw *storage.Writer
	if !as(&sw) {
		return errors.New("Writer.As failed")
	}
	sw.ContentLanguage = language
	return nil
}

func (verifyContentLanguage) BeforeList(as func(interface{}) bool) error {
	var q *storage.Query
	if !as(&q) {
		return errors.New("List.As failed")
	}
	// Nothing to do.
	return nil
}

func (verifyContentLanguage) AttributesCheck(attrs *blob.Attributes) error {
	var oa storage.ObjectAttrs
	if !attrs.As(&oa) {
		return errors.New("Attributes.As returned false")
	}
	if got := oa.ContentLanguage; got != language {
		return fmt.Errorf("got %q want %q", got, language)
	}
	return nil
}

func (verifyContentLanguage) ReaderCheck(r *blob.Reader) error {
	var rr storage.Reader
	if !r.As(&rr) {
		return errors.New("Reader.As returned false")
	}
	// GCS doesn't return Content-Language via storage.Reader.
	return nil
}

func (verifyContentLanguage) ListObjectCheck(o *blob.ListObject) error {
	var oa storage.ObjectAttrs
	if !o.As(&oa) {
		return errors.New("ListObject.As returned false")
	}
	if got := oa.ContentLanguage; got != language {
		return fmt.Errorf("got %q want %q", got, language)
	}
	return nil
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
