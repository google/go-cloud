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
	"net/url"
	"os"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/google/go-cmp/cmp"
	"gocloud.dev/blob"
	"gocloud.dev/blob/driver"
	"gocloud.dev/blob/drivertest"
	"gocloud.dev/gcp"
	"gocloud.dev/internal/testing/setup"
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
	return &h.client.Client
}

func (h *harness) MakeDriver(ctx context.Context) (driver.Bucket, error) {
	return openBucket(ctx, h.client, bucketName, h.opts)
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
	// Can't really verify this one because the storage library returns
	// a sentinel error, storage.ErrObjectNotExist, for "not exists"
	// instead of the supported As type googleapi.Error.
	// Call ErrorAs anyway, and expect it to fail.
	var to *googleapi.Error
	if blob.ErrorAs(err, &to) {
		return errors.New("expected ErrorAs to fail")
	}
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

func TestOpenBucket(t *testing.T) {
	tests := []struct {
		description string
		bucketName  string
		nilClient   bool
		want        string
		wantErr     bool
	}{
		{
			description: "empty bucket name results in error",
			wantErr:     true,
		},
		{
			description: "nil client results in error",
			bucketName:  "foo",
			nilClient:   true,
			wantErr:     true,
		},
		{
			description: "success",
			bucketName:  "foo",
			want:        "foo",
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			var client *gcp.HTTPClient
			if !test.nilClient {
				var done func()
				client, _, done = setup.NewGCPClient(ctx, t)
				defer done()
			}

			// Create driver impl.
			drv, err := openBucket(ctx, client, test.bucketName, nil)
			if (err != nil) != test.wantErr {
				t.Errorf("got err %v want error %v", err, test.wantErr)
			}
			if drv != nil {
				if drv.name != test.want {
					t.Errorf("got %q want %q", drv.name, test.want)
				}
			}

			// Create concrete type.
			_, err = OpenBucket(ctx, client, test.bucketName, nil)
			if (err != nil) != test.wantErr {
				t.Errorf("got err %v want error %v", err, test.wantErr)
			}
		})
	}
}

func TestOpenURL(t *testing.T) {
	ctx := context.Background()

	// Create a file for use as a dummy private key file.
	privateKey := []byte("some content")
	pkFile, err := ioutil.TempFile("", "my-private-key")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(pkFile.Name())
	if _, err := pkFile.Write(privateKey); err != nil {
		t.Fatal(err)
	}
	if err := pkFile.Close(); err != nil {
		t.Fatal(err)
	}

	jsonCred := []byte(`{"client_id": "foo.apps.googleusercontent.com", "client_secret": "bar", "refresh_token": "baz", "type": "authorized_user"}`)
	credFile, err := ioutil.TempFile("", "my-creds")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(credFile.Name())
	if _, err := credFile.Write(jsonCred); err != nil {
		t.Fatal(err)
	}
	if err := credFile.Close(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		url      string
		wantName string
		wantOpts Options
		wantErr  bool
	}{
		{
			url:      "gs://mybucket?cred_path=" + credFile.Name(),
			wantName: "mybucket",
		},
		{
			url:      "gs://mybucket2?cred_path=" + credFile.Name(),
			wantName: "mybucket2",
		},
		{
			url:      "gs://foo?access_id=bar&cred_path=" + credFile.Name(),
			wantName: "foo",
			wantOpts: Options{GoogleAccessID: "bar"},
		},
		{
			url:     "gs://foo?private_key_path=/path/does/not/exist",
			wantErr: true,
		},
		{
			url:      "gs://foo?cred_path=" + credFile.Name() + "&private_key_path=" + pkFile.Name(),
			wantName: "foo",
			wantOpts: Options{PrivateKey: privateKey},
		},
		{
			url:     "gs://foo?cred_path=/path/does/not/exist",
			wantErr: true,
		},
		{
			url:     "gs://foo?cred_path=" + pkFile.Name(),
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.url, func(t *testing.T) {
			u, err := url.Parse(test.url)
			if err != nil {
				t.Fatal(err)
			}
			got, err := openURL(ctx, u)
			if (err != nil) != test.wantErr {
				t.Errorf("got err %v want error %v", err, test.wantErr)
			}
			if err != nil {
				return
			}
			gotB, ok := got.(*bucket)
			if !ok {
				t.Fatalf("got type %T want *bucket", got)
			}
			if gotB.name != test.wantName {
				t.Errorf("got bucket name %q want %q", gotB.name, test.wantName)
			}
			if diff := cmp.Diff(*gotB.opts, test.wantOpts); diff != "" {
				t.Errorf("got\n%v\nwant\n%v\ndiff\n%s", *gotB.opts, test.wantOpts, diff)
			}
		})
	}
}
