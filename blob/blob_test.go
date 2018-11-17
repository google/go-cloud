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
	db := &fakeBucket{pages: [][]string{
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

type fakeBucket struct {
	driver.Bucket

	pages [][]string
}

func (b *fakeBucket) ListPaged(ctx context.Context, opts *driver.ListOptions) (*driver.ListPage, error) {
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
