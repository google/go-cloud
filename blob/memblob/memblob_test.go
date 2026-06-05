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
	"bytes"
	"context"
	"crypto/md5"
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

func (h *harness) Close() {}

func TestConformance(t *testing.T) {
	newHarnessNoPrefix := func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
		t.Helper()

		return newHarness(ctx, t, "")
	}
	drivertest.RunConformanceTests(t, newHarnessNoPrefix, nil)
}

func TestConformanceWithPrefix(t *testing.T) {
	const prefix = "some/prefix/dir/"
	newHarnessWithPrefix := func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
		t.Helper()

		return newHarness(ctx, t, prefix)
	}
	drivertest.RunConformanceTests(t, newHarnessWithPrefix, nil)
}

func BenchmarkMemblob(b *testing.B) {
	drivertest.RunBenchmarks(b, OpenBucket(nil))
}

// TestUploadMD5 verifies that blobs written via the Upload fast-path store the
// correct MD5 attribute. Previously, writer.Upload wrote only to the buffer and
// skipped feeding data through the md5 hasher, so the stored MD5 was always
// the hash of an empty input rather than the actual content hash.
func TestUploadMD5(t *testing.T) {
	ctx := context.Background()
	b := OpenBucket(nil)
	defer b.Close()

	content := []byte("hello memblob upload md5")

	// blob.Bucket.Upload uses the driver's Upload method when available.
	if err := b.Upload(ctx, "testkey", bytes.NewReader(content), &blob.WriterOptions{
		ContentType: "text/plain",
	}); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	attrs, err := b.Attributes(ctx, "testkey")
	if err != nil {
		t.Fatalf("Attributes: %v", err)
	}

	want := md5.Sum(content)
	if !bytes.Equal(attrs.MD5, want[:]) {
		t.Errorf("MD5 after Upload = %x, want %x", attrs.MD5, want)
	}
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
			defer b.Close()
		}
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
	}
}
