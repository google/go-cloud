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

package fileblob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gocloud.dev/blob"
	"gocloud.dev/blob/driver"
	"gocloud.dev/blob/drivertest"
)

type harness struct {
	dir       string
	prefix    string
	server    *httptest.Server
	urlSigner URLSigner
	closer    func()
}

func newHarness(ctx context.Context, t *testing.T, prefix string) (drivertest.Harness, error) {
	dir := filepath.Join(os.TempDir(), "go-cloud-fileblob")
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return nil, err
	}
	if prefix != "" {
		if err := os.MkdirAll(filepath.Join(dir, prefix), os.ModePerm); err != nil {
			return nil, err
		}
	}
	h := &harness{dir: dir, prefix: prefix}

	localServer := httptest.NewServer(http.HandlerFunc(h.serveSignedURL))
	h.server = localServer

	u, err := url.Parse(h.server.URL)
	if err != nil {
		return nil, err
	}
	h.urlSigner = NewURLSignerHMAC(u, []byte("I'm a secret key"))

	h.closer = func() { _ = os.RemoveAll(dir); localServer.Close() }

	return h, nil
}

func (h *harness) serveSignedURL(w http.ResponseWriter, r *http.Request) {
	objKey, err := h.urlSigner.KeyFromURL(r.Context(), r.URL)
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	allowedMethod := r.URL.Query().Get("method")
	if allowedMethod == "" {
		allowedMethod = http.MethodGet
	}
	if allowedMethod != r.Method {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	contentType := r.URL.Query().Get("contentType")
	if r.Header.Get("Content-Type") != contentType {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	bucket, err := OpenBucket(h.dir, &Options{})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer bucket.Close()

	switch r.Method {
	case http.MethodGet:
		reader, err := bucket.NewReader(r.Context(), objKey, nil)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		defer reader.Close()
		io.Copy(w, reader)
	case http.MethodPut:
		writer, err := bucket.NewWriter(r.Context(), objKey, &blob.WriterOptions{
			ContentType: contentType,
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		io.Copy(writer, r.Body)
		if err := writer.Close(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	case http.MethodDelete:
		if err := bucket.Delete(r.Context(), objKey); err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
	default:
		w.WriteHeader(http.StatusForbidden)
	}
}

func (h *harness) HTTPClient() *http.Client {
	return &http.Client{}
}

func (h *harness) MakeDriver(ctx context.Context) (driver.Bucket, error) {
	opts := &Options{
		URLSigner: h.urlSigner,
	}
	drv, err := openBucket(h.dir, opts)
	if err != nil {
		return nil, err
	}
	if h.prefix == "" {
		return drv, nil
	}
	return driver.NewPrefixedBucket(drv, h.prefix), nil
}

func (h *harness) Close() {
	h.closer()
}

func TestConformance(t *testing.T) {
	newHarnessNoPrefix := func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
		return newHarness(ctx, t, "")
	}
	drivertest.RunConformanceTests(t, newHarnessNoPrefix, []drivertest.AsTest{verifyPathError{}})
}

func TestConformanceWithPrefix(t *testing.T) {
	const prefix = "some/prefix/dir/"
	newHarnessWithPrefix := func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
		return newHarness(ctx, t, prefix)
	}
	drivertest.RunConformanceTests(t, newHarnessWithPrefix, []drivertest.AsTest{verifyPathError{prefix: prefix}})
}

func BenchmarkFileblob(b *testing.B) {
	dir := filepath.Join(os.TempDir(), "go-cloud-fileblob")
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		b.Fatal(err)
	}
	bkt, err := OpenBucket(dir, nil)
	if err != nil {
		b.Fatal(err)
	}
	drivertest.RunBenchmarks(b, bkt)
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
			t.Errorf("got nil want error")
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
			t.Errorf("got nil want error")
		}
	})
}

type verifyPathError struct {
	prefix string
}

func (verifyPathError) Name() string { return "verify ErrorAs handles os.PathError" }

func (verifyPathError) BucketCheck(b *blob.Bucket) error             { return nil }
func (verifyPathError) BeforeRead(as func(interface{}) bool) error   { return nil }
func (verifyPathError) BeforeWrite(as func(interface{}) bool) error  { return nil }
func (verifyPathError) BeforeCopy(as func(interface{}) bool) error   { return nil }
func (verifyPathError) BeforeList(as func(interface{}) bool) error   { return nil }
func (verifyPathError) AttributesCheck(attrs *blob.Attributes) error { return nil }
func (verifyPathError) ReaderCheck(r *blob.Reader) error             { return nil }
func (verifyPathError) ListObjectCheck(o *blob.ListObject) error     { return nil }

func (v verifyPathError) ErrorCheck(b *blob.Bucket, err error) error {
	var perr *os.PathError
	if !b.ErrorAs(err, &perr) {
		return errors.New("want ErrorAs to succeed for PathError")
	}
	wantSuffix := filepath.Join("go-cloud-fileblob", v.prefix, "key-does-not-exist")
	if got := perr.Path; !strings.HasSuffix(got, wantSuffix) {
		return fmt.Errorf("got path %q, want suffix %q", got, wantSuffix)
	}
	return nil
}

func TestOpenBucketFromURL(t *testing.T) {
	const subdir = "mysubdir"
	dir := filepath.Join(os.TempDir(), "fileblob")
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, subdir), os.ModePerm); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(filepath.Join(dir, "myfile.txt"), []byte("hello world"), 0666); err != nil {
		t.Fatal(err)
	}
	// To avoid making another temp dir, use the bucket directory to hold the secret key file.
	secretKeyPath := filepath.Join(dir, "secret.key")
	if err := ioutil.WriteFile(secretKeyPath, []byte("secret key"), 0666); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(filepath.Join(dir, subdir, "myfileinsubdir.txt"), []byte("hello world in subdir"), 0666); err != nil {
		t.Fatal(err)
	}
	// Convert dir to a URL path, adding a leading "/" if needed on Windows.
	dirpath := filepath.ToSlash(dir)
	if os.PathSeparator != '/' && !strings.HasPrefix(dirpath, "/") {
		dirpath = "/" + dirpath
	}

	tests := []struct {
		URL         string
		Key         string
		WantErr     bool
		WantReadErr bool
		Want        string
	}{
		// Bucket doesn't exist -> error at construction time.
		{"file:///bucket-not-found", "", true, false, ""},
		// File doesn't exist -> error at read time.
		{"file://" + dirpath, "filenotfound.txt", false, true, ""},
		// Relative path using host="."; bucket is created but error at read time.
		{"file://./../..", "filenotfound.txt", false, true, ""},
		// OK.
		{"file://" + dirpath, "myfile.txt", false, false, "hello world"},
		// OK, host is ignored.
		{"file://localhost" + dirpath, "myfile.txt", false, false, "hello world"},
		// OK, with prefix.
		{"file://" + dirpath + "?prefix=" + subdir + "/", "myfileinsubdir.txt", false, false, "hello world in subdir"},
		// Invalid query parameter.
		{"file://" + dirpath + "?param=value", "myfile.txt", true, false, ""},
		// OK, with params.
		{
			fmt.Sprintf("file://%s?base_url=/show&secret_key_path=%s", dirpath, secretKeyPath),
			"myfile.txt", false, false, "hello world",
		},
		// Bad secret key filename.
		{
			fmt.Sprintf("file://%s?base_url=/show&secret_key_path=%s", dirpath, "bad"),
			"myfile.txt", true, false, "",
		},
		// Missing base_url.
		{
			fmt.Sprintf("file://%s?secret_key_path=%s", dirpath, secretKeyPath),
			"myfile.txt", true, false, "",
		},
		// Missing secret_key_path.
		{"file://" + dirpath + "?base_url=/show", "myfile.txt", true, false, ""},
	}

	ctx := context.Background()
	for i, test := range tests {
		b, err := blob.OpenBucket(ctx, test.URL)
		if b != nil {
			defer b.Close()
		}
		if (err != nil) != test.WantErr {
			t.Errorf("#%d: %s: got error %v, want error %v", i, test.URL, err, test.WantErr)
		}
		if err != nil {
			continue
		}
		got, err := b.ReadAll(ctx, test.Key)
		if (err != nil) != test.WantReadErr {
			t.Errorf("%s: got read error %v, want error %v", test.URL, err, test.WantReadErr)
		}
		if err != nil {
			continue
		}
		if string(got) != test.Want {
			t.Errorf("%s: got %q want %q", test.URL, got, test.Want)
		}
	}
}
