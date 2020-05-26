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
	"os/user"
	"path/filepath"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/google/go-cmp/cmp"
	"gocloud.dev/blob"
	"gocloud.dev/blob/driver"
	"gocloud.dev/blob/drivertest"
	"gocloud.dev/gcerrors"
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
	serviceAccountID = "storage-updater@go-cloud-test-216917.iam.gserviceaccount.com"
)

var pathToPrivateKey = flag.String("privatekey", "", "path to .pem file containing private key (required for --record); defaults to ~/Downloads/gcs-private-key.pem")

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
			usr, _ := user.Current()
			*pathToPrivateKey = filepath.Join(usr.HomeDir, "Downloads", "gcs-private-key.pem")
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
	return openBucket(ctx, h.client, bucketName, h.opts)
}

func (h *harness) Close() {
	h.closer()
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyContentLanguage{}})
}

func BenchmarkGcsblob(b *testing.B) {
	ctx := context.Background()
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		b.Fatal(err)
	}
	client, err := gcp.NewHTTPClient(gcp.DefaultTransport(), gcp.CredentialsTokenSource(creds))
	if err != nil {
		b.Fatal(err)
	}
	bkt, err := OpenBucket(context.Background(), client, bucketName, nil)
	if err != nil {
		b.Fatal(err)
	}
	drivertest.RunBenchmarks(b, bkt)
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

func (verifyContentLanguage) ErrorCheck(b *blob.Bucket, err error) error {
	// Can't really verify this one because the storage library returns
	// a sentinel error, storage.ErrObjectNotExist, for "not exists"
	// instead of the supported As type googleapi.Error.
	// Call ErrorAs anyway, and expect it to fail.
	var to *googleapi.Error
	if b.ErrorAs(err, &to) {
		return errors.New("expected ErrorAs to fail")
	}
	return nil
}

func (verifyContentLanguage) BeforeRead(as func(interface{}) bool) error {
	var objp **storage.ObjectHandle
	if !as(&objp) {
		return errors.New("BeforeRead.As failed to get ObjectHandle")
	}
	var sr *storage.Reader
	if !as(&sr) {
		return errors.New("BeforeRead.As failed to get Reader")
	}
	return nil
}

func (verifyContentLanguage) BeforeWrite(as func(interface{}) bool) error {
	var objp **storage.ObjectHandle
	if !as(&objp) {
		return errors.New("Writer.As failed to get ObjectHandle")
	}
	var sw *storage.Writer
	if !as(&sw) {
		return errors.New("Writer.As failed to get Writer")
	}
	sw.ContentLanguage = language
	return nil
}

func (verifyContentLanguage) BeforeCopy(as func(interface{}) bool) error {
	var coh *CopyObjectHandles
	if !as(&coh) {
		return errors.New("BeforeCopy.As failed to get CopyObjectHandles")
	}
	var copier *storage.Copier
	if !as(&copier) {
		return errors.New("BeforeCopy.As failed")
	}
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
	var rr *storage.Reader
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
	if o.IsDir {
		return nil
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
			if err == nil && drv != nil && drv.name != test.want {
				t.Errorf("got %q want %q", drv.name, test.want)
			}

			// Create portable type.
			b, err := OpenBucket(ctx, client, test.bucketName, nil)
			if b != nil {
				defer b.Close()
			}
			if (err != nil) != test.wantErr {
				t.Errorf("got err %v want error %v", err, test.wantErr)
			}
		})
	}
}

// TestBeforeReadNonExistentKey tests using BeforeRead on a nonexistent key.
func TestBeforeReadNonExistentKey(t *testing.T) {
	ctx := context.Background()
	h, err := newHarness(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	drv, err := h.MakeDriver(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bucket := blob.NewBucket(drv)
	defer bucket.Close()

	// Try reading a nonexistent key.
	_, err = bucket.NewReader(ctx, "nonexistent-key", &blob.ReaderOptions{
		BeforeRead: func(asFunc func(interface{}) bool) error {
			var objp **storage.ObjectHandle
			if !asFunc(&objp) {
				return errors.New("Reader.As failed to get ObjectHandle")
			}
			var rp *storage.Reader
			if asFunc(&rp) {
				return errors.New("Reader.As unexpectedly got storage.Reader")
			}
			return nil
		},
	})
	if err == nil || gcerrors.Code(err) != gcerrors.NotFound {
		t.Errorf("got error %v, wanted NotFound for Read", err)
	}
}

// TestPreconditions tests setting of ObjectHandle preconditions via As.
func TestPreconditions(t *testing.T) {
	const (
		key     = "precondition-key"
		key2    = "precondition-key2"
		content = "hello world"
	)

	ctx := context.Background()
	h, err := newHarness(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	drv, err := h.MakeDriver(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bucket := blob.NewBucket(drv)
	defer bucket.Close()

	// Try writing with a failing precondition.
	if err := bucket.WriteAll(ctx, key, []byte(content), &blob.WriterOptions{
		BeforeWrite: func(asFunc func(interface{}) bool) error {
			var objp **storage.ObjectHandle
			if !asFunc(&objp) {
				return errors.New("Writer.As failed to get ObjectHandle")
			}
			// Replace the ObjectHandle with a new one that adds Conditions.
			*objp = (*objp).If(storage.Conditions{GenerationMatch: -999})
			return nil
		},
	}); err == nil || gcerrors.Code(err) != gcerrors.FailedPrecondition {
		t.Errorf("got error %v, wanted FailedPrecondition for Write", err)
	}

	// Repeat with a precondition that will pass.
	if err := bucket.WriteAll(ctx, key, []byte(content), &blob.WriterOptions{
		BeforeWrite: func(asFunc func(interface{}) bool) error {
			var objp **storage.ObjectHandle
			if !asFunc(&objp) {
				return errors.New("Writer.As failed to get ObjectHandle")
			}
			// Replace the ObjectHandle with a new one that adds Conditions.
			*objp = (*objp).If(storage.Conditions{DoesNotExist: true})
			return nil
		},
	}); err != nil {
		t.Errorf("got error %v, wanted nil", err)
	}
	defer bucket.Delete(ctx, key)

	// Try reading with a failing precondition.
	_, err = bucket.NewReader(ctx, key, &blob.ReaderOptions{
		BeforeRead: func(asFunc func(interface{}) bool) error {
			var objp **storage.ObjectHandle
			if !asFunc(&objp) {
				return errors.New("Reader.As failed to get ObjectHandle")
			}
			// Replace the ObjectHandle with a new one.
			*objp = (*objp).Generation(999999)
			return nil
		},
	})
	if err == nil || gcerrors.Code(err) != gcerrors.NotFound {
		t.Errorf("got error %v, wanted NotFound for Read", err)
	}

	attrs, err := bucket.Attributes(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	var oa storage.ObjectAttrs
	if !attrs.As(&oa) {
		t.Fatal("Attributes.As failed")
	}
	generation := oa.Generation

	// Repeat with a precondition that will pass.
	reader, err := bucket.NewReader(ctx, key, &blob.ReaderOptions{
		BeforeRead: func(asFunc func(interface{}) bool) error {
			var objp **storage.ObjectHandle
			if !asFunc(&objp) {
				return errors.New("Reader.As failed to get ObjectHandle")
			}
			// Replace the ObjectHandle with a new one.
			*objp = (*objp).Generation(generation)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	gotBytes, err := ioutil.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(gotBytes); got != content {
		t.Errorf("got %q want %q", got, content)
	}

	// Try copying with a failing precondition on Dst.
	err = bucket.Copy(ctx, key2, key, &blob.CopyOptions{
		BeforeCopy: func(asFunc func(interface{}) bool) error {
			var coh *CopyObjectHandles
			if !asFunc(&coh) {
				return errors.New("Copy.As failed to get CopyObjectHandles")
			}
			// Replace the dst ObjectHandle with a new one.
			coh.Dst = coh.Dst.If(storage.Conditions{GenerationMatch: -999})
			return nil
		},
	})
	if err == nil || gcerrors.Code(err) != gcerrors.FailedPrecondition {
		t.Errorf("got error %v, wanted FailedPrecondition for Copy", err)
	}

	// Try copying with a failing precondition on Src.
	err = bucket.Copy(ctx, key2, key, &blob.CopyOptions{
		BeforeCopy: func(asFunc func(interface{}) bool) error {
			var coh *CopyObjectHandles
			if !asFunc(&coh) {
				return errors.New("Copy.As failed to get CopyObjectHandles")
			}
			// Replace the src ObjectHandle with a new one.
			coh.Src = coh.Src.Generation(9999999)
			return nil
		},
	})
	if err == nil || gcerrors.Code(err) != gcerrors.NotFound {
		t.Errorf("got error %v, wanted NotFound for Copy", err)
	}

	// Repeat with preconditions on Dst and Src that will succeed.
	err = bucket.Copy(ctx, key2, key, &blob.CopyOptions{
		BeforeCopy: func(asFunc func(interface{}) bool) error {
			var coh *CopyObjectHandles
			if !asFunc(&coh) {
				return errors.New("Reader.As failed to get CopyObjectHandles")
			}
			coh.Dst = coh.Dst.If(storage.Conditions{DoesNotExist: true})
			coh.Src = coh.Src.Generation(generation)
			return nil
		},
	})
	if err != nil {
		t.Error(err)
	}
	defer bucket.Delete(ctx, key2)
}

func TestURLOpenerForParams(t *testing.T) {
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

	tests := []struct {
		name     string
		currOpts Options
		query    url.Values
		wantOpts Options
		wantErr  bool
	}{
		{
			name: "InvalidParam",
			query: url.Values{
				"foo": {"bar"},
			},
			wantErr: true,
		},
		{
			name: "AccessID",
			query: url.Values{
				"access_id": {"bar"},
			},
			wantOpts: Options{GoogleAccessID: "bar"},
		},
		{
			name:     "AccessID override",
			currOpts: Options{GoogleAccessID: "foo"},
			query: url.Values{
				"access_id": {"bar"},
			},
			wantOpts: Options{GoogleAccessID: "bar"},
		},
		{
			name:     "AccessID not overridden",
			currOpts: Options{GoogleAccessID: "bar"},
			wantOpts: Options{GoogleAccessID: "bar"},
		},
		{
			name: "BadPrivateKeyPath",
			query: url.Values{
				"private_key_path": {"/path/does/not/exist"},
			},
			wantErr: true,
		},
		{
			name: "PrivateKeyPath",
			query: url.Values{
				"private_key_path": {pkFile.Name()},
			},
			wantOpts: Options{PrivateKey: privateKey},
		},
		{
			name:     "PrivateKey cleared",
			currOpts: Options{PrivateKey: privateKey},
			query: url.Values{
				"private_key_path": {""},
			},
			wantOpts: Options{},
		},
		{
			name: "AccessID change clears PrivateKey and MakeSignBytes",
			currOpts: Options{
				GoogleAccessID: "foo",
				PrivateKey:     privateKey,
				MakeSignBytes: func(context.Context) SignBytesFunc {
					return func([]byte) ([]byte, error) {
						return nil, context.DeadlineExceeded
					}
				},
			},
			query: url.Values{
				"access_id": {"bar"},
			},
			wantOpts: Options{GoogleAccessID: "bar"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			o := &URLOpener{Options: test.currOpts}
			got, err := o.forParams(ctx, test.query)
			if (err != nil) != test.wantErr {
				t.Errorf("got err %v want error %v", err, test.wantErr)
			}
			if err != nil {
				return
			}
			if diff := cmp.Diff(got, &test.wantOpts); diff != "" {
				t.Errorf("opener.forParams(...) diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestOpenBucketFromURL(t *testing.T) {
	cleanup := setup.FakeGCPDefaultCredentials(t)
	defer cleanup()

	pkFile, err := ioutil.TempFile("", "my-private-key")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(pkFile.Name())
	if err := ioutil.WriteFile(pkFile.Name(), []byte("key"), 0666); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"gs://mybucket", false},
		// OK, setting access_id.
		{"gs://mybucket?access_id=foo", false},
		// OK, setting private_key_path.
		{"gs://mybucket?private_key_path=" + pkFile.Name(), false},
		// OK, clearing any pre-existing private key.
		{"gs://mybucket?private_key_path=", false},
		// Invalid private_key_path.
		{"gs://mybucket?private_key_path=invalid-path", true},
		// Invalid parameter.
		{"gs://mybucket?param=value", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		b, err := blob.OpenBucket(ctx, test.URL)
		if b != nil {
			defer b.Close()
		}
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
	}
}

func TestReadDefaultCredentials(t *testing.T) {
	tests := []struct {
		givenJSON      string
		WantAccessID   string
		WantPrivateKey []byte
	}{
		// Variant A: service account file
		{`{
			"type": "service_account",
			"project_id": "project-id",
			"private_key_id": "key-id",
			"private_key": "-----BEGIN PRIVATE KEY-----\nprivate-key\n-----END PRIVATE KEY-----\n",
			"client_email": "service-account-email",
			"client_id": "client-id",
			"auth_uri": "https://accounts.google.com/o/oauth2/auth",
			"token_uri": "https://accounts.google.com/o/oauth2/token",
			"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
			"client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/service-account-email"
		  }`,
			"service-account-email",
			[]byte("-----BEGIN PRIVATE KEY-----\nprivate-key\n-----END PRIVATE KEY-----\n"),
		},
		// Variant A: credentials file absent a private key (stripped)
		{`{
			"google": {},
			"client_email": "service-account-email",
			"client_id": "client-id"
		  }`,
			"service-account-email",
			[]byte(""),
		},
		// Variant B: obtained through the REST API
		{`{
			"name": "projects/project-id/serviceAccounts/service-account-email/keys/key-id",
			"privateKeyType": "TYPE_GOOGLE_CREDENTIALS_FILE",
			"privateKeyData": "private-key",
			"validAfterTime": "date",
			"validBeforeTime": "date",
			"keyAlgorithm": "KEY_ALG_RSA_2048"
		  }`,
			"service-account-email",
			[]byte("private-key"),
		},
		// An empty input shall not throw an exception
		{"", "", nil},
	}

	for i, test := range tests {
		inJSON := []byte(test.givenJSON)
		if len(test.givenJSON) == 0 {
			inJSON = nil
		}

		gotAccessID, gotPrivateKey := readDefaultCredentials(inJSON)
		if gotAccessID != test.WantAccessID || string(gotPrivateKey) != string(test.WantPrivateKey) {
			t.Errorf("Mismatched field values in case %d:\n -- got:  %v, %v\n -- want: %v, %v", i,
				gotAccessID, gotPrivateKey,
				test.WantAccessID, test.WantPrivateKey,
			)
		}
	}
}

func TestRemainingSignedURLSchemes(t *testing.T) {
	tests := []struct {
		name          string
		currOpts      Options
		wantSignedURL string // Not the actual URL, which is subject to change, but a mimickry.
		wantErr       bool
	}{
		{
			name:    "no scheme available, error",
			wantErr: true,
		},
		{
			name: "too many schemes configured",
			currOpts: Options{
				GoogleAccessID: "foo",
				PrivateKey:     []byte("private-key"),
				SignBytes: func([]byte) ([]byte, error) {
					return []byte("signed"), nil
				},
			},
			wantErr: true,
		},
		{
			name: "SignBytes",
			currOpts: Options{
				GoogleAccessID: "foo",
				SignBytes: func([]byte) ([]byte, error) {
					return []byte("signed"), nil
				},
			},
			wantSignedURL: "https://host/go-cloud-blob-test-bucket/some-key?GoogleAccessId=foo&Signature=c2lnbmVk",
		},
		{
			name: "MakeSignBytes is being used",
			currOpts: Options{
				GoogleAccessID: "foo",
				MakeSignBytes: func(context.Context) SignBytesFunc {
					return func([]byte) ([]byte, error) {
						return []byte("signed"), nil
					}
				},
			},
			wantSignedURL: "https://host/go-cloud-blob-test-bucket/some-key?GoogleAccessId=foo&Signature=c2lnbmVk",
		},
	}

	ctx := context.Background()
	signOpts := &driver.SignedURLOptions{
		Expiry: 30 * time.Second,
		Method: http.MethodGet,
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bucket := bucket{name: bucketName, opts: &test.currOpts}

			// SignedURL doesn't check whether a key exists.
			gotURL, gotErr := bucket.SignedURL(ctx, "some-key", signOpts)
			if (gotErr != nil) != test.wantErr {
				t.Errorf("Got unexpected error %v", gotErr)
			}
			if test.wantSignedURL == "" {
				return
			}

			got, _ := url.Parse(gotURL)
			want, _ := url.Parse(test.wantSignedURL)
			gotParams, wantParams := got.Query(), want.Query()
			for _, param := range []string{"GoogleAccessId", "Signature"} {
				if gotParams.Get(param) != wantParams.Get(param) {
					// Print the full URL because the parameter might've not been set at all.
					t.Errorf("Query parameter in SignedURL differs: %v\n -- got URL:  %v\n -- want URL: %v",
						param, got, want)
				}
			}
		})
	}
}
