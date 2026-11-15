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

package memblob

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"gocloud.dev/blob"
	"gocloud.dev/blob/driver"
	"gocloud.dev/blob/drivertest"
)

type harness struct {
	prefix string
}

func newHarness(ctx context.Context, t *testing.T, prefix string) (drivertest.Harness, error) {
	t.Helper()

	return &harness{prefix: prefix}, nil
}

func (h *harness) HTTPClient() *http.Client {
	return nil
}

func (h *harness) MakeDriver(ctx context.Context) (driver.Bucket, error) {
	drv := openBucket(nil)
	if h.prefix == "" {
		return drv, nil
	}
	return driver.NewPrefixedBucket(drv, h.prefix), nil
}

func (h *harness) MakeDriverForNonexistentBucket(ctx context.Context) (driver.Bucket, error) {
	// Does not make sense for this driver.
	return nil, nil
}

func (*harness) SupportsCopy() bool               { return true }
func (*harness) SupportsNonSlashDelimiters() bool { return true }
func (*harness) SkipDownloadAt() bool             { return false }

func (h *harness) Close() {}

type verifyAs struct{}

func (verifyAs) Name() string { return "verify As types for memblob" }

func (verifyAs) BucketCheck(b *blob.Bucket) error {
	var s string
	if b.As(&s) {
		return errors.New("Bucket.As unexpectedly succeeded")
	}
	return nil
}

func (verifyAs) BeforeRead(as func(any) bool) error {
	var s string
	if as(&s) {
		return errors.New("BeforeRead.As unexpectedly succeeded")
	}
	return nil
}

func (verifyAs) BeforeWrite(as func(any) bool) error {
	var s string
	if as(&s) {
		return errors.New("BeforeWrite.As unexpectedly succeeded")
	}
	return nil
}

func (verifyAs) BeforeCopy(as func(any) bool) error {
	var s string
	if as(&s) {
		return errors.New("BeforeCopy.As unexpectedly succeeded")
	}
	return nil
}
func (verifyAs) BeforeDelete(as func(any) bool) error { return nil }
func (verifyAs) BeforeList(as func(any) bool) error   { return nil }
func (verifyAs) BeforeSign(as func(any) bool) error   { return nil }
func (verifyAs) AttributesCheck(attrs *blob.Attributes) error {
	var s string
	if attrs.As(s) {
		return errors.New("Attributes.As unexpectedly succeeded")
	}
	return nil
}

func (verifyAs) ReaderCheck(r *blob.Reader) error {
	var ior io.Reader
	if r.As(&ior) {
		return errors.New("Reader.As unexpectedly succeeded")
	}
	return nil
}

func (verifyAs) ListObjectCheck(o *blob.ListObject) error {
	var s string
	if o.As(&s) {
		return errors.New("ListObject.As unexpectedly succeeded")
	}
	return nil
}

func (v verifyAs) ErrorCheck(b *blob.Bucket, err error) error {
	var err2 error
	if b.ErrorAs(err, &err2) {
		return errors.New("ErrorAs unexpectedly succeeded")
	}
	return nil
}

func TestConformance(t *testing.T) {
	newHarnessNoPrefix := func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
		t.Helper()

		return newHarness(ctx, t, "")
	}
	drivertest.RunConformanceTests(t, newHarnessNoPrefix, []drivertest.AsTest{verifyAs{}})
}

func TestConformanceWithPrefix(t *testing.T) {
	const prefix = "some/prefix/dir/"
	newHarnessWithPrefix := func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
		t.Helper()

		return newHarness(ctx, t, prefix)
	}
	drivertest.RunConformanceTests(t, newHarnessWithPrefix, []drivertest.AsTest{verifyAs{}})
}

func BenchmarkMemblob(b *testing.B) {
	drivertest.RunBenchmarks(b, OpenBucket(nil))
}

func TestOpenBucketFromURL(t *testing.T) {
	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"mem://", false},
		// NoMD5.
		{"mem://?nomd5", false},
		// With prefix.
		{"mem://?prefix=foo/bar", false},
		// Invalid parameter.
		{"mem://?param=value", true},
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
