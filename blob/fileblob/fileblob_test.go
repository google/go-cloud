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
	"runtime"
	"strings"
	"testing"

	"gocloud.dev/blob"
	"gocloud.dev/blob/driver"
	"gocloud.dev/blob/drivertest"
	"gocloud.dev/gcerrors"
)

type harness struct {
	dir         string
	prefix      string
	metadataHow metadataOption
	noTempDir   bool
	server      *httptest.Server
	urlSigner   URLSigner
	closer      func()
}

func newHarness(ctx context.Context, t *testing.T, prefix string, metadataHow metadataOption, noTempDir bool) (drivertest.Harness, error) {
	if metadataHow == MetadataDontWrite {
		// Skip tests for if no metadata gets written.
		// For these it is currently undefined whether any gets read (back).
		switch name := t.Name(); {
		case strings.Contains(name, "ContentType"), strings.HasSuffix(name, "TestAttributes"), strings.Contains(name, "TestMetadata/"):
			t.SkipNow()
			return nil, nil
		}
	}

	dir := filepath.Join(os.TempDir(), "go-cloud-fileblob")
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return nil, err
	}
	if prefix != "" {
		if err := os.MkdirAll(filepath.Join(dir, prefix), os.ModePerm); err != nil {
			return nil, err
		}
	}
	h := &harness{dir: dir, prefix: prefix, metadataHow: metadataHow, noTempDir: noTempDir}

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
		Metadata:  h.metadataHow,
		NoTempDir: h.noTempDir,
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

func (h *harness) MakeDriverForNonexistentBucket(ctx context.Context) (driver.Bucket, error) {
	// Does not make sense for this driver, as it verifies
	// that the directory exists in OpenBucket.
	return nil, nil
}

func (h *harness) Close() {
	h.closer()
}

func TestConformance(t *testing.T) {
	newHarnessNoPrefix := func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
		return newHarness(ctx, t, "", MetadataInSidecar, false)
	}
	drivertest.RunConformanceTests(t, newHarnessNoPrefix, []drivertest.AsTest{verifyAs{}})
}

func TestConformanceNoTempDir(t *testing.T) {
	newHarnessNoTmpDir := func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
		return newHarness(ctx, t, "", MetadataInSidecar, true)
	}
	drivertest.RunConformanceTests(t, newHarnessNoTmpDir, []drivertest.AsTest{verifyAs{}})
}

func TestConformanceWithPrefix(t *testing.T) {
	const prefix = "some/prefix/dir/"
	newHarnessWithPrefix := func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
		return newHarness(ctx, t, prefix, MetadataInSidecar, false)
	}
	drivertest.RunConformanceTests(t, newHarnessWithPrefix, []drivertest.AsTest{verifyAs{prefix: prefix}})
}

func TestConformanceSkipMetadata(t *testing.T) {
	newHarnessSkipMetadata := func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
		return newHarness(ctx, t, "", MetadataDontWrite, false)
	}
	drivertest.RunConformanceTests(t, newHarnessSkipMetadata, []drivertest.AsTest{verifyAs{}})
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
	t.Run("BucketDirMissingWithCreateDir", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "fileblob")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		b, gotErr := OpenBucket(filepath.Join(dir, "notfound"), &Options{CreateDir: true})
		if gotErr != nil {
			t.Errorf("got error %v", gotErr)
		}
		defer b.Close()

		// Make sure the subdir has gotten permissions to be used.
		gotErr = b.WriteAll(context.Background(), "key", []byte("delme"), nil)
		if gotErr != nil {
			t.Errorf("got error writing to bucket from CreateDir %v", gotErr)
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

func TestSignedURLReturnsUnimplementedWithNoURLSigner(t *testing.T) {
	dir, err := ioutil.TempDir("", "fileblob")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	b, err := OpenBucket(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	_, gotErr := b.SignedURL(context.Background(), "key", nil)
	if gcerrors.Code(gotErr) != gcerrors.Unimplemented {
		t.Errorf("want Unimplemented error, got %v", gotErr)
	}
}

type verifyAs struct {
	prefix string
}

func (verifyAs) Name() string { return "verify As types for fileblob" }

func (verifyAs) BucketCheck(b *blob.Bucket) error {
	var fi os.FileInfo
	if !b.As(&fi) {
		return errors.New("Bucket.As failed")
	}
	return nil
}
func (verifyAs) BeforeRead(as func(interface{}) bool) error {
	var f *os.File
	if !as(&f) {
		return errors.New("BeforeRead.As failed")
	}
	return nil
}
func (verifyAs) BeforeWrite(as func(interface{}) bool) error {
	var f *os.File
	if !as(&f) {
		return errors.New("BeforeWrite.As failed")
	}
	return nil
}
func (verifyAs) BeforeCopy(as func(interface{}) bool) error {
	var f *os.File
	if !as(&f) {
		return errors.New("BeforeCopy.As failed")
	}
	return nil
}
func (verifyAs) BeforeList(as func(interface{}) bool) error { return nil }
func (verifyAs) BeforeSign(as func(interface{}) bool) error { return nil }
func (verifyAs) AttributesCheck(attrs *blob.Attributes) error {
	var fi os.FileInfo
	if !attrs.As(&fi) {
		return errors.New("Attributes.As failed")
	}
	return nil
}
func (verifyAs) ReaderCheck(r *blob.Reader) error {
	var ior io.Reader
	if !r.As(&ior) {
		return errors.New("Reader.As failed")
	}
	return nil
}
func (verifyAs) ListObjectCheck(o *blob.ListObject) error {
	var fi os.FileInfo
	if !o.As(&fi) {
		return errors.New("ListObject.As failed")
	}
	return nil
}

func (v verifyAs) ErrorCheck(b *blob.Bucket, err error) error {
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
		// OK, with no_tmp_dir.
		{"file://" + dirpath + "?no_tmp_dir", "myfile.txt", false, false, "hello world"},
		// OK, host is ignored.
		{"file://localhost" + dirpath, "myfile.txt", false, false, "hello world"},
		// OK, with prefix.
		{"file://" + dirpath + "?prefix=" + subdir + "/", "myfileinsubdir.txt", false, false, "hello world in subdir"},
		// Subdir does not exist.
		{"file://" + dirpath + "subdir", "", true, false, ""},
		// Subdir does not exist, but create_dir creates it. Error is at file read time.
		{"file://" + dirpath + "subdir2?create_dir=true", "filenotfound.txt", false, true, ""},
		// Invalid query parameter.
		{"file://" + dirpath + "?param=value", "myfile.txt", true, false, ""},
		// Unrecognized value for parameter "metadata".
		{"file://" + dirpath + "?metadata=nosuchstrategy", "myfile.txt", true, false, ""},
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

func TestListAtRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("/ as root is a unix concept")
	}

	ctx := context.Background()
	b, err := OpenBucket("/", nil)
	if err != nil {
		t.Fatalf("Got error creating bucket; %#v", err)
	}
	defer b.Close()

	dir, err := ioutil.TempDir("", "fileblob")
	if err != nil {
		t.Fatalf("Got error creating temp dir: %#v", err)
	}
	f, err := os.Create(filepath.Join(dir, "file.txt"))
	if err != nil {
		t.Fatalf("Got error creating file: %#v", err)
	}
	defer f.Close()

	it := b.List(&blob.ListOptions{
		Prefix: dir[1:],
	})
	obj, err := it.Next(ctx)
	if err != nil {
		t.Fatalf("Got error reading next item from list: %#v", err)
	}
	if obj.Key != filepath.Join(dir, "file.txt")[1:] {
		t.Fatalf("Got unexpected filename in list: %q", obj.Key)
	}
	_, err = it.Next(ctx)
	if err != io.EOF {
		t.Fatalf("Expecting an EOF on next item in list, got: %#v", err)
	}
}

func TestSkipMetadata(t *testing.T) {
	dir, err := ioutil.TempDir("", "fileblob*")
	if err != nil {
		t.Fatalf("Got error creating temp dir: %#v", err)
	}
	defer os.RemoveAll(dir)
	dirpath := filepath.ToSlash(dir)
	if os.PathSeparator != '/' && !strings.HasPrefix(dirpath, "/") {
		dirpath = "/" + dirpath
	}

	tests := []struct {
		URL         string
		wantSidecar bool
	}{
		{"file://" + dirpath + "?metadata=skip", false},
		{"file://" + dirpath, true},                // Implicitly sets the default strategy…
		{"file://" + dirpath + "?metadata=", true}, // … and explicitly.
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, test := range tests {
		b, err := blob.OpenBucket(ctx, test.URL)
		if b != nil {
			defer b.Close()
		}
		if err != nil {
			t.Fatal(err)
		}

		err = b.WriteAll(ctx, "key", []byte("hello world"), &blob.WriterOptions{
			ContentType: "text/plain",
		})
		if err != nil {
			t.Fatal(err)
		}

		_, err = os.Stat(filepath.Join(dir, "key"+attrsExt))
		if gotSidecar := !errors.Is(err, os.ErrNotExist); test.wantSidecar != gotSidecar {
			t.Errorf("Metadata sidecar file (extension %s) exists: %v, did we want it: %v",
				attrsExt, gotSidecar, test.wantSidecar)
		}
		b.Delete(ctx, "key")
	}
}
