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

package blob

import (
	"context"
	"errors"
	"io"
	"net/url"
	"testing"

	"github.com/google/go-cloud/blob/driver"
	"github.com/google/go-cmp/cmp"
)

// Verify that ListIterator works even if driver.ListPaged returns empty pages.
func TestListIterator(t *testing.T) {
	ctx := context.Background()
	want := []string{"a", "b", "c"}
	db := &fakeLister{pages: [][]string{
		{"a"},
		{},
		{},
		{"b", "c"},
		{},
		{},
	}}
	b := NewBucket(db)
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

// faikeLister implements driver.Bucket. Only ListPaged is implemented,
// returning static data from pages.
type fakeLister struct {
	driver.Bucket
	pages [][]string
}

func (b *fakeLister) ListPaged(ctx context.Context, opts *driver.ListOptions) (*driver.ListPage, error) {
	if len(b.pages) == 0 {
		return &driver.ListPage{}, nil
	}
	page := b.pages[0]
	b.pages = b.pages[1:]
	var objs []*driver.ListObject
	for _, key := range page {
		objs = append(objs, &driver.ListObject{Key: key})
	}
	return &driver.ListPage{Objects: objs, NextPageToken: []byte{1}}, nil
}

var errFake = errors.New("fake")

// fakeError implements driver.Bucket. All interface methods that return
// errors are implemented, and return errFake.
// In addition, when passed the key "work", NewRangedReader and NewTypedWriter
// will return a Reader/Writer respectively, that always return errFake
// from Read/Write and Close.
type fakeErrorer struct {
	driver.Bucket
}

type fakeErrorReader struct {
	driver.Reader
}

func (r *fakeErrorReader) Read(p []byte) (int, error) {
	return 0, errFake
}

func (r *fakeErrorReader) Close() error {
	return errFake
}

type fakeErrorWriter struct {
	driver.Writer
}

func (r *fakeErrorWriter) Write(p []byte) (int, error) {
	return 0, errFake
}

func (r *fakeErrorWriter) Close() error {
	return errFake
}

func (b *fakeErrorer) Attributes(ctx context.Context, key string) (driver.Attributes, error) {
	return driver.Attributes{}, errFake
}

func (b *fakeErrorer) ListPaged(ctx context.Context, opts *driver.ListOptions) (*driver.ListPage, error) {
	return nil, errFake
}

func (b *fakeErrorer) NewRangeReader(ctx context.Context, key string, offset, length int64) (driver.Reader, error) {
	if key == "work" {
		return &fakeErrorReader{}, nil
	}
	return nil, errFake
}

func (b *fakeErrorer) NewTypedWriter(ctx context.Context, key string, contentType string, opts *driver.WriterOptions) (driver.Writer, error) {
	if key == "work" {
		return &fakeErrorWriter{}, nil
	}
	return nil, errFake
}

func (b *fakeErrorer) Delete(ctx context.Context, key string) error {
	return errFake
}

func (b *fakeErrorer) SignedURL(ctx context.Context, key string, opts *driver.SignedURLOptions) (string, error) {
	return "", errFake
}

// TestErrorsAreWrapped tests that all errors returned from the driver are
// wrapped exactly once by the concrete type.
func TestErrorsAreWrapped(t *testing.T) {
	ctx := context.Background()
	b := NewBucket(&fakeErrorer{})

	// verifyWrap ensures that err is wrapped exactly once.
	verifyWrap := func(description string, err error) {
		if unwrapped, ok := err.(*wrappedError); !ok {
			t.Errorf("%s: not wrapped: %v", description, err)
		} else if du, ok := unwrapped.err.(*wrappedError); ok {
			t.Errorf("%s: double wrapped: %v", description, du)
		}
	}

	_, err := b.Attributes(ctx, "")
	verifyWrap("Attributes", err)

	iter := b.List(nil)
	_, err = iter.Next(ctx)
	verifyWrap("ListIterator.Next", err)

	_, err = b.NewRangeReader(ctx, "", 0, 1)
	verifyWrap("NewRangeReader", err)

	_, err = b.NewWriter(ctx, "", &WriterOptions{ContentType: "foo"})
	verifyWrap("NewWriter", err)

	var buf []byte
	r, _ := b.NewRangeReader(ctx, "work", 0, 1)
	_, err = r.Read(buf)
	verifyWrap("Reader.Read", err)

	err = r.Close()
	verifyWrap("Reader.Close", err)

	w, _ := b.NewWriter(ctx, "work", &WriterOptions{ContentType: "foo"})
	_, err = w.Write(buf)
	verifyWrap("Writer.Write", err)

	err = w.Close()
	verifyWrap("Writer.Close", err)

	err = b.Delete(ctx, "")
	verifyWrap("Delete", err)

	_, err = b.SignedURL(ctx, "", nil)
	verifyWrap("SignedURL", err)
}

// TestOpen tests blob.Open.
func TestOpen(t *testing.T) {
	ctx := context.Background()
	var got *url.URL

	// Register scheme foo to always return nil. Sets got as a side effect
	Register("foo", func(_ context.Context, u *url.URL) (driver.Bucket, error) {
		got = u
		return nil, nil
	})

	// Register scheme err to always return an error.
	Register("err", func(_ context.Context, u *url.URL) (driver.Bucket, error) {
		return nil, errors.New("fail")
	})

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
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, gotErr := Open(ctx, tc.url)
			if (gotErr != nil) != tc.wantErr {
				t.Fatalf("got err %v, want error %v", gotErr, tc.wantErr)
			}
			if gotErr != nil {
				return
			}
			want, err := url.Parse(tc.url)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(got, want); diff != "" {
				t.Errorf("got\n%v\nwant\n%v\ndiff\n%s", got, want, diff)
			}
		})
	}
}

func TestCallsWithUnwrappedError(t *testing.T) {
	errFail := errors.New("fail")

	if got := IsNotExist(errFail); got {
		t.Errorf("IsNotExist got true with unwrapped error, wanted false")
	}
	if got := IsNotImplemented(errFail); got {
		t.Errorf("IsNotImplemented got true with unwrapped error, wanted false")
	}
	if got := ErrorAs(errFail, nil); got {
		t.Errorf("ErrorAs got true with unwrapped error, wanted false")
	}
}
