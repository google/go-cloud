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

package fileblob

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/drivertest"
)

// makeBucket creates a *blob.Bucket and a function to close it after the test
// is done. It fails the test if the creation fails.
func makeBucket(t *testing.T) (*blob.Bucket, func()) {
	dir := path.Join(os.TempDir(), "go-cloud-fileblob")
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		t.Fatal(err)
	}
	b, err := NewBucket(dir)
	if err != nil {
		t.Fatal(err)
	}
	return b, func() { _ = os.RemoveAll(dir) }
}
func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, makeBucket, "../testdata")
}

// File-specific unit tests.
func TestNewBucket(t *testing.T) {
	t.Run("BucketDirMissing", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "fileblob")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		_, gotErr := NewBucket(filepath.Join(dir, "notfound"))
		if gotErr == nil {
			t.Errorf("want error, got nil")
		}
	})
	t.Run("BucketIsFile", func(t *testing.T) {
		f, err := ioutil.TempFile("", "fileblob")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(f.Name())
		_, gotErr := NewBucket(f.Name())
		if gotErr == nil {
			t.Error("want error, got nil")
		}
	})
}
