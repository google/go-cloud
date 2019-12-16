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

// Package drivertest provides a conformance test for implementations of
// driver.
package drivertest // import "gocloud.dev/blob/drivertest"

import (
	"bytes"
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gocloud.dev/blob"
	"gocloud.dev/blob/driver"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/escape"
)

// Harness descibes the functionality test harnesses must provide to run
// conformance tests.
type Harness interface {
	// MakeDriver creates a driver.Bucket to test.
	// Multiple calls to MakeDriver during a test run must refer to the
	// same storage bucket; i.e., a blob created using one driver.Bucket must
	// be readable by a subsequent driver.Bucket.
	MakeDriver(ctx context.Context) (driver.Bucket, error)
	// HTTPClient should return an unauthorized *http.Client, or nil.
	// Required if the service supports SignedURL.
	HTTPClient() *http.Client
	// Close closes resources used by the harness.
	Close()
}

// HarnessMaker describes functions that construct a harness for running tests.
// It is called exactly once per test; Harness.Close() will be called when the test is complete.
type HarnessMaker func(ctx context.Context, t *testing.T) (Harness, error)

// AsTest represents a test of As functionality.
// The conformance test:
// 1. Calls BucketCheck.
// 2. Creates a blob in a directory, using BeforeWrite as a WriterOption.
// 3. Fetches the blob's attributes and calls AttributeCheck.
// 4. Creates a Reader for the blob using BeforeReader as a ReaderOption,
//    and calls ReaderCheck with the resulting Reader.
// 5. Calls List using BeforeList as a ListOption, with Delimiter set so
//    that only the directory is returned, and calls ListObjectCheck
//    on the single directory list entry returned.
// 6. Calls List using BeforeList as a ListOption, and calls ListObjectCheck
//    on the single blob entry returned.
// 7. Tries to read a non-existent blob, and calls ErrorCheck with the error.
// 8. Makes a copy of the blob, using BeforeCopy as a CopyOption.
//
// For example, an AsTest might set a driver-specific field to a custom
// value in BeforeWrite, and then verify the custom value was returned in
// AttributesCheck and/or ReaderCheck.
type AsTest interface {
	// Name should return a descriptive name for the test.
	Name() string
	// BucketCheck will be called to allow verification of Bucket.As.
	BucketCheck(b *blob.Bucket) error
	// ErrorCheck will be called to allow verification of Bucket.ErrorAs.
	ErrorCheck(b *blob.Bucket, err error) error
	// BeforeRead will be passed directly to ReaderOptions as part of reading
	// a test blob.
	BeforeRead(as func(interface{}) bool) error
	// BeforeWrite will be passed directly to WriterOptions as part of creating
	// a test blob.
	BeforeWrite(as func(interface{}) bool) error
	// BeforeCopy will be passed directly to CopyOptions as part of copying
	// the test blob.
	BeforeCopy(as func(interface{}) bool) error
	// BeforeList will be passed directly to ListOptions as part of listing the
	// test blob.
	BeforeList(as func(interface{}) bool) error
	// AttributesCheck will be called after fetching the test blob's attributes.
	// It should call attrs.As and verify the results.
	AttributesCheck(attrs *blob.Attributes) error
	// ReaderCheck will be called after creating a blob.Reader.
	// It should call r.As and verify the results.
	ReaderCheck(r *blob.Reader) error
	// ListObjectCheck will be called after calling List with the test object's
	// name as the Prefix. It should call o.As and verify the results.
	ListObjectCheck(o *blob.ListObject) error
}

type verifyAsFailsOnNil struct{}

func (verifyAsFailsOnNil) Name() string {
	return "verify As returns false when passed nil"
}

func (verifyAsFailsOnNil) BucketCheck(b *blob.Bucket) error {
	if b.As(nil) {
		return errors.New("want Bucket.As to return false when passed nil")
	}
	return nil
}

func (verifyAsFailsOnNil) ErrorCheck(b *blob.Bucket, err error) (ret error) {
	defer func() {
		if recover() == nil {
			ret = errors.New("want ErrorAs to panic when passed nil")
		}
	}()
	b.ErrorAs(err, nil)
	return nil
}

func (verifyAsFailsOnNil) BeforeRead(as func(interface{}) bool) error {
	if as(nil) {
		return errors.New("want BeforeReader's As to return false when passed nil")
	}
	return nil
}

func (verifyAsFailsOnNil) BeforeWrite(as func(interface{}) bool) error {
	if as(nil) {
		return errors.New("want BeforeWrite's As to return false when passed nil")
	}
	return nil
}

func (verifyAsFailsOnNil) BeforeCopy(as func(interface{}) bool) error {
	if as(nil) {
		return errors.New("want BeforeCopy's As to return false when passed nil")
	}
	return nil
}

func (verifyAsFailsOnNil) BeforeList(as func(interface{}) bool) error {
	if as(nil) {
		return errors.New("want BeforeList's As to return false when passed nil")
	}
	return nil
}

func (verifyAsFailsOnNil) AttributesCheck(attrs *blob.Attributes) error {
	if attrs.As(nil) {
		return errors.New("want Attributes.As to return false when passed nil")
	}
	return nil
}

func (verifyAsFailsOnNil) ReaderCheck(r *blob.Reader) error {
	if r.As(nil) {
		return errors.New("want Reader.As to return false when passed nil")
	}
	return nil
}

func (verifyAsFailsOnNil) ListObjectCheck(o *blob.ListObject) error {
	if o.As(nil) {
		return errors.New("want ListObject.As to return false when passed nil")
	}
	return nil
}

// RunConformanceTests runs conformance tests for driver implementations of blob.
func RunConformanceTests(t *testing.T, newHarness HarnessMaker, asTests []AsTest) {
	t.Run("TestList", func(t *testing.T) {
		testList(t, newHarness)
	})
	t.Run("TestListWeirdKeys", func(t *testing.T) {
		testListWeirdKeys(t, newHarness)
	})
	t.Run("TestListDelimiters", func(t *testing.T) {
		testListDelimiters(t, newHarness)
	})
	t.Run("TestRead", func(t *testing.T) {
		testRead(t, newHarness)
	})
	t.Run("TestAttributes", func(t *testing.T) {
		testAttributes(t, newHarness)
	})
	t.Run("TestWrite", func(t *testing.T) {
		testWrite(t, newHarness)
	})
	t.Run("TestCanceledWrite", func(t *testing.T) {
		testCanceledWrite(t, newHarness)
	})
	t.Run("TestConcurrentWriteAndRead", func(t *testing.T) {
		testConcurrentWriteAndRead(t, newHarness)
	})
	t.Run("TestMetadata", func(t *testing.T) {
		testMetadata(t, newHarness)
	})
	t.Run("TestMD5", func(t *testing.T) {
		testMD5(t, newHarness)
	})
	t.Run("TestCopy", func(t *testing.T) {
		testCopy(t, newHarness)
	})
	t.Run("TestDelete", func(t *testing.T) {
		testDelete(t, newHarness)
	})
	t.Run("TestKeys", func(t *testing.T) {
		testKeys(t, newHarness)
	})
	t.Run("TestSignedURL", func(t *testing.T) {
		testSignedURL(t, newHarness)
	})
	asTests = append(asTests, verifyAsFailsOnNil{})
	t.Run("TestAs", func(t *testing.T) {
		for _, st := range asTests {
			if st.Name() == "" {
				t.Fatalf("AsTest.Name is required")
			}
			t.Run(st.Name(), func(t *testing.T) {
				testAs(t, newHarness, st)
			})
		}
	})
}

// RunBenchmarks runs benchmarks for driver implementations of blob.
func RunBenchmarks(b *testing.B, bkt *blob.Bucket) {
	b.Run("BenchmarkRead", func(b *testing.B) {
		benchmarkRead(b, bkt)
	})
	b.Run("BenchmarkWriteReadDelete", func(b *testing.B) {
		benchmarkWriteReadDelete(b, bkt)
	})
}

// testList tests the functionality of List.
func testList(t *testing.T, newHarness HarnessMaker) {
	const keyPrefix = "blob-for-list"
	content := []byte("hello")

	keyForIndex := func(i int) string { return fmt.Sprintf("%s-%d", keyPrefix, i) }
	gotIndices := func(t *testing.T, objs []*driver.ListObject) []int {
		var got []int
		for _, obj := range objs {
			if !strings.HasPrefix(obj.Key, keyPrefix) {
				t.Errorf("got name %q, expected it to have prefix %q", obj.Key, keyPrefix)
				continue
			}
			i, err := strconv.Atoi(obj.Key[len(keyPrefix)+1:])
			if err != nil {
				t.Error(err)
				continue
			}
			got = append(got, i)
		}
		return got
	}

	tests := []struct {
		name      string
		pageSize  int
		prefix    string
		wantPages [][]int
		want      []int
	}{
		{
			name:      "no objects",
			prefix:    "no-objects-with-this-prefix",
			wantPages: [][]int{nil},
		},
		{
			name:      "exactly 1 object due to prefix",
			prefix:    keyForIndex(1),
			wantPages: [][]int{{1}},
			want:      []int{1},
		},
		{
			name:      "no pagination",
			prefix:    keyPrefix,
			wantPages: [][]int{{0, 1, 2}},
			want:      []int{0, 1, 2},
		},
		{
			name:      "by 1",
			prefix:    keyPrefix,
			pageSize:  1,
			wantPages: [][]int{{0}, {1}, {2}},
			want:      []int{0, 1, 2},
		},
		{
			name:      "by 2",
			prefix:    keyPrefix,
			pageSize:  2,
			wantPages: [][]int{{0, 1}, {2}},
			want:      []int{0, 1, 2},
		},
		{
			name:      "by 3",
			prefix:    keyPrefix,
			pageSize:  3,
			wantPages: [][]int{{0, 1, 2}},
			want:      []int{0, 1, 2},
		},
	}

	ctx := context.Background()

	// Creates blobs for sub-tests below.
	// We only create the blobs once, for efficiency and because there's
	// no guarantee that after we create them they will be immediately returned
	// from List. The very first time the test is run against a Bucket, it may be
	// flaky due to this race.
	init := func(t *testing.T) (driver.Bucket, func()) {
		h, err := newHarness(ctx, t)
		if err != nil {
			t.Fatal(err)
		}
		drv, err := h.MakeDriver(ctx)
		if err != nil {
			t.Fatal(err)
		}
		// See if the blobs are already there.
		b := blob.NewBucket(drv)
		iter := b.List(&blob.ListOptions{Prefix: keyPrefix})
		found := iterToSetOfKeys(ctx, t, iter)
		for i := 0; i < 3; i++ {
			key := keyForIndex(i)
			if !found[key] {
				if err := b.WriteAll(ctx, key, content, nil); err != nil {
					b.Close()
					t.Fatal(err)
				}
			}
		}
		return drv, func() { b.Close(); h.Close() }
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			drv, done := init(t)
			defer done()

			var gotPages [][]int
			var got []int
			var nextPageToken []byte
			for {
				page, err := drv.ListPaged(ctx, &driver.ListOptions{
					PageSize:  tc.pageSize,
					Prefix:    tc.prefix,
					PageToken: nextPageToken,
				})
				if err != nil {
					t.Fatal(err)
				}
				gotThisPage := gotIndices(t, page.Objects)
				got = append(got, gotThisPage...)
				gotPages = append(gotPages, gotThisPage)
				if len(page.NextPageToken) == 0 {
					break
				}
				nextPageToken = page.NextPageToken
			}
			if diff := cmp.Diff(gotPages, tc.wantPages); diff != "" {
				t.Errorf("got\n%v\nwant\n%v\ndiff\n%s", gotPages, tc.wantPages, diff)
			}
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("got\n%v\nwant\n%v\ndiff\n%s", got, tc.want, diff)
			}
		})
	}

	// Verify pagination works when inserting in a retrieved page.
	t.Run("PaginationConsistencyAfterInsert", func(t *testing.T) {
		drv, done := init(t)
		defer done()

		// Fetch a page of 2 results: 0, 1.
		page, err := drv.ListPaged(ctx, &driver.ListOptions{
			PageSize: 2,
			Prefix:   keyPrefix,
		})
		if err != nil {
			t.Fatal(err)
		}
		got := gotIndices(t, page.Objects)
		want := []int{0, 1}
		if diff := cmp.Diff(got, want); diff != "" {
			t.Fatalf("got\n%v\nwant\n%v\ndiff\n%s", got, want, diff)
		}

		// Insert a key "0a" in the middle of the page we already retrieved.
		b := blob.NewBucket(drv)
		defer b.Close()
		key := page.Objects[0].Key + "a"
		if err := b.WriteAll(ctx, key, content, nil); err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = b.Delete(ctx, key)
		}()

		// Fetch the next page. It should not include 0, 0a, or 1, and it should
		// include 2.
		page, err = drv.ListPaged(ctx, &driver.ListOptions{
			Prefix:    keyPrefix,
			PageToken: page.NextPageToken,
		})
		if err != nil {
			t.Fatal(err)
		}
		got = gotIndices(t, page.Objects)
		want = []int{2}
		if diff := cmp.Diff(got, want); diff != "" {
			t.Errorf("got\n%v\nwant\n%v\ndiff\n%s", got, want, diff)
		}
	})

	// Verify pagination works when deleting in a retrieved page.
	t.Run("PaginationConsistencyAfterDelete", func(t *testing.T) {
		drv, done := init(t)
		defer done()

		// Fetch a page of 2 results: 0, 1.
		page, err := drv.ListPaged(ctx, &driver.ListOptions{
			PageSize: 2,
			Prefix:   keyPrefix,
		})
		if err != nil {
			t.Fatal(err)
		}
		got := gotIndices(t, page.Objects)
		want := []int{0, 1}
		if diff := cmp.Diff(got, want); diff != "" {
			t.Fatalf("got\n%v\nwant\n%v\ndiff\n%s", got, want, diff)
		}

		// Delete key "1".
		b := blob.NewBucket(drv)
		defer b.Close()
		key := page.Objects[1].Key
		if err := b.Delete(ctx, key); err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = b.WriteAll(ctx, key, content, nil)
		}()

		// Fetch the next page. It should not include 0 or 1, and it should
		// include 2.
		page, err = drv.ListPaged(ctx, &driver.ListOptions{
			Prefix:    keyPrefix,
			PageToken: page.NextPageToken,
		})
		if err != nil {
			t.Fatal(err)
		}
		got = gotIndices(t, page.Objects)
		want = []int{2}
		if diff := cmp.Diff(got, want); diff != "" {
			t.Errorf("got\n%v\nwant\n%v\ndiff\n%s", got, want, diff)
		}
	})
}

// testListWeirdKeys tests the functionality of List on weird keys.
func testListWeirdKeys(t *testing.T, newHarness HarnessMaker) {
	const keyPrefix = "list-weirdkeys-"
	content := []byte("hello")
	ctx := context.Background()

	// We're going to create a blob for each of the weird key strings, and
	// then verify we can see them with List.
	want := map[string]bool{}
	for _, k := range escape.WeirdStrings {
		want[keyPrefix+k] = true
	}

	// Creates blobs for sub-tests below.
	// We only create the blobs once, for efficiency and because there's
	// no guarantee that after we create them they will be immediately returned
	// from List. The very first time the test is run against a Bucket, it may be
	// flaky due to this race.
	init := func(t *testing.T) (*blob.Bucket, func()) {
		h, err := newHarness(ctx, t)
		if err != nil {
			t.Fatal(err)
		}
		drv, err := h.MakeDriver(ctx)
		if err != nil {
			t.Fatal(err)
		}
		// See if the blobs are already there.
		b := blob.NewBucket(drv)
		iter := b.List(&blob.ListOptions{Prefix: keyPrefix})
		found := iterToSetOfKeys(ctx, t, iter)
		for _, k := range escape.WeirdStrings {
			key := keyPrefix + k
			if !found[key] {
				if err := b.WriteAll(ctx, key, content, nil); err != nil {
					b.Close()
					t.Fatal(err)
				}
			}
		}
		return b, func() { b.Close(); h.Close() }
	}

	b, done := init(t)
	defer done()

	iter := b.List(&blob.ListOptions{Prefix: keyPrefix})
	got := iterToSetOfKeys(ctx, t, iter)

	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("got\n%v\nwant\n%v\ndiff\n%s", got, want, diff)
	}
}

// listResult is a recursive view of the hierarchy. It's used to verify List
// using Delimiter.
type listResult struct {
	Key   string
	IsDir bool
	// If IsDir is true and recursion is enabled, the recursive listing of the directory.
	Sub []listResult
}

// doList lists b using prefix and delim.
// If recurse is true, it recurses into directories filling in listResult.Sub.
func doList(ctx context.Context, b *blob.Bucket, prefix, delim string, recurse bool) ([]listResult, error) {
	iter := b.List(&blob.ListOptions{
		Prefix:    prefix,
		Delimiter: delim,
	})
	var retval []listResult
	for {
		obj, err := iter.Next(ctx)
		if err == io.EOF {
			if obj != nil {
				return nil, errors.New("obj is not nil on EOF")
			}
			break
		}
		if err != nil {
			return nil, err
		}
		var sub []listResult
		if obj.IsDir && recurse {
			sub, err = doList(ctx, b, obj.Key, delim, true)
			if err != nil {
				return nil, err
			}
		}
		retval = append(retval, listResult{
			Key:   obj.Key,
			IsDir: obj.IsDir,
			Sub:   sub,
		})
	}
	return retval, nil
}

// testListDelimiters tests the functionality of List using Delimiters.
func testListDelimiters(t *testing.T, newHarness HarnessMaker) {
	const keyPrefix = "blob-for-delimiters-"
	content := []byte("hello")

	// The set of files to use for these tests. The strings in each entry will
	// be joined using delim, so the result is a directory structure like this
	// (using / as delimiter):
	// dir1/a.txt
	// dir1/b.txt
	// dir1/subdir/c.txt
	// dir1/subdir/d.txt
	// dir2/e.txt
	// f.txt
	keys := [][]string{
		{"dir1", "a.txt"},
		{"dir1", "b.txt"},
		{"dir1", "subdir", "c.txt"},
		{"dir1", "subdir", "d.txt"},
		{"dir2", "e.txt"},
		{"f.txt"},
	}

	// Test with several different delimiters.
	tests := []struct {
		name, delim string
		// Expected result of doList with an empty delimiter.
		// All keys should be listed at the top level, with no directories.
		wantFlat []listResult
		// Expected result of doList with delimiter and recurse = true.
		// All keys should be listed, with keys in directories in the Sub field
		// of their directory.
		wantRecursive []listResult
		// Expected result of repeatedly calling driver.ListPaged with delimiter
		// and page size = 1.
		wantPaged []listResult
		// expected result of doList with delimiter and recurse = false
		// after dir2/e.txt is deleted
		// dir1/ and f.txt should be listed; dir2/ should no longer be present
		// because there are no keys in it.
		wantAfterDel []listResult
	}{
		{
			name:  "fwdslash",
			delim: "/",
			wantFlat: []listResult{
				{Key: keyPrefix + "/dir1/a.txt"},
				{Key: keyPrefix + "/dir1/b.txt"},
				{Key: keyPrefix + "/dir1/subdir/c.txt"},
				{Key: keyPrefix + "/dir1/subdir/d.txt"},
				{Key: keyPrefix + "/dir2/e.txt"},
				{Key: keyPrefix + "/f.txt"},
			},
			wantRecursive: []listResult{
				{
					Key:   keyPrefix + "/dir1/",
					IsDir: true,
					Sub: []listResult{
						{Key: keyPrefix + "/dir1/a.txt"},
						{Key: keyPrefix + "/dir1/b.txt"},
						{
							Key:   keyPrefix + "/dir1/subdir/",
							IsDir: true,
							Sub: []listResult{
								{Key: keyPrefix + "/dir1/subdir/c.txt"},
								{Key: keyPrefix + "/dir1/subdir/d.txt"},
							},
						},
					},
				},
				{
					Key:   keyPrefix + "/dir2/",
					IsDir: true,
					Sub: []listResult{
						{Key: keyPrefix + "/dir2/e.txt"},
					},
				},
				{Key: keyPrefix + "/f.txt"},
			},
			wantPaged: []listResult{
				{
					Key:   keyPrefix + "/dir1/",
					IsDir: true,
				},
				{
					Key:   keyPrefix + "/dir2/",
					IsDir: true,
				},
				{Key: keyPrefix + "/f.txt"},
			},
			wantAfterDel: []listResult{
				{
					Key:   keyPrefix + "/dir1/",
					IsDir: true,
				},
				{Key: keyPrefix + "/f.txt"},
			},
		},
		{
			name:  "backslash",
			delim: "\\",
			wantFlat: []listResult{
				{Key: keyPrefix + "\\dir1\\a.txt"},
				{Key: keyPrefix + "\\dir1\\b.txt"},
				{Key: keyPrefix + "\\dir1\\subdir\\c.txt"},
				{Key: keyPrefix + "\\dir1\\subdir\\d.txt"},
				{Key: keyPrefix + "\\dir2\\e.txt"},
				{Key: keyPrefix + "\\f.txt"},
			},
			wantRecursive: []listResult{
				{
					Key:   keyPrefix + "\\dir1\\",
					IsDir: true,
					Sub: []listResult{
						{Key: keyPrefix + "\\dir1\\a.txt"},
						{Key: keyPrefix + "\\dir1\\b.txt"},
						{
							Key:   keyPrefix + "\\dir1\\subdir\\",
							IsDir: true,
							Sub: []listResult{
								{Key: keyPrefix + "\\dir1\\subdir\\c.txt"},
								{Key: keyPrefix + "\\dir1\\subdir\\d.txt"},
							},
						},
					},
				},
				{
					Key:   keyPrefix + "\\dir2\\",
					IsDir: true,
					Sub: []listResult{
						{Key: keyPrefix + "\\dir2\\e.txt"},
					},
				},
				{Key: keyPrefix + "\\f.txt"},
			},
			wantPaged: []listResult{
				{
					Key:   keyPrefix + "\\dir1\\",
					IsDir: true,
				},
				{
					Key:   keyPrefix + "\\dir2\\",
					IsDir: true,
				},
				{Key: keyPrefix + "\\f.txt"},
			},
			wantAfterDel: []listResult{
				{
					Key:   keyPrefix + "\\dir1\\",
					IsDir: true,
				},
				{Key: keyPrefix + "\\f.txt"},
			},
		},
		{
			name:  "abc",
			delim: "abc",
			wantFlat: []listResult{
				{Key: keyPrefix + "abcdir1abca.txt"},
				{Key: keyPrefix + "abcdir1abcb.txt"},
				{Key: keyPrefix + "abcdir1abcsubdirabcc.txt"},
				{Key: keyPrefix + "abcdir1abcsubdirabcd.txt"},
				{Key: keyPrefix + "abcdir2abce.txt"},
				{Key: keyPrefix + "abcf.txt"},
			},
			wantRecursive: []listResult{
				{
					Key:   keyPrefix + "abcdir1abc",
					IsDir: true,
					Sub: []listResult{
						{Key: keyPrefix + "abcdir1abca.txt"},
						{Key: keyPrefix + "abcdir1abcb.txt"},
						{
							Key:   keyPrefix + "abcdir1abcsubdirabc",
							IsDir: true,
							Sub: []listResult{
								{Key: keyPrefix + "abcdir1abcsubdirabcc.txt"},
								{Key: keyPrefix + "abcdir1abcsubdirabcd.txt"},
							},
						},
					},
				},
				{
					Key:   keyPrefix + "abcdir2abc",
					IsDir: true,
					Sub: []listResult{
						{Key: keyPrefix + "abcdir2abce.txt"},
					},
				},
				{Key: keyPrefix + "abcf.txt"},
			},
			wantPaged: []listResult{
				{
					Key:   keyPrefix + "abcdir1abc",
					IsDir: true,
				},
				{
					Key:   keyPrefix + "abcdir2abc",
					IsDir: true,
				},
				{Key: keyPrefix + "abcf.txt"},
			},
			wantAfterDel: []listResult{
				{
					Key:   keyPrefix + "abcdir1abc",
					IsDir: true,
				},
				{Key: keyPrefix + "abcf.txt"},
			},
		},
	}

	ctx := context.Background()

	// Creates blobs for sub-tests below.
	// We only create the blobs once, for efficiency and because there's
	// no guarantee that after we create them they will be immediately returned
	// from List. The very first time the test is run against a Bucket, it may be
	// flaky due to this race.
	init := func(t *testing.T, delim string) (driver.Bucket, *blob.Bucket, func()) {
		h, err := newHarness(ctx, t)
		if err != nil {
			t.Fatal(err)
		}
		drv, err := h.MakeDriver(ctx)
		if err != nil {
			t.Fatal(err)
		}
		b := blob.NewBucket(drv)

		// See if the blobs are already there.
		prefix := keyPrefix + delim
		iter := b.List(&blob.ListOptions{Prefix: prefix})
		found := iterToSetOfKeys(ctx, t, iter)
		for _, keyParts := range keys {
			key := prefix + strings.Join(keyParts, delim)
			if !found[key] {
				if err := b.WriteAll(ctx, key, content, nil); err != nil {
					b.Close()
					t.Fatal(err)
				}
			}
		}
		return drv, b, func() { b.Close(); h.Close() }
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			drv, b, done := init(t, tc.delim)
			defer done()

			// Fetch without using delimiter.
			got, err := doList(ctx, b, keyPrefix+tc.delim, "", true)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(got, tc.wantFlat); diff != "" {
				t.Errorf("with no delimiter, got\n%v\nwant\n%v\ndiff\n%s", got, tc.wantFlat, diff)
			}

			// Fetch using delimiter, recursively.
			got, err = doList(ctx, b, keyPrefix+tc.delim, tc.delim, true)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(got, tc.wantRecursive); diff != "" {
				t.Errorf("with delimiter, got\n%v\nwant\n%v\ndiff\n%s", got, tc.wantRecursive, diff)
			}

			// Test pagination via driver.ListPaged.
			var nextPageToken []byte
			got = nil
			for {
				page, err := drv.ListPaged(ctx, &driver.ListOptions{
					Prefix:    keyPrefix + tc.delim,
					Delimiter: tc.delim,
					PageSize:  1,
					PageToken: nextPageToken,
				})
				if err != nil {
					t.Fatal(err)
				}
				if len(page.Objects) > 1 {
					t.Errorf("got %d objects on a page, want 0 or 1", len(page.Objects))
				}
				for _, obj := range page.Objects {
					got = append(got, listResult{
						Key:   obj.Key,
						IsDir: obj.IsDir,
					})
				}
				if len(page.NextPageToken) == 0 {
					break
				}
				nextPageToken = page.NextPageToken
			}
			if diff := cmp.Diff(got, tc.wantPaged); diff != "" {
				t.Errorf("paged got\n%v\nwant\n%v\ndiff\n%s", got, tc.wantPaged, diff)
			}

			// Delete dir2/e.txt and verify that dir2/ is no longer returned.
			key := strings.Join(append([]string{keyPrefix}, "dir2", "e.txt"), tc.delim)
			if err := b.Delete(ctx, key); err != nil {
				t.Fatal(err)
			}
			// Attempt to restore dir2/e.txt at the end of the test for the next run.
			defer func() {
				_ = b.WriteAll(ctx, key, content, nil)
			}()

			got, err = doList(ctx, b, keyPrefix+tc.delim, tc.delim, false)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(got, tc.wantAfterDel); diff != "" {
				t.Errorf("after delete, got\n%v\nwant\n%v\ndiff\n%s", got, tc.wantAfterDel, diff)
			}
		})
	}
}

func iterToSetOfKeys(ctx context.Context, t *testing.T, iter *blob.ListIterator) map[string]bool {
	retval := map[string]bool{}
	for {
		if item, err := iter.Next(ctx); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		} else {
			retval[item.Key] = true
		}
	}
	return retval
}

// testRead tests the functionality of NewReader, NewRangeReader, and Reader.
func testRead(t *testing.T, newHarness HarnessMaker) {
	const key = "blob-for-reading"
	content := []byte("abcdefghijklmnopqurstuvwxyz")
	contentSize := int64(len(content))

	tests := []struct {
		name           string
		key            string
		offset, length int64
		want           []byte
		wantReadSize   int64
		wantErr        bool
		// set to true to skip creation of the object for
		// tests where we expect an error without any actual
		// read.
		skipCreate bool
	}{
		{
			name:    "read of nonexistent key fails",
			key:     "key-does-not-exist",
			length:  -1,
			wantErr: true,
		},
		{
			name:       "negative offset fails",
			key:        key,
			offset:     -1,
			wantErr:    true,
			skipCreate: true,
		},
		{
			name: "length 0 read",
			key:  key,
			want: []byte{},
		},
		{
			name:         "read from positive offset to end",
			key:          key,
			offset:       10,
			length:       -1,
			want:         content[10:],
			wantReadSize: contentSize - 10,
		},
		{
			name:         "read a part in middle",
			key:          key,
			offset:       10,
			length:       5,
			want:         content[10:15],
			wantReadSize: 5,
		},
		{
			name:         "read in full",
			key:          key,
			length:       -1,
			want:         content,
			wantReadSize: contentSize,
		},
		{
			name:         "read in full with negative length not -1",
			key:          key,
			length:       -42,
			want:         content,
			wantReadSize: contentSize,
		},
	}

	ctx := context.Background()

	// Creates a blob for sub-tests below.
	init := func(t *testing.T, skipCreate bool) (*blob.Bucket, func()) {
		h, err := newHarness(ctx, t)
		if err != nil {
			t.Fatal(err)
		}

		drv, err := h.MakeDriver(ctx)
		if err != nil {
			t.Fatal(err)
		}
		b := blob.NewBucket(drv)
		if skipCreate {
			return b, func() { b.Close(); h.Close() }
		}
		if err := b.WriteAll(ctx, key, content, nil); err != nil {
			b.Close()
			t.Fatal(err)
		}
		return b, func() {
			_ = b.Delete(ctx, key)
			b.Close()
			h.Close()
		}
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, done := init(t, tc.skipCreate)
			defer done()

			r, err := b.NewRangeReader(ctx, tc.key, tc.offset, tc.length, nil)
			if (err != nil) != tc.wantErr {
				t.Errorf("got err %v want error %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			defer r.Close()
			// Make the buffer bigger than needed to make sure we actually only read
			// the expected number of bytes.
			got := make([]byte, tc.wantReadSize+10)
			n, err := r.Read(got)
			// EOF error is optional, see https://golang.org/pkg/io/#Reader.
			if err != nil && err != io.EOF {
				t.Errorf("unexpected error during read: %v", err)
			}
			if int64(n) != tc.wantReadSize {
				t.Errorf("got read length %d want %d", n, tc.wantReadSize)
			}
			if !cmp.Equal(got[:tc.wantReadSize], tc.want) {
				t.Errorf("got %q want %q", string(got), string(tc.want))
			}
			if r.Size() != contentSize {
				t.Errorf("got size %d want %d", r.Size(), contentSize)
			}
			if r.ModTime().IsZero() {
				t.Errorf("got zero mod time, want non-zero")
			}
		})
	}
}

// testAttributes tests Attributes.
func testAttributes(t *testing.T, newHarness HarnessMaker) {
	const (
		dirKey             = "someDir"
		key                = dirKey + "/blob-for-attributes"
		contentType        = "text/plain"
		cacheControl       = "no-cache"
		contentDisposition = "inline"
		contentEncoding    = "identity"
		contentLanguage    = "en"
	)
	content := []byte("Hello World!")

	ctx := context.Background()

	// Creates a blob for sub-tests below.
	init := func(t *testing.T) (*blob.Bucket, func()) {
		h, err := newHarness(ctx, t)
		if err != nil {
			t.Fatal(err)
		}
		drv, err := h.MakeDriver(ctx)
		if err != nil {
			t.Fatal(err)
		}
		b := blob.NewBucket(drv)
		opts := &blob.WriterOptions{
			ContentType:        contentType,
			CacheControl:       cacheControl,
			ContentDisposition: contentDisposition,
			ContentEncoding:    contentEncoding,
			ContentLanguage:    contentLanguage,
		}
		if err := b.WriteAll(ctx, key, content, opts); err != nil {
			b.Close()
			t.Fatal(err)
		}
		return b, func() {
			_ = b.Delete(ctx, key)
			b.Close()
			h.Close()
		}
	}

	b, done := init(t)
	defer done()

	for _, badKey := range []string{
		"not-found",
		dirKey,
		dirKey + "/",
	} {
		_, err := b.Attributes(ctx, badKey)
		if err == nil {
			t.Errorf("got nil want error")
		} else if gcerrors.Code(err) != gcerrors.NotFound {
			t.Errorf("got %v want NotFound error", err)
		} else if !strings.Contains(err.Error(), badKey) {
			t.Errorf("got %v want error to include missing key", err)
		}
	}

	a, err := b.Attributes(ctx, key)
	if err != nil {
		t.Fatalf("failed Attributes: %v", err)
	}
	// Also make a Reader so we can verify the subset of attributes
	// that it exposes.
	r, err := b.NewReader(ctx, key, nil)
	if err != nil {
		t.Fatalf("failed Attributes: %v", err)
	}
	if a.CacheControl != cacheControl {
		t.Errorf("got CacheControl %q want %q", a.CacheControl, cacheControl)
	}
	if a.ContentDisposition != contentDisposition {
		t.Errorf("got ContentDisposition %q want %q", a.ContentDisposition, contentDisposition)
	}
	if a.ContentEncoding != contentEncoding {
		t.Errorf("got ContentEncoding %q want %q", a.ContentEncoding, contentEncoding)
	}
	if a.ContentLanguage != contentLanguage {
		t.Errorf("got ContentLanguage %q want %q", a.ContentLanguage, contentLanguage)
	}
	if a.ContentType != contentType {
		t.Errorf("got ContentType %q want %q", a.ContentType, contentType)
	}
	if r.ContentType() != contentType {
		t.Errorf("got Reader.ContentType() %q want %q", r.ContentType(), contentType)
	}
	if a.Size != int64(len(content)) {
		t.Errorf("got Size %d want %d", a.Size, len(content))
	}
	if r.Size() != int64(len(content)) {
		t.Errorf("got Reader.Size() %d want %d", r.Size(), len(content))
	}
	r.Close()

	t1 := a.ModTime
	if err := b.WriteAll(ctx, key, content, nil); err != nil {
		t.Fatal(err)
	}
	a2, err := b.Attributes(ctx, key)
	if err != nil {
		t.Errorf("failed Attributes#2: %v", err)
	}
	t2 := a2.ModTime
	if t2.Before(t1) {
		t.Errorf("ModTime %v is before %v", t2, t1)
	}
}

// loadTestData loads test data, inlined using go-bindata.
func loadTestData(t testing.TB, name string) []byte {
	data, err := Asset(name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// testWrite tests the functionality of NewWriter and Writer.
func testWrite(t *testing.T, newHarness HarnessMaker) {
	const key = "blob-for-reading"
	const existingContent = "existing content"
	smallText := loadTestData(t, "test-small.txt")
	mediumHTML := loadTestData(t, "test-medium.html")
	largeJpg := loadTestData(t, "test-large.jpg")
	helloWorld := []byte("hello world")
	helloWorldMD5 := md5.Sum(helloWorld)

	tests := []struct {
		name            string
		key             string
		exists          bool
		content         []byte
		contentType     string
		contentMD5      []byte
		firstChunk      int
		wantContentType string
		wantErr         bool
		wantReadErr     bool // if wantErr is true, and Read after err should fail with something other than NotExists
	}{
		{
			name:        "write to empty key fails",
			wantErr:     true,
			wantReadErr: true, // read from empty key fails, but not always with NotExists
		},
		{
			name: "no write then close results in empty blob",
			key:  key,
		},
		{
			name: "no write then close results in empty blob, blob existed",
			key:  key,
		},
		{
			name:        "invalid ContentType fails",
			key:         key,
			contentType: "application/octet/stream",
			wantErr:     true,
		},
		{
			name:            "ContentType is discovered if not provided",
			key:             key,
			content:         mediumHTML,
			wantContentType: "text/html",
		},
		{
			name:            "write with explicit ContentType overrides discovery",
			key:             key,
			content:         mediumHTML,
			contentType:     "application/json",
			wantContentType: "application/json",
		},
		{
			name:       "Content md5 match",
			key:        key,
			content:    helloWorld,
			contentMD5: helloWorldMD5[:],
		},
		{
			name:       "Content md5 did not match",
			key:        key,
			content:    []byte("not hello world"),
			contentMD5: helloWorldMD5[:],
			wantErr:    true,
		},
		{
			name:       "Content md5 did not match, blob existed",
			exists:     true,
			key:        key,
			content:    []byte("not hello world"),
			contentMD5: helloWorldMD5[:],
			wantErr:    true,
		},
		{
			name:            "a small text file",
			key:             key,
			content:         smallText,
			wantContentType: "text/html",
		},
		{
			name:            "a large jpg file",
			key:             key,
			content:         largeJpg,
			wantContentType: "image/jpg",
		},
		{
			name:            "a large jpg file written in two chunks",
			key:             key,
			firstChunk:      10,
			content:         largeJpg,
			wantContentType: "image/jpg",
		},
		// TODO(issue #304): Fails for GCS.
		/*
			{
				name:            "ContentType is parsed and reformatted",
				key:             key,
				content:         []byte("foo"),
				contentType:     `FORM-DATA;name="foo"`,
				wantContentType: `form-data; name=foo`,
			},
		*/
	}

	ctx := context.Background()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, err := newHarness(ctx, t)
			if err != nil {
				t.Fatal(err)
			}
			defer h.Close()
			drv, err := h.MakeDriver(ctx)
			if err != nil {
				t.Fatal(err)
			}
			b := blob.NewBucket(drv)
			defer b.Close()

			// If the test wants the blob to already exist, write it.
			if tc.exists {
				if err := b.WriteAll(ctx, key, []byte(existingContent), nil); err != nil {
					t.Fatal(err)
				}
				defer func() {
					_ = b.Delete(ctx, key)
				}()
			}

			// Write the content.
			opts := &blob.WriterOptions{
				ContentType: tc.contentType,
				ContentMD5:  tc.contentMD5[:],
			}
			w, err := b.NewWriter(ctx, tc.key, opts)
			if err == nil {
				if len(tc.content) > 0 {
					if tc.firstChunk == 0 {
						// Write the whole thing.
						_, err = w.Write(tc.content)
					} else {
						// Write it in 2 chunks.
						_, err = w.Write(tc.content[:tc.firstChunk])
						if err == nil {
							_, err = w.Write(tc.content[tc.firstChunk:])
						}
					}
				}
				if err == nil {
					err = w.Close()
				}
			}
			if (err != nil) != tc.wantErr {
				t.Errorf("NewWriter or Close got err %v want error %v", err, tc.wantErr)
			}
			if err != nil {
				// The write failed; verify that it had no effect.
				buf, err := b.ReadAll(ctx, tc.key)
				if tc.exists {
					// Verify the previous content is still there.
					if !bytes.Equal(buf, []byte(existingContent)) {
						t.Errorf("Write failed as expected, but content doesn't match expected previous content; got \n%s\n want \n%s", string(buf), existingContent)
					}
				} else {
					// Verify that the read fails with NotFound.
					if err == nil {
						t.Error("Write failed as expected, but Read after that didn't return an error")
					} else if !tc.wantReadErr && gcerrors.Code(err) != gcerrors.NotFound {
						t.Errorf("Write failed as expected, but Read after that didn't return the right error; got %v want NotFound", err)
					} else if !strings.Contains(err.Error(), tc.key) {
						t.Errorf("got %v want error to include missing key", err)
					}
				}
				return
			}
			defer func() { _ = b.Delete(ctx, tc.key) }()

			// Read it back.
			buf, err := b.ReadAll(ctx, tc.key)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(buf, tc.content) {
				if len(buf) < 100 && len(tc.content) < 100 {
					t.Errorf("read didn't match write; got \n%s\n want \n%s", string(buf), string(tc.content))
				} else {
					t.Error("read didn't match write, content too large to display")
				}
			}
		})
	}
}

// testCanceledWrite tests the functionality of canceling an in-progress write.
func testCanceledWrite(t *testing.T, newHarness HarnessMaker) {
	const key = "blob-for-canceled-write"
	content := []byte("hello world")
	cancelContent := []byte("going to cancel")

	tests := []struct {
		description string
		contentType string
		exists      bool
	}{
		{
			// The write will be buffered in the portable type as part of
			// ContentType detection, so the first call to the Driver will be Close.
			description: "EmptyContentType",
		},
		{
			// The write will be sent to the Driver, which may do its own
			// internal buffering.
			description: "NonEmptyContentType",
			contentType: "text/plain",
		},
		{
			description: "BlobExists",
			exists:      true,
		},
		// TODO(issue #482): Find a way to test that a chunked upload that's interrupted
		// after some chunks are uploaded cancels correctly.
	}

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			cancelCtx, cancel := context.WithCancel(ctx)
			h, err := newHarness(ctx, t)
			if err != nil {
				t.Fatal(err)
			}
			defer h.Close()
			drv, err := h.MakeDriver(ctx)
			if err != nil {
				t.Fatal(err)
			}
			b := blob.NewBucket(drv)
			defer b.Close()

			opts := &blob.WriterOptions{
				ContentType: test.contentType,
			}
			// If the test wants the blob to already exist, write it.
			if test.exists {
				if err := b.WriteAll(ctx, key, content, opts); err != nil {
					t.Fatal(err)
				}
				defer func() {
					_ = b.Delete(ctx, key)
				}()
			}

			// Create a writer with the context that we're going
			// to cancel.
			w, err := b.NewWriter(cancelCtx, key, opts)
			if err != nil {
				t.Fatal(err)
			}
			// Write the content.
			if _, err := w.Write(cancelContent); err != nil {
				t.Fatal(err)
			}

			// Verify that the previous content (if any) is still readable,
			// because the write hasn't been Closed yet.
			got, err := b.ReadAll(ctx, key)
			if test.exists {
				// The previous content should still be there.
				if !cmp.Equal(got, content) {
					t.Errorf("during unclosed write, got %q want %q", string(got), string(content))
				}
			} else {
				// The read should fail; the write hasn't been Closed so the
				// blob shouldn't exist.
				if err == nil {
					t.Error("wanted read to return an error when write is not yet Closed")
				}
			}

			// Cancel the context to abort the write.
			cancel()
			// Close should return some kind of canceled context error.
			// We can't verify the kind of error cleanly, so we just verify there's
			// an error.
			if err := w.Close(); err == nil {
				t.Errorf("got Close error %v want canceled ctx error", err)
			}

			// Verify the write was truly aborted.
			got, err = b.ReadAll(ctx, key)
			if test.exists {
				// The previous content should still be there.
				if !cmp.Equal(got, content) {
					t.Errorf("after canceled write, got %q want %q", string(got), string(content))
				}
			} else {
				// The read should fail; the write was aborted so the
				// blob shouldn't exist.
				if err == nil {
					t.Error("wanted read to return an error when write was canceled")
				}
			}
		})
	}
}

// testMetadata tests writing and reading the key/value metadata for a blob.
func testMetadata(t *testing.T, newHarness HarnessMaker) {
	const key = "blob-for-metadata"
	hello := []byte("hello")

	weirdMetadata := map[string]string{}
	for _, k := range escape.WeirdStrings {
		weirdMetadata[k] = k
	}

	tests := []struct {
		name        string
		metadata    map[string]string
		content     []byte
		contentType string
		want        map[string]string
		wantErr     bool
	}{
		{
			name:     "empty",
			content:  hello,
			metadata: map[string]string{},
			want:     nil,
		},
		{
			name:     "empty key fails",
			content:  hello,
			metadata: map[string]string{"": "empty key value"},
			wantErr:  true,
		},
		{
			name:     "duplicate case-insensitive key fails",
			content:  hello,
			metadata: map[string]string{"abc": "foo", "aBc": "bar"},
			wantErr:  true,
		},
		{
			name:    "valid metadata",
			content: hello,
			metadata: map[string]string{
				"key_a": "value-a",
				"kEy_B": "value-b",
				"key_c": "vAlUe-c",
			},
			want: map[string]string{
				"key_a": "value-a",
				"key_b": "value-b",
				"key_c": "vAlUe-c",
			},
		},
		{
			name:     "valid metadata with empty body",
			content:  nil,
			metadata: map[string]string{"foo": "bar"},
			want:     map[string]string{"foo": "bar"},
		},
		{
			name:        "valid metadata with content type",
			content:     hello,
			contentType: "text/plain",
			metadata:    map[string]string{"foo": "bar"},
			want:        map[string]string{"foo": "bar"},
		},
		{
			name:     "weird metadata keys",
			content:  hello,
			metadata: weirdMetadata,
			want:     weirdMetadata,
		},
		{
			name:     "non-utf8 metadata key",
			content:  hello,
			metadata: map[string]string{escape.NonUTF8String: "bar"},
			wantErr:  true,
		},
		{
			name:     "non-utf8 metadata value",
			content:  hello,
			metadata: map[string]string{"foo": escape.NonUTF8String},
			wantErr:  true,
		},
	}

	ctx := context.Background()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, err := newHarness(ctx, t)
			if err != nil {
				t.Fatal(err)
			}
			defer h.Close()

			drv, err := h.MakeDriver(ctx)
			if err != nil {
				t.Fatal(err)
			}
			b := blob.NewBucket(drv)
			defer b.Close()
			opts := &blob.WriterOptions{
				Metadata:    tc.metadata,
				ContentType: tc.contentType,
			}
			err = b.WriteAll(ctx, key, hello, opts)
			if (err != nil) != tc.wantErr {
				t.Errorf("got error %v want error %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			defer func() {
				_ = b.Delete(ctx, key)
			}()
			a, err := b.Attributes(ctx, key)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(a.Metadata, tc.want); diff != "" {
				t.Errorf("got\n%v\nwant\n%v\ndiff\n%s", a.Metadata, tc.want, diff)
			}
		})
	}
}

// testMD5 tests reading MD5 hashes via List and Attributes.
func testMD5(t *testing.T, newHarness HarnessMaker) {
	ctx := context.Background()

	// Define two blobs with different content; we'll write them and then verify
	// their returned MD5 hashes.
	const aKey, bKey = "blob-for-md5-aaa", "blob-for-md5-bbb"
	aContent, bContent := []byte("hello"), []byte("goodbye")
	aMD5 := md5.Sum(aContent)
	bMD5 := md5.Sum(bContent)

	h, err := newHarness(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	drv, err := h.MakeDriver(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b := blob.NewBucket(drv)
	defer b.Close()

	// Write the two blobs.
	if err := b.WriteAll(ctx, aKey, aContent, nil); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Delete(ctx, aKey) }()
	if err := b.WriteAll(ctx, bKey, bContent, nil); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Delete(ctx, bKey) }()

	// Check the MD5 we get through Attributes. Note that it's always legal to
	// return a nil MD5.
	aAttr, err := b.Attributes(ctx, aKey)
	if err != nil {
		t.Fatal(err)
	}
	if aAttr.MD5 != nil && !bytes.Equal(aAttr.MD5, aMD5[:]) {
		t.Errorf("got MD5\n%x\nwant\n%x", aAttr.MD5, aMD5)
	}

	bAttr, err := b.Attributes(ctx, bKey)
	if err != nil {
		t.Fatal(err)
	}
	if bAttr.MD5 != nil && !bytes.Equal(bAttr.MD5, bMD5[:]) {
		t.Errorf("got MD5\n%x\nwant\n%x", bAttr.MD5, bMD5)
	}

	// Check the MD5 we get through List. Note that it's always legal to
	// return a nil MD5.
	iter := b.List(&blob.ListOptions{Prefix: "blob-for-md5-"})
	obj, err := iter.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if obj.Key != aKey {
		t.Errorf("got name %q want %q", obj.Key, aKey)
	}
	if obj.MD5 != nil && !bytes.Equal(obj.MD5, aMD5[:]) {
		t.Errorf("got MD5\n%x\nwant\n%x", obj.MD5, aMD5)
	}
	obj, err = iter.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if obj.Key != bKey {
		t.Errorf("got name %q want %q", obj.Key, bKey)
	}
	if obj.MD5 != nil && !bytes.Equal(obj.MD5, bMD5[:]) {
		t.Errorf("got MD5\n%x\nwant\n%x", obj.MD5, bMD5)
	}
}

// testCopy tests the functionality of Copy.
func testCopy(t *testing.T, newHarness HarnessMaker) {
	const (
		srcKey             = "blob-for-copying-src"
		dstKey             = "blob-for-copying-dest"
		dstKeyExists       = "blob-for-copying-dest-exists"
		contentType        = "text/plain"
		cacheControl       = "no-cache"
		contentDisposition = "inline"
		contentEncoding    = "identity"
		contentLanguage    = "en"
	)
	var contents = []byte("Hello World")

	ctx := context.Background()
	t.Run("NonExistentSourceFails", func(t *testing.T) {
		h, err := newHarness(ctx, t)
		if err != nil {
			t.Fatal(err)
		}
		defer h.Close()
		drv, err := h.MakeDriver(ctx)
		if err != nil {
			t.Fatal(err)
		}
		b := blob.NewBucket(drv)
		defer b.Close()

		err = b.Copy(ctx, dstKey, "does-not-exist", nil)
		if err == nil {
			t.Errorf("got nil want error")
		} else if gcerrors.Code(err) != gcerrors.NotFound {
			t.Errorf("got %v want NotFound error", err)
		} else if !strings.Contains(err.Error(), "does-not-exist") {
			t.Errorf("got %v want error to include missing key", err)
		}
	})

	t.Run("Works", func(t *testing.T) {
		h, err := newHarness(ctx, t)
		if err != nil {
			t.Fatal(err)
		}
		defer h.Close()
		drv, err := h.MakeDriver(ctx)
		if err != nil {
			t.Fatal(err)
		}
		b := blob.NewBucket(drv)
		defer b.Close()

		// Create the source blob.
		wopts := &blob.WriterOptions{
			ContentType:        contentType,
			CacheControl:       cacheControl,
			ContentDisposition: contentDisposition,
			ContentEncoding:    contentEncoding,
			ContentLanguage:    contentLanguage,
			Metadata:           map[string]string{"foo": "bar"},
		}
		if err := b.WriteAll(ctx, srcKey, contents, wopts); err != nil {
			t.Fatal(err)
		}

		// Grab its attributes to compare to the copy's attributes later.
		wantAttr, err := b.Attributes(ctx, srcKey)
		if err != nil {
			t.Fatal(err)
		}
		wantAttr.ModTime = time.Time{} // don't compare this field

		// Create another blob that we're going to overwrite.
		if err := b.WriteAll(ctx, dstKeyExists, []byte("clobber me"), nil); err != nil {
			t.Fatal(err)
		}

		// Copy the source to the destination.
		if err := b.Copy(ctx, dstKey, srcKey, nil); err != nil {
			t.Errorf("got unexpected error copying blob: %v", err)
		}
		// Read the copy.
		got, err := b.ReadAll(ctx, dstKey)
		if err != nil {
			t.Fatal(err)
		}
		if !cmp.Equal(got, contents) {
			t.Errorf("got %q want %q", string(got), string(contents))
		}
		// Verify attributes of the copy.
		gotAttr, err := b.Attributes(ctx, dstKey)
		if err != nil {
			t.Fatal(err)
		}
		gotAttr.ModTime = time.Time{} // don't compare this field
		if diff := cmp.Diff(gotAttr, wantAttr, cmpopts.IgnoreUnexported(blob.Attributes{})); diff != "" {
			t.Errorf("got %v want %v diff %s", gotAttr, wantAttr, diff)
		}

		// Copy the source to the second destination, where there's an existing blob.
		// It should be overwritten.
		if err := b.Copy(ctx, dstKeyExists, srcKey, nil); err != nil {
			t.Errorf("got unexpected error copying blob: %v", err)
		}
		// Read the copy.
		got, err = b.ReadAll(ctx, dstKeyExists)
		if err != nil {
			t.Fatal(err)
		}
		if !cmp.Equal(got, contents) {
			t.Errorf("got %q want %q", string(got), string(contents))
		}
		// Verify attributes of the copy.
		gotAttr, err = b.Attributes(ctx, dstKeyExists)
		if err != nil {
			t.Fatal(err)
		}
		gotAttr.ModTime = time.Time{} // don't compare this field
		if diff := cmp.Diff(gotAttr, wantAttr, cmpopts.IgnoreUnexported(blob.Attributes{})); diff != "" {
			t.Errorf("got %v want %v diff %s", gotAttr, wantAttr, diff)
		}
	})
}

// testDelete tests the functionality of Delete.
func testDelete(t *testing.T, newHarness HarnessMaker) {
	const key = "blob-for-deleting"

	ctx := context.Background()
	t.Run("NonExistentFails", func(t *testing.T) {
		h, err := newHarness(ctx, t)
		if err != nil {
			t.Fatal(err)
		}
		defer h.Close()
		drv, err := h.MakeDriver(ctx)
		if err != nil {
			t.Fatal(err)
		}
		b := blob.NewBucket(drv)
		defer b.Close()

		err = b.Delete(ctx, "does-not-exist")
		if err == nil {
			t.Errorf("got nil want error")
		} else if gcerrors.Code(err) != gcerrors.NotFound {
			t.Errorf("got %v want NotFound error", err)
		} else if !strings.Contains(err.Error(), "does-not-exist") {
			t.Errorf("got %v want error to include missing key", err)
		}
	})

	t.Run("Works", func(t *testing.T) {
		h, err := newHarness(ctx, t)
		if err != nil {
			t.Fatal(err)
		}
		defer h.Close()
		drv, err := h.MakeDriver(ctx)
		if err != nil {
			t.Fatal(err)
		}
		b := blob.NewBucket(drv)
		defer b.Close()

		// Create the blob.
		if err := b.WriteAll(ctx, key, []byte("Hello world"), nil); err != nil {
			t.Fatal(err)
		}
		// Delete it.
		if err := b.Delete(ctx, key); err != nil {
			t.Errorf("got unexpected error deleting blob: %v", err)
		}
		// Subsequent read fails with NotFound.
		_, err = b.NewReader(ctx, key, nil)
		if err == nil {
			t.Errorf("read after delete got nil, want error")
		} else if gcerrors.Code(err) != gcerrors.NotFound {
			t.Errorf("read after delete want NotFound error, got %v", err)
		} else if !strings.Contains(err.Error(), key) {
			t.Errorf("got %v want error to include missing key", err)
		}
		// Subsequent delete also fails.
		err = b.Delete(ctx, key)
		if err == nil {
			t.Errorf("delete after delete got nil, want error")
		} else if gcerrors.Code(err) != gcerrors.NotFound {
			t.Errorf("delete after delete got %v, want NotFound error", err)
		} else if !strings.Contains(err.Error(), key) {
			t.Errorf("got %v want error to include missing key", err)
		}
	})
}

// testConcurrentWriteAndRead tests that concurrent writing to multiple blob
// keys and concurrent reading from multiple blob keys works.
func testConcurrentWriteAndRead(t *testing.T, newHarness HarnessMaker) {
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
	b := blob.NewBucket(drv)
	defer b.Close()

	// Prepare data. Each of the numKeys blobs has dataSize bytes, with each byte
	// set to the numeric key index. For example, the blob at "key0" consists of
	// all dataSize bytes set to 0.
	const numKeys = 20
	const dataSize = 4 * 1024
	keyData := make(map[int][]byte)
	for k := 0; k < numKeys; k++ {
		data := make([]byte, dataSize)
		for i := 0; i < dataSize; i++ {
			data[i] = byte(k)
		}
		keyData[k] = data
	}

	blobName := func(k int) string {
		return fmt.Sprintf("key%d", k)
	}

	var wg sync.WaitGroup

	// Write all blobs concurrently.
	for k := 0; k < numKeys; k++ {
		wg.Add(1)
		go func(key int) {
			if err := b.WriteAll(ctx, blobName(key), keyData[key], nil); err != nil {
				t.Fatal(err)
			}
			wg.Done()
		}(k)
		defer b.Delete(ctx, blobName(k))
	}
	wg.Wait()

	// Read all blobs concurrently and verify that they contain the expected data.
	for k := 0; k < numKeys; k++ {
		wg.Add(1)
		go func(key int) {
			buf, err := b.ReadAll(ctx, blobName(key))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(buf, keyData[key]) {
				t.Errorf("read data mismatch for key %d", key)
			}
			wg.Done()
		}(k)
	}
	wg.Wait()
}

// testKeys tests a variety of weird keys.
func testKeys(t *testing.T, newHarness HarnessMaker) {
	const keyPrefix = "weird-keys"
	content := []byte("hello")
	ctx := context.Background()

	t.Run("non-UTF8 fails", func(t *testing.T) {
		h, err := newHarness(ctx, t)
		if err != nil {
			t.Fatal(err)
		}
		defer h.Close()
		drv, err := h.MakeDriver(ctx)
		if err != nil {
			t.Fatal(err)
		}
		b := blob.NewBucket(drv)
		defer b.Close()

		// Write the blob.
		key := keyPrefix + escape.NonUTF8String
		if err := b.WriteAll(ctx, key, content, nil); err == nil {
			t.Error("got nil error, expected error for using non-UTF8 string as key")
		}
	})

	for description, key := range escape.WeirdStrings {
		t.Run(description, func(t *testing.T) {
			h, err := newHarness(ctx, t)
			if err != nil {
				t.Fatal(err)
			}
			defer h.Close()
			drv, err := h.MakeDriver(ctx)
			if err != nil {
				t.Fatal(err)
			}
			b := blob.NewBucket(drv)
			defer b.Close()

			// Write the blob.
			key = keyPrefix + key
			if err := b.WriteAll(ctx, key, content, nil); err != nil {
				t.Fatal(err)
			}

			defer func() {
				err := b.Delete(ctx, key)
				if err != nil {
					t.Error(err)
				}
			}()

			// Verify read works.
			got, err := b.ReadAll(ctx, key)
			if err != nil {
				t.Fatal(err)
			}
			if !cmp.Equal(got, content) {
				t.Errorf("got %q want %q", string(got), string(content))
			}

			// Verify Attributes works.
			_, err = b.Attributes(ctx, key)
			if err != nil {
				t.Error(err)
			}

			// Verify SignedURL works.
			url, err := b.SignedURL(ctx, key, nil)
			if gcerrors.Code(err) != gcerrors.Unimplemented {
				if err != nil {
					t.Error(err)
				}
				client := h.HTTPClient()
				if client == nil {
					t.Error("can't verify SignedURL, Harness.HTTPClient() returned nil")
				}
				resp, err := client.Get(url)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				if resp.StatusCode != 200 {
					t.Errorf("got status code %d, want 200", resp.StatusCode)
				}
				got, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, content) {
					t.Errorf("got body %q, want %q", string(got), string(content))
				}
			}
		})
	}
}

// testSignedURL tests the functionality of SignedURL.
func testSignedURL(t *testing.T, newHarness HarnessMaker) {
	const key = "blob-for-signing"
	const contents = "hello world"

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
	b := blob.NewBucket(drv)
	defer b.Close()

	// Verify that a negative Expiry gives an error. This is enforced in the
	// portable type, so works regardless of driver support.
	_, err = b.SignedURL(ctx, key, &blob.SignedURLOptions{Expiry: -1 * time.Minute})
	if err == nil {
		t.Error("got nil error, expected error for negative SignedURLOptions.Expiry")
	}

	// Generate real signed URLs for GET, GET with the query params remvoed, PUT, and DELETE.
	getURL, err := b.SignedURL(ctx, key, nil)
	if err != nil {
		if gcerrors.Code(err) == gcerrors.Unimplemented {
			t.Skipf("SignedURL not supported")
			return
		}
		t.Fatal(err)
	} else if getURL == "" {
		t.Fatal("got empty GET url")
	}
	// Copy getURL, but remove all query params. This URL should not be allowed
	// to GET since the client is unauthorized.
	getURLNoParamsURL, err := url.Parse(getURL)
	if err != nil {
		t.Fatalf("failed to parse getURL: %v", err)
	}
	getURLNoParamsURL.RawQuery = ""
	getURLNoParams := getURLNoParamsURL.String()
	const (
		allowedContentType   = "text/plain"
		differentContentType = "application/octet-stream"
	)
	putURLWithContentType, err := b.SignedURL(ctx, key, &blob.SignedURLOptions{
		Method:      http.MethodPut,
		ContentType: allowedContentType,
	})
	if gcerrors.Code(err) == gcerrors.Unimplemented {
		t.Log("PUT URLs with content type not supported, skipping")
	} else if err != nil {
		t.Fatal(err)
	} else if putURLWithContentType == "" {
		t.Fatal("got empty PUT url")
	}
	putURLEnforcedAbsentContentType, err := b.SignedURL(ctx, key, &blob.SignedURLOptions{
		Method:                   http.MethodPut,
		EnforceAbsentContentType: true,
	})
	if gcerrors.Code(err) == gcerrors.Unimplemented {
		t.Log("PUT URLs with enforced absent content type not supported, skipping")
	} else if err != nil {
		t.Fatal(err)
	} else if putURLEnforcedAbsentContentType == "" {
		t.Fatal("got empty PUT url")
	}
	putURLWithoutContentType, err := b.SignedURL(ctx, key, &blob.SignedURLOptions{
		Method: http.MethodPut,
	})
	if err != nil {
		t.Fatal(err)
	} else if putURLWithoutContentType == "" {
		t.Fatal("got empty PUT url")
	}
	deleteURL, err := b.SignedURL(ctx, key, &blob.SignedURLOptions{Method: http.MethodDelete})
	if err != nil {
		t.Fatal(err)
	} else if deleteURL == "" {
		t.Fatal("got empty DELETE url")
	}

	client := h.HTTPClient()
	if client == nil {
		t.Fatal("can't verify SignedURL, Harness.HTTPClient() returned nil")
	}

	// PUT the blob. Try with all URLs, only putURL should work when given the
	// content type used in the signature.
	type signedURLTest struct {
		urlMethod   string
		contentType string
		url         string
		wantSuccess bool
	}
	tests := []signedURLTest{
		{http.MethodGet, "", getURL, false},
		{http.MethodDelete, "", deleteURL, false},
	}
	if putURLWithContentType != "" {
		tests = append(tests, signedURLTest{http.MethodPut, allowedContentType, putURLWithContentType, true})
		tests = append(tests, signedURLTest{http.MethodPut, differentContentType, putURLWithContentType, false})
		tests = append(tests, signedURLTest{http.MethodPut, "", putURLWithContentType, false})
	}
	if putURLEnforcedAbsentContentType != "" {
		tests = append(tests, signedURLTest{http.MethodPut, "", putURLWithoutContentType, true})
		tests = append(tests, signedURLTest{http.MethodPut, differentContentType, putURLWithoutContentType, false})
	}
	if putURLWithoutContentType != "" {
		tests = append(tests, signedURLTest{http.MethodPut, "", putURLWithoutContentType, true})
	}
	for _, test := range tests {
		req, err := http.NewRequest(http.MethodPut, test.url, strings.NewReader(contents))
		if err != nil {
			t.Fatalf("failed to create PUT HTTP request using %s URL (content-type=%q): %v", test.urlMethod, test.contentType, err)
		}
		if test.contentType != "" {
			req.Header.Set("Content-Type", test.contentType)
		}
		if resp, err := client.Do(req); err != nil {
			t.Fatalf("PUT failed with %s URL (content-type=%q): %v", test.urlMethod, test.contentType, err)
		} else {
			defer resp.Body.Close()
			success := resp.StatusCode >= 200 && resp.StatusCode < 300
			if success != test.wantSuccess {
				t.Errorf("PUT with %s URL (content-type=%q) got status code %d, want 2xx? %v", test.urlMethod, test.contentType, resp.StatusCode, test.wantSuccess)
				gotBody, _ := ioutil.ReadAll(resp.Body)
				t.Errorf(string(gotBody))
			}
		}
	}

	// GET it. Try with all URLs, only getURL should work.
	for _, test := range []struct {
		urlMethod   string
		url         string
		wantSuccess bool
	}{
		{http.MethodDelete, deleteURL, false},
		{http.MethodPut, putURLWithoutContentType, false},
		{http.MethodGet, getURLNoParams, false},
		{http.MethodGet, getURL, true},
	} {
		if resp, err := client.Get(test.url); err != nil {
			t.Fatalf("GET with %s URL failed: %v", test.urlMethod, err)
		} else {
			defer resp.Body.Close()
			success := resp.StatusCode >= 200 && resp.StatusCode < 300
			if success != test.wantSuccess {
				t.Errorf("GET with %s URL got status code %d, want 2xx? %v", test.urlMethod, resp.StatusCode, test.wantSuccess)
				gotBody, _ := ioutil.ReadAll(resp.Body)
				t.Errorf(string(gotBody))
			} else if success {
				gotBody, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					t.Errorf("GET with %s URL failed to read response body: %v", test.urlMethod, err)
				} else if gotBodyStr := string(gotBody); gotBodyStr != contents {
					t.Errorf("GET with %s URL got body %q, want %q", test.urlMethod, gotBodyStr, contents)
				}
			}
		}
	}

	// DELETE it. Try with all URLs, only deleteURL should work.
	for _, test := range []struct {
		urlMethod   string
		url         string
		wantSuccess bool
	}{
		{http.MethodGet, getURL, false},
		{http.MethodPut, putURLWithoutContentType, false},
		{http.MethodDelete, deleteURL, true},
	} {
		req, err := http.NewRequest(http.MethodDelete, test.url, nil)
		if err != nil {
			t.Fatalf("failed to create DELETE HTTP request using %s URL: %v", test.urlMethod, err)
		}
		if resp, err := client.Do(req); err != nil {
			t.Fatalf("DELETE with %s URL failed: %v", test.urlMethod, err)
		} else {
			defer resp.Body.Close()
			success := resp.StatusCode >= 200 && resp.StatusCode < 300
			if success != test.wantSuccess {
				t.Fatalf("DELETE with %s URL got status code %d, want 2xx? %v", test.urlMethod, resp.StatusCode, test.wantSuccess)
				gotBody, _ := ioutil.ReadAll(resp.Body)
				t.Errorf(string(gotBody))
			}
		}
	}

	// GET should fail now that the blob has been deleted.
	if resp, err := client.Get(getURL); err != nil {
		t.Errorf("GET after DELETE failed: %v", err)
	} else {
		defer resp.Body.Close()
		if resp.StatusCode != 404 {
			t.Errorf("GET after DELETE got status code %d, want 404", resp.StatusCode)
			gotBody, _ := ioutil.ReadAll(resp.Body)
			t.Errorf(string(gotBody))
		}
	}
}

// testAs tests the various As functions, using AsTest.
func testAs(t *testing.T, newHarness HarnessMaker, st AsTest) {
	const (
		dir     = "mydir"
		key     = dir + "/as-test"
		copyKey = dir + "/as-test-copy"
	)
	var content = []byte("hello world")
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
	b := blob.NewBucket(drv)
	defer b.Close()

	// Verify Bucket.As.
	if err := st.BucketCheck(b); err != nil {
		t.Error(err)
	}

	// Create a blob, using the provided callback.
	if err := b.WriteAll(ctx, key, content, &blob.WriterOptions{BeforeWrite: st.BeforeWrite}); err != nil {
		t.Error(err)
	}
	defer func() { _ = b.Delete(ctx, key) }()

	// Verify Attributes.As.
	attrs, err := b.Attributes(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttributesCheck(attrs); err != nil {
		t.Error(err)
	}

	// Verify Reader.As.
	r, err := b.NewReader(ctx, key, &blob.ReaderOptions{BeforeRead: st.BeforeRead})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := st.ReaderCheck(r); err != nil {
		t.Error(err)
	}

	// Verify ListObject.As for the directory.
	iter := b.List(&blob.ListOptions{Prefix: dir, Delimiter: "/", BeforeList: st.BeforeList})
	found := false
	for {
		obj, err := iter.Next(ctx)
		if err == io.EOF {
			break
		}
		if found {
			t.Fatal("got a second object returned from List, only wanted one")
		}
		found = true
		if err != nil {
			log.Fatal(err)
		}
		if err := st.ListObjectCheck(obj); err != nil {
			t.Error(err)
		}
	}

	// Verify ListObject.As for the blob.
	iter = b.List(&blob.ListOptions{Prefix: key, BeforeList: st.BeforeList})
	found = false
	for {
		obj, err := iter.Next(ctx)
		if err == io.EOF {
			break
		}
		if found {
			t.Fatal("got a second object returned from List, only wanted one")
		}
		found = true
		if err != nil {
			log.Fatal(err)
		}
		if err := st.ListObjectCheck(obj); err != nil {
			t.Error(err)
		}
	}

	_, gotErr := b.NewReader(ctx, "key-does-not-exist", nil)
	if gotErr == nil {
		t.Fatalf("got nil error from NewReader for nonexistent key, want an error")
	}
	if err := st.ErrorCheck(b, gotErr); err != nil {
		t.Error(err)
	}

	// Copy the blob, using the provided callback.
	if err := b.Copy(ctx, copyKey, key, &blob.CopyOptions{BeforeCopy: st.BeforeCopy}); err != nil {
		t.Error(err)
	} else {
		defer func() { _ = b.Delete(ctx, copyKey) }()
	}
}

func benchmarkRead(b *testing.B, bkt *blob.Bucket) {
	ctx := context.Background()
	const key = "readbenchmark-blob"

	content := loadTestData(b, "test-large.jpg")
	if err := bkt.WriteAll(ctx, key, content, nil); err != nil {
		b.Fatal(err)
	}
	defer func() {
		_ = bkt.Delete(ctx, key)
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf, err := bkt.ReadAll(ctx, key)
			if err != nil {
				b.Error(err)
			}
			if !bytes.Equal(buf, content) {
				b.Error("read didn't match write")
			}
		}
	})
}

func benchmarkWriteReadDelete(b *testing.B, bkt *blob.Bucket) {
	ctx := context.Background()
	const baseKey = "writereaddeletebenchmark-blob-"

	content := loadTestData(b, "test-large.jpg")
	var nextID uint32

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		key := fmt.Sprintf("%s%d", baseKey, atomic.AddUint32(&nextID, 1))
		for pb.Next() {
			if err := bkt.WriteAll(ctx, key, content, nil); err != nil {
				b.Error(err)
				continue
			}
			buf, err := bkt.ReadAll(ctx, key)
			if err != nil {
				b.Error(err)
			}
			if !bytes.Equal(buf, content) {
				b.Error("read didn't match write")
			}
			if err := bkt.Delete(ctx, key); err != nil {
				b.Error(err)
				continue
			}
		}
	})
}
