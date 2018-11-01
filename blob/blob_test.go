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
	"io"
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
	iter, err := b.List(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
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
