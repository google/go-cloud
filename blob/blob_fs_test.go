// Copyright 2023 The Go Cloud Development Kit Authors
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

package blob_test

import (
	"context"
	"io/fs"
	"sort"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/blob"
	"gocloud.dev/blob/memblob"
)

var fsFiles = []string{
	"a/very/deeply/nested/sub/dir/with/a/file.txt",
	"baz.txt",
	"bazfoo.txt",
	"dir/foo.txt",
	"dir/subdir/foo.txt",
	"foo.txt",
	"foobar.txt",
}

func initBucket(t *testing.T, files []string) *blob.Bucket {
	ctx := context.Background()

	b := memblob.OpenBucket(nil)
	b.SetIOFSCallback(func() (context.Context, *blob.ReaderOptions) { return ctx, nil })
	for _, f := range files {
		if err := b.WriteAll(ctx, f, []byte("data"), nil); err != nil {
			t.Fatal(err)
		}
	}
	return b
}

// TestIOFS runs the test/fstest test suite for fs.FS.
func TestIOFS(t *testing.T) {
	tests := []struct {
		Description string
		Files       []string
	}{
		{
			Description: "empty bucket",
		},
		{
			Description: "non-empty bucket",
			Files:       fsFiles,
		},
	}

	for _, test := range tests {
		t.Run(test.Description, func(t *testing.T) {
			b := initBucket(t, test.Files)
			defer b.Close()
			if err := fstest.TestFS(b, test.Files...); err != nil {
				t.Error(err)
			}
		})
	}
}

// TestGlob does some basic verification that fs.Glob works as expected
// when given a blob.Bucket.
func TestGlob(t *testing.T) {
	b := initBucket(t, fsFiles)
	defer b.Close()

	tests := []struct {
		Pattern string
		Want    []string
	}{
		{
			Pattern: "*",
			Want:    []string{"a", "baz.txt", "bazfoo.txt", "dir", "foo.txt", "foobar.txt"},
		},
		{
			Pattern: "foo*",
			Want:    []string{"foo.txt", "foobar.txt"},
		},
		{
			Pattern: "*foo*",
			Want:    []string{"bazfoo.txt", "foo.txt", "foobar.txt"},
		},
	}
	for _, test := range tests {
		t.Run(test.Pattern, func(t *testing.T) {
			if got, err := fs.Glob(b, test.Pattern); err != nil {
				t.Fatalf("Failed to glob: %v", err)
			} else if diff := cmp.Diff(got, test.Want); diff != "" {
				t.Error(diff)
			}
		})
	}
}

// TestWalkDir does some basic verification that fs.WalkDir works as expected
// when given a blob.Bucket.
func TestWalkDir(t *testing.T) {
	b := initBucket(t, fsFiles)
	defer b.Close()

	var got []string
	fn := func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			t.Errorf("WalkFunc with path %s got error: %v", path, err)
			return err
		}
		got = append(got, path)
		return nil
	}
	if err := fs.WalkDir(b, ".", fn); err != nil {
		t.Fatalf("WalkDir got an unexpected error: %v", err)
	}
	// We want all of the files, plus the directories.
	want := append(fsFiles,
		".",
		"a",
		"a/very",
		"a/very/deeply",
		"a/very/deeply/nested",
		"a/very/deeply/nested/sub",
		"a/very/deeply/nested/sub/dir",
		"a/very/deeply/nested/sub/dir/with",
		"a/very/deeply/nested/sub/dir/with/a",
		"dir",
		"dir/subdir",
	)
	sort.Strings(want)
	if diff := cmp.Diff(got, want); diff != "" {
		t.Error(diff)
	}
}
