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

	tests := []struct {
		URL          string
		WantErr      bool
		WantWatchErr bool
		Want         interface{}
	}{
		// Variable construction succeeds, but myvar does not exist.
		{"blob://myvar?bucket=mem://&decoder=string", false, true, nil},
		// Variable construction fails because the directory dirnotfound does not
		// exist, so the Bucket creation fails.
		{"blob://myvar.txt?bucket=file:///dirnotfound&decoder=string", true, false, nil},
		// Variable construction succeeds, but filenotfound does not exist so Watch
		// returns an error.
		{"blob://filenotfound?bucket=file://" + dirpath + "&decoder=string", false, true, nil},
		// Variable construction fails due to missing bucket arg.
		{"blob://myvar.txt?decoder=string", true, false, nil},
		// Variable construction fails due to invalid wait arg.
		{"blob://myvar.txt?bucket=file://" + dirpath + "&decoder=string&wait=notaduration", true, false, nil},
		// Variable construction fails due to invalid decoder arg.
		{"blob://myvar.txt?bucket=file://" + dirpath + "&decoder=notadecoder", true, false, nil},
		// Variable construction fails due to invalid arg.
		{"blob://myvar.txt?bucket=file://" + dirpath + "&decoder=string&param=value", true, false, nil},
		// Working example with default decoder.
		{"blob://myvar.txt?bucket=file://" + dirpath, false, false, []byte("hello world!")},
		// Working example with string decoder and wait.
		{"blob://myvar.txt?bucket=file://" + dirpath + "&decoder=string&wait=5s", false, false, "hello world!"},
		// Working example with JSON decoder.
		{"blob://myvar.json?bucket=file://" + dirpath + "&decoder=jsonmap", false, false, &map[string]interface{}{"Foo": "Bar"}},
	}

	ctx := context.Background()
	for _, test := range tests {
		v, err := runtimevar.OpenVariable(ctx, test.URL)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
		if err != nil {
			continue
		}
		snapshot, err := v.Watch(ctx)
		if (err != nil) != test.WantWatchErr {
			t.Errorf("%s: got Watch error %v, want error %v", test.URL, err, test.WantWatchErr)
		}
		if err != nil {
			continue
		}
		if !cmp.Equal(snapshot.Value, test.Want) {
			t.Errorf("%s: got snapshot value\n%v\n  want\n%v", test.URL, snapshot.Value, test.Want)
		}
	}
}
