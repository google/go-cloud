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

package huaweiblob

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/google/go-replayers/httpreplay"
	"gocloud.dev/blob"
	"gocloud.dev/blob/driver"
	"gocloud.dev/blob/drivertest"
	"gocloud.dev/internal/testing/setup"
)

// These constants record the endpoint, region & bucket used for the last
// --record. If you want to use --record mode:
//  1. Make sure the OBS bucket exists (or create it, e.g. via the Huawei
//     Cloud console) and set OBS_ACCESS_KEY_ID / OBS_SECRET_KEY.
//  2. Update these constants to match your bucket and endpoint.
const (
	bucketName = "go-cloud-testing-2"
	region     = "cn-north-4"
	// endpoint uses an "http" prefix so the driver sends path-style
	// requests (http://<endpoint>/<bucket>/<key>) to the local OBS mock.
	// This keeps the recorded and replayed URLs identical and allows
	// offline replay without network access or real credentials.
	endpoint = "http://127.0.0.1:28080"
)

type harness struct {
	client    *http.Client
	accessKey string
	secretKey string
	closer    func()
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	t.Helper()
	client, done, _ := setup.NewRecordReplayClient(ctx, t, func(r *httpreplay.Recorder) {
		// OBS signs requests with SigV4; the Authorization header includes
		// the request host and would not match across record/replay.
		r.RemoveRequestHeaders("Authorization")
		// The date header changes on every request.
		r.ClearHeaders("X-Obs-Date")
		r.ClearQueryParams("X-Obs-Date")
		r.ClearHeaders("User-Agent")
	})
	return &harness{client: client, closer: done, accessKey: obsAccessKeyID(t), secretKey: obsSecretKey(t)}, nil
}

func (h *harness) HTTPClient() *http.Client {
	return h.client
}

func (h *harness) MakeDriver(ctx context.Context) (driver.Bucket, error) {
	return openBucket(ctx, &Options{
		Endpoint:    endpoint,
		BucketName:  bucketName,
		AccessKeyID: h.accessKey,
		SecretKey:   h.secretKey,
		Region:      region,
		HTTPClient:  h.client,
	})
}

func (h *harness) MakeDriverForNonexistentBucket(ctx context.Context) (driver.Bucket, error) {
	return openBucket(ctx, &Options{
		Endpoint:    endpoint,
		BucketName:  "go-cdk-bucket-does-not-exist",
		AccessKeyID: h.accessKey,
		SecretKey:   h.secretKey,
		Region:      region,
		HTTPClient:  h.client,
	})
}

func (*harness) SupportsCopy() bool               { return true }
func (*harness) SupportsNonSlashDelimiters() bool { return true }

// SkipDownloadAt avoids testing OBS's handling of read-with-offset from a
// directory-like prefix; OBS doesn't support the needed range semantics.
func (*harness) SkipDownloadAt() bool { return true }

func (h *harness) Close() {
	h.closer()
}

// obsAccessKeyID and obsSecretKey return the credentials to use. In replay
// mode they return fake credentials (the Authorization header for the
// recorded requests is scrubbed during recording), and in --record mode they
// require the OBS_ACCESS_KEY_ID / OBS_SECRET_KEY environment variables.
func obsAccessKeyID(t *testing.T) string {
	t.Helper()
	if *setup.Record {
		if v := os.Getenv("OBS_ACCESS_KEY_ID"); v != "" {
			return v
		}
		t.Fatal("--record requires OBS_ACCESS_KEY_ID to be set")
	}
	return "FAKE_ACCESS_KEY"
}

func obsSecretKey(t *testing.T) string {
	t.Helper()
	if *setup.Record {
		if v := os.Getenv("OBS_SECRET_KEY"); v != "" {
			return v
		}
		t.Fatal("--record requires OBS_SECRET_KEY to be set")
	}
	return "FAKE_SECRET_KEY"
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyObsAs{}})
}

// verifyObsAs uses As to access the underlying HTTP request/response types,
// validating that the driver exposes the raw OBS API interactions.
type verifyObsAs struct{}

func (verifyObsAs) Name() string {
	return "verify OBS requests/responses can be accessed through As"
}

func (verifyObsAs) BucketCheck(b *blob.Bucket) error {
	var client *http.Client
	if !b.As(&client) {
		return errors.New("Bucket.As for *http.Client failed")
	}
	return nil
}

func (verifyObsAs) ErrorCheck(b *blob.Bucket, err error) error {
	var e *obsError
	if !b.ErrorAs(err, &e) {
		return errors.New("blob.ErrorAs for *obsError failed")
	}
	if !errors.As(err, &e) {
		return errors.New("error.As for *obsError failed")
	}
	return nil
}

func (verifyObsAs) BeforeRead(as func(any) bool) error {
	var req *http.Request
	if !as(&req) {
		return errors.New("BeforeRead As for *http.Request failed")
	}
	return nil
}

func (verifyObsAs) BeforeWrite(as func(any) bool) error {
	var req *http.Request
	if !as(&req) {
		return errors.New("BeforeWrite As for *http.Request failed")
	}
	return nil
}

func (verifyObsAs) BeforeCopy(as func(any) bool) error {
	var req *http.Request
	if !as(&req) {
		return errors.New("BeforeCopy As for *http.Request failed")
	}
	return nil
}

func (verifyObsAs) BeforeDelete(as func(any) bool) error {
	var req *http.Request
	if !as(&req) {
		return errors.New("BeforeDelete As for *http.Request failed")
	}
	return nil
}

func (verifyObsAs) BeforeList(as func(any) bool) error {
	var req *http.Request
	if !as(&req) {
		return errors.New("BeforeList As for *http.Request failed")
	}
	return nil
}

func (verifyObsAs) BeforeSign(as func(any) bool) error {
	var req *http.Request
	if as(&req) {
		return nil
	}
	return errors.New("BeforeSign As for *http.Request failed")
}

func (verifyObsAs) AttributesCheck(attrs *blob.Attributes) error {
	var hdr http.Header
	if !attrs.As(&hdr) {
		return errors.New("Attributes.As for http.Header returned false")
	}
	return nil
}

func (verifyObsAs) ReaderCheck(r *blob.Reader) error {
	var resp *http.Response
	if !r.As(&resp) {
		return errors.New("Reader.As for *http.Response returned false")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("got status %d, want 200", resp.StatusCode)
	}
	return nil
}

func (verifyObsAs) ListObjectCheck(o *blob.ListObject) error {
	if o.IsDir {
		var cp listCommonPrefix
		if !o.As(&cp) {
			return errors.New("ListObject.As for directory returned false")
		}
		return nil
	}
	var lc listContents
	if !o.As(&lc) {
		return errors.New("ListObject.As for object returned false")
	}
	if lc.Key != o.Key {
		return errors.New("ListObject.As for object returned a different item")
	}
	return nil
}

func TestObsErrorMessage(t *testing.T) {
	tests := []struct {
		description string
		err         *obsError
		want        string
	}{
		{
			description: "HEAD 404 has no body: status text fallback",
			err:         &obsError{StatusCode: http.StatusNotFound},
			want:        "huaweiblob: OBS error (status 404): Not Found",
		},
		{
			description: "key is included when known",
			err:         &obsError{StatusCode: http.StatusNotFound, Key: "does-not-exist"},
			want:        `huaweiblob: OBS error (key "does-not-exist") (status 404): Not Found`,
		},
		{
			description: "XML body provides code and message",
			err:         &obsError{StatusCode: http.StatusBadRequest, Code: "InvalidRequest", Message: "bad request"},
			want:        "huaweiblob: OBS error (status 400): InvalidRequest: bad request",
		},
		{
			description: "message without code",
			err:         &obsError{StatusCode: http.StatusBadRequest, Message: "plain message"},
			want:        "huaweiblob: OBS error (status 400): plain message",
		},
	}
	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			if got := test.err.Error(); got != test.want {
				t.Errorf("got %q want %q", got, test.want)
			}
		})
	}
}

// TestWithKey verifies that withKey stamps the key on an *obsError while
// preserving the error identity required by drivertest's ErrorCheck, which
// uses both blob.Bucket.ErrorAs and errors.As.
func TestWithKey(t *testing.T) {
	plain := errors.New("plain error")
	if got := withKey(plain, "foo"); got != plain {
		t.Errorf("withKey must pass non-obsError through unchanged, got %v", got)
	}

	oe := &obsError{StatusCode: http.StatusNotFound}
	err := withKey(oe, "missing-key")
	var target *obsError
	if !errors.As(err, &target) || target != oe {
		t.Errorf("withKey must preserve the *obsError identity, got %v", err)
	}
	if oe.Key != "missing-key" {
		t.Errorf("got key %q want %q", oe.Key, "missing-key")
	}

	// An empty key must be a no-op (e.g. an already-stamped key is kept).
	oe2 := &obsError{StatusCode: http.StatusNotFound, Key: "orig"}
	if got := withKey(oe2, ""); !errors.As(got, &target) || target.Key != "orig" {
		t.Errorf("empty key must be a no-op, got %v", got)
	}
}

func TestOpenBucket(t *testing.T) {
	tests := []struct {
		description string
		opts        *Options
		want        string
		wantErr     bool
	}{
		{
			description: "empty endpoint results in error",
			opts:        &Options{BucketName: "foo", AccessKeyID: "ak", SecretKey: "sk"},
			wantErr:     true,
		},
		{
			description: "empty bucketName results in error",
			opts:        &Options{Endpoint: endpoint, AccessKeyID: "ak", SecretKey: "sk"},
			wantErr:     true,
		},
		{
			description: "empty access key results in error",
			opts:        &Options{Endpoint: endpoint, BucketName: "foo", SecretKey: "sk"},
			wantErr:     true,
		},
		{
			description: "empty secret key results in error",
			opts:        &Options{Endpoint: endpoint, BucketName: "foo", AccessKeyID: "ak"},
			wantErr:     true,
		},
		{
			description: "success",
			opts:        &Options{Endpoint: endpoint, BucketName: "foo", AccessKeyID: "ak", SecretKey: "sk"},
			want:        "foo",
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			// Create driver impl.
			drv, err := openBucket(ctx, test.opts)
			if (err != nil) != test.wantErr {
				t.Errorf("got err %v want error %v", err, test.wantErr)
			}
			if err == nil {
				if b, ok := drv.(*bucket); !ok || b.bucketName != test.want {
					t.Errorf("got bucket name %v want %q", drv, test.want)
				}
			}

			// Create portable type.
			b, err := OpenBucket(ctx, test.opts)
			if b != nil {
				defer func(b *blob.Bucket) {
					if err := b.Close(); err != nil {
						t.Errorf("failed to Close: %v", err)
					}
				}(b)
			}
			if (err != nil) != test.wantErr {
				t.Errorf("got err %v want error %v", err, test.wantErr)
			}
		})
	}
}

func TestOpenBucketFromURL(t *testing.T) {
	tests := []struct {
		URL     string
		WantErr bool
	}{
		// Error: missing all required options.
		{"obs://mybucket", true},
		// Error: missing endpoint.
		{"obs://mybucket?access_key=AK&secret_key=SK", true},
		// Error: missing access key.
		{"obs://mybucket?endpoint=" + endpoint + "&secret_key=SK", true},
		// Error: missing secret key.
		{"obs://mybucket?endpoint=" + endpoint + "&access_key=AK", true},
		// OK, all options.
		{"obs://mybucket?endpoint=" + endpoint + "&access_key=AK&secret_key=SK", false},
		// OK, all options plus region.
		{"obs://mybucket?endpoint=" + endpoint + "&access_key=AK&secret_key=SK&region=cn-north-4", false},
		// Error: invalid query parameter.
		{"obs://mybucket?endpoint=" + endpoint + "&access_key=AK&secret_key=SK&param=value", true},
		// Error: query parameter with multiple values.
		{"obs://mybucket?endpoint=" + endpoint + "&access_key=AK&secret_key=SK&region=a&region=b", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		b, err := blob.OpenBucket(ctx, test.URL)
		if b != nil {
			defer func(b *blob.Bucket) {
				if err := b.Close(); err != nil {
					t.Errorf("failed to Close: %v", err)
				}
			}(b)
		}
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
	}
}
