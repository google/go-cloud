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
	"net/http"
	"testing"
	"time"

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

// TestCopyPreservesDestinationCreateTime checks that copying onto an existing
// key preserves that key's original CreateTime, matching fileblob and GCS
// semantics. The bug: Copy used to assign the source blobEntry pointer directly,
// so the destination's previous CreateTime was replaced with the source's.
func TestCopyPreservesDestinationCreateTime(t *testing.T) {
	ctx := context.Background()
	b := OpenBucket(nil)
	defer b.Close()

	// Write dst first so it has an established CreateTime.
	if err := b.WriteAll(ctx, "dst", []byte("old"), &blob.WriterOptions{ContentType: "text/plain"}); err != nil {
		t.Fatal(err)
	}
	// Sleep briefly so src's CreateTime is strictly later than dst's.
	time.Sleep(2 * time.Millisecond)

	if err := b.WriteAll(ctx, "src", []byte("new"), &blob.WriterOptions{ContentType: "text/plain"}); err != nil {
		t.Fatal(err)
	}

	dstAttrsBefore, err := b.Attributes(ctx, "dst")
	if err != nil {
		t.Fatal(err)
	}
	wantCreateTime := dstAttrsBefore.CreateTime

	if err := b.Copy(ctx, "dst", "src", nil); err != nil {
		t.Fatal(err)
	}

	dstAttrsAfter, err := b.Attributes(ctx, "dst")
	if err != nil {
		t.Fatal(err)
	}
	if !dstAttrsAfter.CreateTime.Equal(wantCreateTime) {
		t.Errorf("Copy onto existing key: dst CreateTime = %v, want original %v (got src CreateTime instead)",
			dstAttrsAfter.CreateTime, wantCreateTime)
	}
}

// TestCopyDoesNotAliasEntry checks that after Copy(dst, src), the driver-level
// entries are independent pointers. The bug: Copy assigned the source *blobEntry
// pointer directly, so both keys shared the same Attributes and Metadata map.
// Deleting src and verifying dst's attributes still work catches the broken share.
func TestCopyDoesNotAliasEntry(t *testing.T) {
	ctx := context.Background()
	bkt := openBucket(nil).(*bucket)

	wopts := &driver.WriterOptions{
		CacheControl: "max-age=60",
		Metadata:     map[string]string{"k": "original"},
	}
	w, err := bkt.NewTypedWriter(ctx, "src", "text/plain", wopts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	if err := bkt.Copy(ctx, "dst", "src", &driver.CopyOptions{}); err != nil {
		t.Fatal(err)
	}

	// Directly check that src and dst do not share the same *blobEntry or
	// *driver.Attributes pointer.
	bkt.mu.Lock()
	srcEntry := bkt.blobs["src"]
	dstEntry := bkt.blobs["dst"]
	bkt.mu.Unlock()

	if srcEntry == dstEntry {
		t.Error("Copy: src and dst share the same *blobEntry pointer; mutations to one will affect the other")
	}
	if srcEntry.Attributes == dstEntry.Attributes {
		t.Error("Copy: src and dst share the same *driver.Attributes pointer")
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
