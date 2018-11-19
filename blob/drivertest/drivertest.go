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

// Package drivertest provides a conformance test for implementations of
// driver.
package drivertest

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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/driver"
	"github.com/google/go-cmp/cmp"
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
	// Required if the provider supports SignedURL.
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
// 2. Creates a blob using BeforeWrite as a WriterOption.
// 3. Fetches the blob's attributes and calls AttributeCheck.
// 4. Creates a Reader for the blob and calls ReaderCheck.
// 5. Calls List using BeforeList as a ListOption, and calls ListObjectCheck
//    on the single list entry returned.
// 6. Tries to read a non-existent blob, and calls ErrorCheck with the error.
//
// For example, an AsTest might set a provider-specific field to a custom
// value in BeforeWrite, and then verify the custom value was returned in
// AttributesCheck and/or ReaderCheck.
type AsTest interface {
	// Name should return a descriptive name for the test.
	Name() string
	// BucketCheck will be called to allow verification of Bucket.As.
	BucketCheck(b *blob.Bucket) error
	// ErrorCheck will be called to allow verification of Bucket.ErrorAs.
	ErrorCheck(err error) error
	// BeforeWrite will be passed directly to WriterOptions as part of creating
	// a test blob.
	BeforeWrite(as func(interface{}) bool) error
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

func (verifyAsFailsOnNil) ErrorCheck(err error) error {
	if blob.ErrorAs(err, nil) {
		return errors.New("want ErrorAs to return false when passed nil")
	}
	return nil
}

func (verifyAsFailsOnNil) BeforeWrite(as func(interface{}) bool) error {
	if as(nil) {
		return errors.New("want Writer.As to return false when passed nil")
	}
	return nil
}

func (verifyAsFailsOnNil) BeforeList(as func(interface{}) bool) error {
	if as(nil) {
		return errors.New("want List.As to return false when passed nil")
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

// RunConformanceTests runs conformance tests for provider implementations of blob.
func RunConformanceTests(t *testing.T, newHarness HarnessMaker, asTests []AsTest) {
	t.Run("TestList", func(t *testing.T) {
		testList(t, newHarness)
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
	t.Run("TestMetadata", func(t *testing.T) {
		testMetadata(t, newHarness)
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
			wantPages: [][]int{[]int{1}},
			want:      []int{1},
		},
		{
			name:      "no pagination",
			prefix:    keyPrefix,
			wantPages: [][]int{[]int{0, 1, 2}},
			want:      []int{0, 1, 2},
		},
		{
			name:      "by 1",
			prefix:    keyPrefix,
			pageSize:  1,
			wantPages: [][]int{[]int{0}, []int{1}, []int{2}},
			want:      []int{0, 1, 2},
		},
		{
			name:      "by 2",
			prefix:    keyPrefix,
			pageSize:  2,
			wantPages: [][]int{[]int{0, 1}, []int{2}},
			want:      []int{0, 1, 2},
		},
		{
			name:      "by 3",
			prefix:    keyPrefix,
			pageSize:  3,
			wantPages: [][]int{[]int{0, 1, 2}},
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
		count := countItems(ctx, t, iter)
		if count != 3 {
			for i := 0; i < 3; i++ {
				if err := b.WriteAll(ctx, keyForIndex(i), content, nil); err != nil {
					t.Fatal(err)
				}
			}
		}
		return drv, func() { h.Close() }
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
		[]string{"dir1", "a.txt"},
		[]string{"dir1", "b.txt"},
		[]string{"dir1", "subdir", "c.txt"},
		[]string{"dir1", "subdir", "d.txt"},
		[]string{"dir2", "e.txt"},
		[]string{"f.txt"},
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
				listResult{Key: keyPrefix + "/dir1/a.txt"},
				listResult{Key: keyPrefix + "/dir1/b.txt"},
				listResult{Key: keyPrefix + "/dir1/subdir/c.txt"},
				listResult{Key: keyPrefix + "/dir1/subdir/d.txt"},
				listResult{Key: keyPrefix + "/dir2/e.txt"},
				listResult{Key: keyPrefix + "/f.txt"},
			},
			wantRecursive: []listResult{
				listResult{
					Key:   keyPrefix + "/dir1/",
					IsDir: true,
					Sub: []listResult{
						listResult{Key: keyPrefix + "/dir1/a.txt"},
						listResult{Key: keyPrefix + "/dir1/b.txt"},
						listResult{
							Key:   keyPrefix + "/dir1/subdir/",
							IsDir: true,
							Sub: []listResult{
								listResult{Key: keyPrefix + "/dir1/subdir/c.txt"},
								listResult{Key: keyPrefix + "/dir1/subdir/d.txt"},
							},
						},
					},
				},
				listResult{
					Key:   keyPrefix + "/dir2/",
					IsDir: true,
					Sub: []listResult{
						listResult{Key: keyPrefix + "/dir2/e.txt"},
					},
				},
				listResult{Key: keyPrefix + "/f.txt"},
			},
			wantPaged: []listResult{
				listResult{
					Key:   keyPrefix + "/dir1/",
					IsDir: true,
				},
				listResult{
					Key:   keyPrefix + "/dir2/",
					IsDir: true,
				},
				listResult{Key: keyPrefix + "/f.txt"},
			},
			wantAfterDel: []listResult{
				listResult{
					Key:   keyPrefix + "/dir1/",
					IsDir: true,
				},
				listResult{Key: keyPrefix + "/f.txt"},
			},
		},
		{
			name:  "backslash",
			delim: "\\",
			wantFlat: []listResult{
				listResult{Key: keyPrefix + "\\dir1\\a.txt"},
				listResult{Key: keyPrefix + "\\dir1\\b.txt"},
				listResult{Key: keyPrefix + "\\dir1\\subdir\\c.txt"},
				listResult{Key: keyPrefix + "\\dir1\\subdir\\d.txt"},
				listResult{Key: keyPrefix + "\\dir2\\e.txt"},
				listResult{Key: keyPrefix + "\\f.txt"},
			},
			wantRecursive: []listResult{
				listResult{
					Key:   keyPrefix + "\\dir1\\",
					IsDir: true,
					Sub: []listResult{
						listResult{Key: keyPrefix + "\\dir1\\a.txt"},
						listResult{Key: keyPrefix + "\\dir1\\b.txt"},
						listResult{
							Key:   keyPrefix + "\\dir1\\subdir\\",
							IsDir: true,
							Sub: []listResult{
								listResult{Key: keyPrefix + "\\dir1\\subdir\\c.txt"},
								listResult{Key: keyPrefix + "\\dir1\\subdir\\d.txt"},
							},
						},
					},
				},
				listResult{
					Key:   keyPrefix + "\\dir2\\",
					IsDir: true,
					Sub: []listResult{
						listResult{Key: keyPrefix + "\\dir2\\e.txt"},
					},
				},
				listResult{Key: keyPrefix + "\\f.txt"},
			},
			wantPaged: []listResult{
				listResult{
					Key:   keyPrefix + "\\dir1\\",
					IsDir: true,
				},
				listResult{
					Key:   keyPrefix + "\\dir2\\",
					IsDir: true,
				},
				listResult{Key: keyPrefix + "\\f.txt"},
			},
			wantAfterDel: []listResult{
				listResult{
					Key:   keyPrefix + "\\dir1\\",
					IsDir: true,
				},
				listResult{Key: keyPrefix + "\\f.txt"},
			},
		},
		{
			name:  "abc",
			delim: "abc",
			wantFlat: []listResult{
				listResult{Key: keyPrefix + "abcdir1abca.txt"},
				listResult{Key: keyPrefix + "abcdir1abcb.txt"},
				listResult{Key: keyPrefix + "abcdir1abcsubdirabcc.txt"},
				listResult{Key: keyPrefix + "abcdir1abcsubdirabcd.txt"},
				listResult{Key: keyPrefix + "abcdir2abce.txt"},
				listResult{Key: keyPrefix + "abcf.txt"},
			},
			wantRecursive: []listResult{
				listResult{
					Key:   keyPrefix + "abcdir1abc",
					IsDir: true,
					Sub: []listResult{
						listResult{Key: keyPrefix + "abcdir1abca.txt"},
						listResult{Key: keyPrefix + "abcdir1abcb.txt"},
						listResult{
							Key:   keyPrefix + "abcdir1abcsubdirabc",
							IsDir: true,
							Sub: []listResult{
								listResult{Key: keyPrefix + "abcdir1abcsubdirabcc.txt"},
								listResult{Key: keyPrefix + "abcdir1abcsubdirabcd.txt"},
							},
						},
					},
				},
				listResult{
					Key:   keyPrefix + "abcdir2abc",
					IsDir: true,
					Sub: []listResult{
						listResult{Key: keyPrefix + "abcdir2abce.txt"},
					},
				},
				listResult{Key: keyPrefix + "abcf.txt"},
			},
			wantPaged: []listResult{
				listResult{
					Key:   keyPrefix + "abcdir1abc",
					IsDir: true,
				},
				listResult{
					Key:   keyPrefix + "abcdir2abc",
					IsDir: true,
				},
				listResult{Key: keyPrefix + "abcf.txt"},
			},
			wantAfterDel: []listResult{
				listResult{
					Key:   keyPrefix + "abcdir1abc",
					IsDir: true,
				},
				listResult{Key: keyPrefix + "abcf.txt"},
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
		count := countItems(ctx, t, iter)
		if count != len(keys) {
			for _, key := range keys {
				if err := b.WriteAll(ctx, prefix+strings.Join(key, delim), content, nil); err != nil {
					t.Fatal(err)
				}
			}
		}
		return drv, b, func() { h.Close() }
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

func countItems(ctx context.Context, t *testing.T, iter *blob.ListIterator) int {
	count := 0
	for {
		if _, err := iter.Next(ctx); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		count++
	}
	return count
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
			name:       "length 0 read fails",
			key:        key,
			wantErr:    true,
			skipCreate: true,
		},
		{
			name:       "negative offset fails",
			key:        key,
			offset:     -1,
			wantErr:    true,
			skipCreate: true,
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
			offset:       0,
			length:       -1,
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
			return b, func() { h.Close() }
		}
		if err := b.WriteAll(ctx, key, content, nil); err != nil {
			t.Fatal(err)
		}
		return b, func() {
			_ = b.Delete(ctx, key)
			h.Close()
		}
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, done := init(t, tc.skipCreate)
			defer done()

			r, err := b.NewRangeReader(ctx, tc.key, tc.offset, tc.length)
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
			r.Close()
		})
	}
}

// testAttributes tests Attributes.
func testAttributes(t *testing.T, newHarness HarnessMaker) {
	const (
		key         = "blob-for-attributes"
		contentType = "text/plain"
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
			ContentType: contentType,
		}
		if err := b.WriteAll(ctx, key, content, opts); err != nil {
			t.Fatal(err)
		}
		return b, func() {
			_ = b.Delete(ctx, key)
			h.Close()
		}
	}

	b, done := init(t)
	defer done()

	a, err := b.Attributes(ctx, key)
	if err != nil {
		t.Fatalf("failed Attributes: %v", err)
	}
	// Also make a Reader so we can verify the subset of attributes
	// that it exposes.
	r, err := b.NewReader(ctx, key)
	if err != nil {
		t.Fatalf("failed Attributes: %v", err)
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
func loadTestData(t *testing.T, name string) []byte {
	data, err := Asset(name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// testWrite tests the functionality of NewWriter and Writer.
func testWrite(t *testing.T, newHarness HarnessMaker) {
	const key = "blob-for-reading"
	smallText := loadTestData(t, "test-small.txt")
	mediumHTML := loadTestData(t, "test-medium.html")
	largeJpg := loadTestData(t, "test-large.jpg")
	helloWorld := []byte("hello world")
	helloWorldMD5 := md5.Sum(helloWorld)

	tests := []struct {
		name            string
		key             string
		content         []byte
		contentType     string
		contentMD5      []byte
		firstChunk      int
		wantContentType string
		wantErr         bool
	}{
		{
			name:    "write to empty key fails",
			wantErr: true,
		},
		{
			name: "no write then close results in empty blob",
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
			// The write will be buffered in the concrete type as part of
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

			opts := &blob.WriterOptions{
				ContentType: test.contentType,
			}
			// If the test wants the blob to already exist, write it.
			if test.exists {
				if err := b.WriteAll(ctx, key, content, opts); err != nil {
					t.Fatal(err)
				}
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
			// Cancel the context to abort the write.
			cancel()
			// Close should return some kind of canceled context error.
			// We can't verify the kind of error cleanly, so we just verify there's
			// an error.
			if err := w.Close(); err == nil {
				t.Errorf("got Close error %v want canceled ctx error", err)
			}
			got, err := b.ReadAll(ctx, key)
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
				"key-a": "value-a",
				"kEy-B": "value-b",
				"key-c": "vAlUe-c",
			},
			want: map[string]string{
				"key-a": "value-a",
				"key-b": "value-b",
				"key-c": "vAlUe-c",
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

		err = b.Delete(ctx, "does-not-exist")
		if err == nil {
			t.Errorf("want error, got nil")
		} else if !blob.IsNotExist(err) {
			t.Errorf("want IsNotExist error, got %v", err)
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

		// Create the blob.
		if err := b.WriteAll(ctx, key, []byte("Hello world"), nil); err != nil {
			t.Fatal(err)
		}
		// Delete it.
		if err := b.Delete(ctx, key); err != nil {
			t.Errorf("got unexpected error deleting blob: %v", err)
		}
		// Subsequent read fails with IsNotExist.
		_, err = b.NewReader(ctx, key)
		if err == nil {
			t.Errorf("read after delete want error, got nil")
		} else if !blob.IsNotExist(err) {
			t.Errorf("read after delete want IsNotExist error, got %v", err)
		}
		// Subsequent delete also fails.
		err = b.Delete(ctx, key)
		if err == nil {
			t.Errorf("delete after delete want error, got nil")
		} else if !blob.IsNotExist(err) {
			t.Errorf("delete after delete want IsNotExist error, got %v", err)
		}
	})
}

// testKeys tests a variety of weird keys.
func testKeys(t *testing.T, newHarness HarnessMaker) {
	const keyPrefix = "weird-keys"
	content := []byte("hello")

	tests := []struct {
		description string
		key         string
	}{
		{
			description: "fwdslashes",
			key:         "foo/bar/baz",
		},
		{
			description: "backslashes",
			key:         "foo\\bar\\baz",
		},
		{
			description: "quote",
			key:         "foo\"bar\"baz",
		},
		{
			description: "punctuation",
			key:         "~!@#$%^&*()_+`-=[]{}\\|;':\",/.<>?",
		},
		{
			description: "unicode",
			key:         strings.Repeat("☺", 10),
		},
	}

	ctx := context.Background()

	// Creates the blob.
	// We don't delete the blob, because there's no guarantee that after we
	// create it that it will be immediately returned from List. The very first
	// time the test is run against a Bucket, it may be flaky due to this race.
	init := func(t *testing.T, key string) (*blob.Bucket, func()) {
		h, err := newHarness(ctx, t)
		if err != nil {
			t.Fatal(err)
		}
		drv, err := h.MakeDriver(ctx)
		if err != nil {
			t.Fatal(err)
		}
		b := blob.NewBucket(drv)
		if err := b.WriteAll(ctx, key, content, nil); err != nil {
			t.Fatal(err)
		}
		return b, func() { h.Close() }
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			key := keyPrefix + tc.key
			b, done := init(t, key)
			defer done()

			// Verify read works.
			gotContent, err := b.ReadAll(ctx, key)
			if err != nil {
				t.Fatal(err)
			}
			if !cmp.Equal(gotContent, content) {
				t.Errorf("got %q want %q", string(gotContent), string(content))
			}
			// Verify List returns the key.
			iter := b.List(&blob.ListOptions{Prefix: key})
			obj, err := iter.Next(ctx)
			if err != nil && err != io.EOF {
				t.Fatal(err)
			}
			if err == io.EOF {
				t.Errorf("key not returned from List")
			} else if obj.Key != key {
				t.Errorf("wrong key returned from List, got %v want %v", obj.Key, key)
			}
		})
	}
}

// testSignedURL tests the functionality of SignedURL.
func testSignedURL(t *testing.T, newHarness HarnessMaker) {
	const key = "blob-for-signing"
	var contents = []byte("hello world")

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

	// Verify that a negative Expiry gives an error. This is enforced in the
	// concrete type, so works regardless of provider support.
	_, err = b.SignedURL(ctx, key, &blob.SignedURLOptions{Expiry: -1 * time.Minute})
	if err == nil {
		t.Error("got nil error, expected error for negative SignedURLOptions.Expiry")
	}

	// Try to generate a real signed URL.
	url, err := b.SignedURL(ctx, key, nil)
	if err != nil {
		if blob.IsNotImplemented(err) {
			t.Skipf("SignedURL not supported")
			return
		}
		t.Fatal(err)
	}
	if url == "" {
		t.Fatal("got empty url")
	}
	client := h.HTTPClient()
	if client == nil {
		t.Fatal("can't verify SignedURL, Harness.HTTPClient() returned nil")
	}
	// Create the blob.
	if err := b.WriteAll(ctx, key, contents, nil); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Delete(ctx, key) }()

	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("got status code %d, want 200", resp.StatusCode)
	}
	got, err := ioutil.ReadAll(resp.Body)
	if !bytes.Equal(got, contents) {
		t.Errorf("got body %q, want %q", string(got), string(contents))
	}
}

// testAs tests the various As functions, using AsTest.
func testAs(t *testing.T, newHarness HarnessMaker, st AsTest) {
	const key = "as-test"
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
	if err := st.AttributesCheck(&attrs); err != nil {
		t.Error(err)
	}

	// Verify Reader.As.
	r, err := b.NewReader(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReaderCheck(r); err != nil {
		t.Error(err)
	}

	// Verify ListObject.As.
	iter := b.List(&blob.ListOptions{Prefix: key, BeforeList: st.BeforeList})
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

	_, gotErr := b.NewReader(ctx, "key-does-not-exist")
	if gotErr == nil {
		t.Fatalf("got nil error from NewReader for nonexistent key, want an error")
	}
	if err := st.ErrorCheck(gotErr); err != nil {
		t.Error(err)
	}
}
