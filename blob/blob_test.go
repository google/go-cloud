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

package blob

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/blob/driver"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/gcerr"
)

var (
	errFake     = errors.New("fake")
	errNotFound = errors.New("fake not found")
)

func TestExists(t *testing.T) {
	tests := []struct {
		Description string
		Err         error
		Want        bool
		WantErr     bool
	}{
		{
			Description: "no error -> exists",
			Err:         nil,
			Want:        true,
			WantErr:     false,
		},
		{
			Description: "notfound error -> !exists",
			Err:         errNotFound,
			Want:        false,
			WantErr:     false,
		},
		{
			Description: "other error -> error",
			Err:         errFake,
			Want:        false,
			WantErr:     true,
		},
	}

	for _, test := range tests {
		t.Run(test.Description, func(t *testing.T) {
			drv := &fakeAttributes{attributesErr: test.Err}
			b := NewBucket(drv)
			defer b.Close()
			got, gotErr := b.Exists(context.Background(), "key")
			if got != test.Want {
				t.Errorf("got %v want %v", got, test.Want)
			}
			if (gotErr != nil) != test.WantErr {
				t.Errorf("got err %v want %v", gotErr, test.WantErr)
			}
		})
	}
}

// fakeAttributes implements driver.Bucket. Only Attributes is implemented,
// returning a zero Attributes struct and attributesErr.
type fakeAttributes struct {
	driver.Bucket
	attributesErr error
}

func (b *fakeAttributes) Attributes(ctx context.Context, key string) (*driver.Attributes, error) {
	if b.attributesErr != nil {
		return nil, b.attributesErr
	}
	return &driver.Attributes{}, nil
}

func (b *fakeAttributes) ErrorCode(err error) gcerrors.ErrorCode {
	if err == errNotFound {
		return gcerrors.NotFound
	}
	return gcerrors.Unknown
}

func (b *fakeAttributes) Close() error { return nil }

// Verify that ListIterator works even if driver.ListPaged returns empty pages.
func TestListIterator(t *testing.T) {
	ctx := context.Background()
	want := []string{"a", "b", "c"}
	db := &fakeLister{
		pages:         [][]string{{"a"}, {}, {}, {"b", "c"}, {}, {}},
		wantPageSizes: []int{0, 0, 0, 0, 0, 0},
	}
	b := NewBucket(db)
	defer b.Close()
	iter := b.List(nil)
	var got []string
	for {
		obj, err := iter.Next(ctx)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, obj.Key)
	}
	if !cmp.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// Verify that ListPage works even if driver.ListPaged returns empty pages.
func TestListPage(t *testing.T) {
	ctx := context.Background()
	want := [][]string{{"a", "b"}, {"c", "d"}, {"e"}}
	db := &fakeLister{
		pages:         [][]string{{}, {"a", "b"}, {}, {}, {"c"}, {}, {"d"}, {}, {}, {"e"}},
		wantPageSizes: []int{2, 2, 2, 2, 2, 1, 1, 2, 2, 2},
	}
	b := NewBucket(db)
	defer b.Close()

	nextToken := FirstPageToken
	got := [][]string{}
	for {
		page, token, err := b.ListPage(ctx, nextToken, 2, nil)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		gotPage := make([]string, len(page))
		for i, o := range page {
			gotPage[i] = o.Key
		}
		got = append(got, gotPage)
		nextToken = token
	}
	if !cmp.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// fakeLister implements driver.Bucket. Only ListPaged is implemented,
// returning static data from pages.
type fakeLister struct {
	driver.Bucket
	pages         [][]string
	wantPageSizes []int
}

func (b *fakeLister) ListPaged(ctx context.Context, opts *driver.ListOptions) (*driver.ListPage, error) {
	if len(b.pages) != len(b.wantPageSizes) {
		return nil, fmt.Errorf("invalid fakeLister setup")
	}
	if len(b.pages) == 0 {
		return &driver.ListPage{}, nil
	}
	page := b.pages[0]
	wantPageSize := b.wantPageSizes[0]
	b.pages = b.pages[1:]
	b.wantPageSizes = b.wantPageSizes[1:]
	if opts.PageSize != wantPageSize {
		return nil, fmt.Errorf("got page size %d, want %d", opts.PageSize, wantPageSize)
	}
	var objs []*driver.ListObject
	for _, key := range page {
		objs = append(objs, &driver.ListObject{Key: key})
	}
	return &driver.ListPage{Objects: objs, NextPageToken: []byte{1}}, nil
}

func (*fakeLister) Close() error                           { return nil }
func (*fakeLister) ErrorCode(err error) gcerrors.ErrorCode { return gcerrors.Unknown }

type stubReader struct {
	driver.Reader
	downloaded bool
}

func (r *stubReader) Download(w io.Writer) error {
	r.downloaded = true
	return nil
}

func (*stubReader) Close() error { return nil }

type stubWriter struct {
	driver.Writer
	uploaded bool
}

func (w *stubWriter) Upload(r io.Reader) error {
	w.uploaded = true
	return nil
}

func (*stubWriter) Close() error { return nil }

// loaderBucket implements driver.Bucket's NewTypedWriter and NewRangedReader methods,
// returning stubReader and stubWriter. It is used to verify that the special driver.Uploader
// and driver.Downloader overrides work when called.
type loaderBucket struct {
	driver.Bucket
	w stubWriter
	r stubReader
}

func (b *loaderBucket) NewTypedWriter(ctx context.Context, key, contentType string, opts *driver.WriterOptions) (driver.Writer, error) {
	return &b.w, nil
}

func (b *loaderBucket) NewRangeReader(ctx context.Context, key string, offset, length int64, opts *driver.ReaderOptions) (driver.Reader, error) {
	return &b.r, nil
}

func (*loaderBucket) Close() error { return nil }

func TestUploader(t *testing.T) {
	ctx := context.Background()
	lb := &loaderBucket{}
	b := NewBucket(lb)
	defer b.Close()
	err := b.Upload(ctx, "key", nil, &WriterOptions{ContentType: "text/html"})
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	if !lb.w.uploaded {
		t.Error("Uploader wasn't called")
	}
}

func TestDownloader(t *testing.T) {
	ctx := context.Background()
	lb := &loaderBucket{}
	b := NewBucket(lb)
	defer b.Close()
	err := b.Download(ctx, "key", nil, nil)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}
	if !lb.r.downloaded {
		t.Error("Downloader wasn't called")
	}
}

func TestSeekAfterReadFailure(t *testing.T) {
	const filename = "f.txt"

	ctx := context.Background()

	bucket := NewBucket(&oneTimeReadBucket{first: true})
	defer bucket.Close()

	reader, err := bucket.NewRangeReader(ctx, filename, 0, 100, nil)
	if err != nil {
		t.Fatalf("failed NewRangeReader: %v", err)
	}
	defer reader.Close()

	b := make([]byte, 10)

	_, err = reader.Read(b)
	if err != nil {
		t.Fatalf("failed Read#1: %v", err)
	}

	_, err = reader.Seek(0, io.SeekStart)
	if err != nil {
		t.Fatalf("failed Seek#1: %v", err)
	}

	// This Read will force a recreation of the reader via NewRangeReader,
	// which will fail.
	_, err = reader.Read(b)
	if err == nil {
		t.Fatalf("unexpectedly succeeded Read#2: %v", err)
	}

	_, err = reader.Seek(0, io.SeekStart)
	if err != nil {
		t.Fatalf("failed Seek#2: %v", err)
	}
}

// oneTimeReadBucket implements driver.Bucket for TestSeekAfterReadFailure.
// It returns a fake reader that succeeds once, then fails.
type oneTimeReadBucket struct {
	driver.Bucket
	first bool
}

type workingReader struct {
	driver.Reader
}

func (r *workingReader) Read(p []byte) (int, error) {
	return len(p), nil
}

func (r *workingReader) Attributes() *driver.ReaderAttributes { return &driver.ReaderAttributes{} }
func (r *workingReader) Close() error                         { return nil }

func (b *oneTimeReadBucket) NewRangeReader(ctx context.Context, key string, offset, length int64, opts *driver.ReaderOptions) (driver.Reader, error) {
	if b.first {
		b.first = false
		return &workingReader{}, nil
	}
	return nil, errFake
}

func (b *oneTimeReadBucket) ErrorCode(err error) gcerrors.ErrorCode { return gcerrors.Unknown }
func (b *oneTimeReadBucket) Close() error                           { return nil }

// erroringBucket implements driver.Bucket. All interface methods that return
// errors are implemented, and return errFake.
// In addition, when passed the key "work", NewRangeReader and NewTypedWriter
// will return a Reader/Writer respectively, that always return errFake
// from Read/Write and Close.
type erroringBucket struct {
	driver.Bucket
}

type erroringReader struct {
	driver.Reader
}

func (r *erroringReader) Read(p []byte) (int, error) {
	return 0, errFake
}

func (r *erroringReader) Close() error {
	return errFake
}

type erroringWriter struct {
	driver.Writer
}

func (r *erroringWriter) Write(p []byte) (int, error) {
	return 0, errFake
}

func (r *erroringWriter) Close() error {
	return errFake
}

func (b *erroringBucket) Attributes(ctx context.Context, key string) (*driver.Attributes, error) {
	return nil, errFake
}

func (b *erroringBucket) ListPaged(ctx context.Context, opts *driver.ListOptions) (*driver.ListPage, error) {
	return nil, errFake
}

func (b *erroringBucket) NewRangeReader(ctx context.Context, key string, offset, length int64, opts *driver.ReaderOptions) (driver.Reader, error) {
	if key == "work" {
		return &erroringReader{}, nil
	}
	return nil, errFake
}

func (b *erroringBucket) NewTypedWriter(ctx context.Context, key, contentType string, opts *driver.WriterOptions) (driver.Writer, error) {
	if key == "work" {
		return &erroringWriter{}, nil
	}
	return nil, errFake
}

func (b *erroringBucket) Copy(ctx context.Context, dstKey, srcKey string, opts *driver.CopyOptions) error {
	return errFake
}

func (b *erroringBucket) Delete(ctx context.Context, key string) error {
	return errFake
}

func (b *erroringBucket) SignedURL(ctx context.Context, key string, opts *driver.SignedURLOptions) (string, error) {
	return "", errFake
}

func (b *erroringBucket) Close() error {
	return errFake
}

func (b *erroringBucket) ErrorCode(err error) gcerrors.ErrorCode {
	return gcerrors.Unknown
}

// TestErrorsAreWrapped tests that all errors returned from the driver are
// wrapped exactly once by the portable type.
func TestErrorsAreWrapped(t *testing.T) {
	ctx := context.Background()
	buf := bytes.Repeat([]byte{'A'}, sniffLen)
	b := NewBucket(&erroringBucket{})

	// verifyWrap ensures that err is wrapped exactly once.
	verifyWrap := func(description string, err error) {
		if err == nil {
			t.Errorf("%s: got nil error, wanted non-nil", description)
			return
		}
		if _, ok := err.(*gcerr.Error); !ok {
			t.Errorf("%s: not wrapped: %v", description, err)
		}
		if s := err.Error(); !strings.HasPrefix(s, "blob ") {
			t.Logf("short form of error: %v", err)
			t.Logf("with details: %+v", err)
			t.Errorf("%s: Error() for wrapped error doesn't start with blob: prefix: %s", description, s)
		}
	}

	_, err := b.Attributes(ctx, "")
	verifyWrap("Attributes", err)

	iter := b.List(nil)
	_, err = iter.Next(ctx)
	verifyWrap("ListIterator.Next", err)

	_, err = b.NewRangeReader(ctx, "", 0, 1, nil)
	verifyWrap("NewRangeReader", err)
	_, err = b.ReadAll(ctx, "")
	verifyWrap("ReadAll", err)

	// Providing ContentType means driver.NewTypedWriter is called right away.
	_, err = b.NewWriter(ctx, "", &WriterOptions{ContentType: "foo"})
	verifyWrap("NewWriter", err)
	err = b.WriteAll(ctx, "", buf, &WriterOptions{ContentType: "foo"})
	verifyWrap("WriteAll", err)

	// Not providing ContentType means driver.NewTypedWriter is only called
	// after writing sniffLen bytes.
	w, _ := b.NewWriter(ctx, "", nil)
	_, err = w.Write(buf)
	verifyWrap("NewWriter (no ContentType)", err)
	w.Close()
	err = b.WriteAll(ctx, "", buf, nil)
	verifyWrap("WriteAll (no ContentType)", err)

	r, _ := b.NewRangeReader(ctx, "work", 0, 1, nil)
	_, err = r.Read(buf)
	verifyWrap("Reader.Read", err)

	err = r.Close()
	verifyWrap("Reader.Close", err)

	w, _ = b.NewWriter(ctx, "work", &WriterOptions{ContentType: "foo"})
	_, err = w.Write(buf)
	verifyWrap("Writer.Write", err)

	err = w.Close()
	verifyWrap("Writer.Close", err)

	err = b.Copy(ctx, "", "", nil)
	verifyWrap("Copy", err)

	err = b.Delete(ctx, "")
	verifyWrap("Delete", err)

	_, err = b.SignedURL(ctx, "", nil)
	verifyWrap("SignedURL", err)

	err = b.Close()
	verifyWrap("Close", err)
}

var (
	testOpenOnce sync.Once
	testOpenGot  *url.URL
)

// TestBucketIsClosed verifies that all Bucket functions return an error
// if the Bucket is closed.
func TestBucketIsClosed(t *testing.T) {
	ctx := context.Background()
	buf := bytes.Repeat([]byte{'A'}, sniffLen)

	bucket := NewBucket(&erroringBucket{})
	bucket.Close()

	if _, err := bucket.Attributes(ctx, ""); err != errClosed {
		t.Error(err)
	}
	iter := bucket.List(nil)
	if _, err := iter.Next(ctx); err != errClosed {
		t.Error(err)
	}

	if _, err := bucket.NewRangeReader(ctx, "", 0, 1, nil); err != errClosed {
		t.Error(err)
	}
	if _, err := bucket.ReadAll(ctx, ""); err != errClosed {
		t.Error(err)
	}
	if _, err := bucket.NewWriter(ctx, "", nil); err != errClosed {
		t.Error(err)
	}
	if err := bucket.WriteAll(ctx, "", buf, nil); err != errClosed {
		t.Error(err)
	}
	if _, err := bucket.NewRangeReader(ctx, "work", 0, 1, nil); err != errClosed {
		t.Error(err)
	}
	if err := bucket.Copy(ctx, "", "", nil); err != errClosed {
		t.Error(err)
	}
	if err := bucket.Delete(ctx, ""); err != errClosed {
		t.Error(err)
	}
	if _, err := bucket.SignedURL(ctx, "", nil); err != errClosed {
		t.Error(err)
	}
	if err := bucket.Close(); err != errClosed {
		t.Error(err)
	}
}

func TestURLMux(t *testing.T) {
	ctx := context.Background()

	mux := new(URLMux)
	fake := &fakeOpener{}
	mux.RegisterBucket("foo", fake)
	mux.RegisterBucket("err", fake)

	if diff := cmp.Diff(mux.BucketSchemes(), []string{"err", "foo"}); diff != "" {
		t.Errorf("Schemes: %s", diff)
	}
	if !mux.ValidBucketScheme("foo") || !mux.ValidBucketScheme("err") {
		t.Errorf("ValidBucketScheme didn't return true for valid scheme")
	}
	if mux.ValidBucketScheme("foo2") || mux.ValidBucketScheme("http") {
		t.Errorf("ValidBucketScheme didn't return false for invalid scheme")
	}

	for _, tc := range []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "empty URL",
			wantErr: true,
		},
		{
			name:    "invalid URL",
			url:     ":foo",
			wantErr: true,
		},
		{
			name:    "invalid URL no scheme",
			url:     "foo",
			wantErr: true,
		},
		{
			name:    "unregistered scheme",
			url:     "bar://mybucket",
			wantErr: true,
		},
		{
			name:    "func returns error",
			url:     "err://mybucket",
			wantErr: true,
		},
		{
			name: "no query options",
			url:  "foo://mybucket",
		},
		{
			name: "empty query options",
			url:  "foo://mybucket?",
		},
		{
			name: "query options",
			url:  "foo://mybucket?aAa=bBb&cCc=dDd",
		},
		{
			name: "multiple query options",
			url:  "foo://mybucket?x=a&x=b&x=c",
		},
		{
			name: "fancy bucket name",
			url:  "foo:///foo/bar/baz",
		},
		{
			name: "using api scheme prefix",
			url:  "blob+foo:///foo/bar/baz",
		},
		{
			name: "using api+type scheme prefix",
			url:  "blob+bucket+foo:///foo/bar/baz",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, gotErr := mux.OpenBucket(ctx, tc.url)
			if (gotErr != nil) != tc.wantErr {
				t.Fatalf("got err %v, want error %v", gotErr, tc.wantErr)
			}
			if gotErr != nil {
				return
			}
			if got := fake.u.String(); got != tc.url {
				t.Errorf("got %q want %q", got, tc.url)
			}
			// Repeat with OpenBucketURL.
			parsed, err := url.Parse(tc.url)
			if err != nil {
				t.Fatal(err)
			}
			_, gotErr = mux.OpenBucketURL(ctx, parsed)
			if gotErr != nil {
				t.Fatalf("got err %v want nil", gotErr)
			}
			if got := fake.u.String(); got != tc.url {
				t.Errorf("got %q want %q", got, tc.url)
			}
		})
	}
}

type fakeOpener struct {
	u *url.URL // last url passed to OpenBucketURL
}

func (o *fakeOpener) OpenBucketURL(ctx context.Context, u *url.URL) (*Bucket, error) {
	if u.Scheme == "err" {
		return nil, errors.New("fail")
	}
	o.u = u
	return nil, nil
}
