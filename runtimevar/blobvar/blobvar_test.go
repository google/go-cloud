// Copyright 2019 The Go Cloud Development Kit Authors
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

package blobvar

import (
	"context"
	"errors"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/blob"
	"gocloud.dev/blob/fileblob"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
	"gocloud.dev/runtimevar/drivertest"
)

type harness struct {
	dir    string
	bucket *blob.Bucket
}

func newHarness(t *testing.T) (drivertest.Harness, error) {
	dir := path.Join(os.TempDir(), "go-cloud-blobvar")
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return nil, err
	}
	b, err := fileblob.OpenBucket(dir, nil)
	if err != nil {
		return nil, err
	}
	return &harness{dir: dir, bucket: b}, nil
}

func (h *harness) MakeWatcher(ctx context.Context, name string, decoder *runtimevar.Decoder) (driver.Watcher, error) {
	return newWatcher(h.bucket, name, decoder, nil, nil), nil
}

func (h *harness) CreateVariable(ctx context.Context, name string, val []byte) error {
	return h.bucket.WriteAll(ctx, name, val, nil)
}

func (h *harness) UpdateVariable(ctx context.Context, name string, val []byte) error {
	return h.bucket.WriteAll(ctx, name, val, nil)
}

func (h *harness) DeleteVariable(ctx context.Context, name string) error {
	return h.bucket.Delete(ctx, name)
}

func (h *harness) Close() {
	h.bucket.Close()
	_ = os.RemoveAll(h.dir)
}

func (h *harness) Mutable() bool { return true }

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyAs{}})
}

type verifyAs struct{}

func (verifyAs) Name() string {
	return "verify As"
}

func (verifyAs) SnapshotCheck(s *runtimevar.Snapshot) error {
	return nil
}

func (verifyAs) ErrorCheck(v *runtimevar.Variable, err error) error {
	var perr *os.PathError
	if !v.ErrorAs(err, &perr) {
		return errors.New("runtimevar.ErrorAs failed with *os.PathError")
	}
	return nil
}

func TestOpenVariable(t *testing.T) {
	dir, err := ioutil.TempDir("", "gcdk-blob-var-example")
	if err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(filepath.Join(dir, "myvar.json"), []byte(`{"Foo": "Bar"}`), 0666); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(filepath.Join(dir, "myvar.txt"), []byte("hello world!"), 0666); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// Convert dir to a URL path, adding a leading "/" if needed on Windows
	// (on Unix, dirpath already has a leading "/").
	dirpath := filepath.ToSlash(dir)
	if os.PathSeparator != '/' && !strings.HasPrefix(dirpath, "/") {
		dirpath = "/" + dirpath
	}
	bucketURL := "file://" + dirpath

	tests := []struct {
		BucketURL    string
		URL          string
		WantErr      bool
		WantWatchErr bool
		Want         interface{}
	}{
		// myvar does not exist.
		{"mem://", "blob://myvar", false, true, nil},
		// badscheme does not exist.
		{"badscheme://", "blob://myvar", true, false, nil},
		// directory dirnotfound does not exist, so Bucket creation fails.
		{"file:///dirnotfound", "blob://myvar.txt", true, false, nil},
		// filenotfound does not exist so Watch returns an error.
		{bucketURL, "blob://filenotfound", false, true, nil},
		// Missing bucket env variable.
		{"", "blob://myvar.txt", true, false, nil},
		// Invalid decoder.
		{bucketURL, "blob://myvar.txt?decoder=notadecoder", true, false, nil},
		// Invalid arg.
		{bucketURL, "blob://myvar.txt?param=value", true, false, nil},
		// Working example with default decoder.
		{bucketURL, "blob://myvar.txt", false, false, []byte("hello world!")},
		// Working example with string decoder.
		{bucketURL, "blob://myvar.txt?decoder=string", false, false, "hello world!"},
		// Working example with JSON decoder.
		{bucketURL, "blob://myvar.json?decoder=jsonmap", false, false, &map[string]interface{}{"Foo": "Bar"}},
	}

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.BucketURL, func(t *testing.T) {
			os.Setenv("BLOBVAR_BUCKET_URL", test.BucketURL)

			opener := &defaultOpener{}
			defer func() {
				if opener.opener != nil && opener.opener.Bucket != nil {
					opener.opener.Bucket.Close()
				}
			}()
			u, err := url.Parse(test.URL)
			if err != nil {
				t.Error(err)
			}
			v, err := opener.OpenVariableURL(ctx, u)
			if v != nil {
				defer v.Close()
			}
			if (err != nil) != test.WantErr {
				t.Errorf("BucketURL %s URL %s: got error %v, want error %v", test.BucketURL, test.URL, err, test.WantErr)
			}
			if err != nil {
				return
			}
			defer v.Close()
			snapshot, err := v.Watch(ctx)
			if (err != nil) != test.WantWatchErr {
				t.Errorf("BucketURL %s URL %s: got Watch error %v, want error %v", test.BucketURL, test.URL, err, test.WantWatchErr)
			}
			if err != nil {
				return
			}
			if !cmp.Equal(snapshot.Value, test.Want) {
				t.Errorf("BucketURL %s URL %s: got snapshot value\n%v\n  want\n%v", test.BucketURL, test.URL, snapshot.Value, test.Want)
			}
		})
	}
}
