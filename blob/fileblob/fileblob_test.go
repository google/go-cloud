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
	"context"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/google/go-cloud/blob/driver"
	"github.com/google/go-cloud/blob/drivertest"
)

type harness struct {
	dir    string
	closer func()
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	dir := path.Join(os.TempDir(), "go-cloud-fileblob")
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return nil, err
	}
	return &harness{
		dir:    dir,
		closer: func() { _ = os.RemoveAll(dir) },
	}, nil
}

func (h *harness) HTTPClient() *http.Client {
	return nil
}

func (h *harness) MakeDriver(ctx context.Context) (driver.Bucket, error) {
	return openBucket(h.dir, nil)
}

func (h *harness) Close() {
	h.closer()
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, nil)
}

// File-specific unit tests.
func TestNewBucket(t *testing.T) {
	t.Run("BucketDirMissing", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "fileblob")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		_, gotErr := OpenBucket(filepath.Join(dir, "notfound"), nil)
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
		_, gotErr := OpenBucket(f.Name(), nil)
		if gotErr == nil {
			t.Error("want error, got nil")
		}
	})
}

func TestEscape(t *testing.T) {
	for _, tc := range []struct {
		key, want string
	}{
		{
			key:  "abc09ABC -_.",
			want: "abc09ABC -_.",
		},
		{
			key:  "~!@#$%^&*()+`=[]{}\\|;:'\",<>,",
			want: "%7E%21%40%23%24%25%5E%26%2A%28%29%2B%60%3D%5B%5D%7B%7D%5C%7C%3B%3A%27%22%2C%3C%3E%2C",
		},
		{
			key:  "/",
			want: string(os.PathSeparator),
		},
		{
			key:  "☺☺",
			want: "%E2%98%BA%E2%98%BA",
		},
	} {
		got := escape(tc.key)
		if got != tc.want {
			t.Errorf("%s: got escaped %q want %q", tc.key, got, tc.want)
		}
		var err error
		got, err = unescape(got)
		if err != nil {
			t.Error(err)
		}
		if got != tc.key {
			t.Errorf("%s: got unescaped %q want %q", tc.key, got, tc.key)
		}
	}
}

func TestUnescape(t *testing.T) {
	for _, tc := range []struct {
		filename, want string
		wantErr        bool
	}{
		{
			filename: "%7E",
			want:     "~",
		},
		{
			filename: "abc%7Eabc",
			want:     "abc~abc",
		},
		{
			filename: "%7e",
			wantErr:  true, // wrong case in hex
		},
		{
			filename: "aa~bb",
			wantErr:  true, // ~ should be escaped
		},
		{
			filename: "abc%gabc",
			wantErr:  true, // invalid hex after %
		},
	} {
		got, err := unescape(tc.filename)
		if tc.wantErr != (err != nil) {
			t.Errorf("%s: got err %v want %v", tc.filename, err, tc.wantErr)
		}
		if got != tc.want {
			t.Errorf("%s: got unescaped %q want %q", tc.filename, got, tc.want)
		}
	}
}
