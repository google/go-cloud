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

package docstore

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/docstore/driver"
	"gocloud.dev/gcerrors"
)

type Book struct {
	Title            string `docstore:"key"`
	Author           Name   `docstore:"author"`
	PublicationYears []int  `docstore:"pub_years,omitempty"`
	NumPublications  int    `docstore:"-"`
}

type Name struct {
	First, Last string
}

func TestIsIncNumber(t *testing.T) {
	for _, x := range []interface{}{int(1), 'x', uint(1), byte(1), float32(1), float64(1), time.Duration(1)} {
		if !isIncNumber(x) {
			t.Errorf("%v: got false, want true", x)
		}
	}
	for _, x := range []interface{}{1 + 1i, "3", time.Time{}} {
		if isIncNumber(x) {
			t.Errorf("%v: got true, want false", x)
		}
	}
}

func TestActionsDo(t *testing.T) {
	c := newCollection(fakeDriverCollection{})
	defer c.Close()
	dn := map[string]interface{}{"key": nil}
	d1 := map[string]interface{}{"key": 1}
	d2 := map[string]interface{}{"key": 2}
	dsn := &Book{}
	ds1 := &Book{Title: "The Master and Margarita"}
	ds2 := &Book{Title: "The Martian"}

	for _, test := range []struct {
		alist *ActionList
		want  []int // error indexes; nil if no error
	}{
		{c.Actions().Get(d1).Get(d2).Get(ds1).Get(ds2), nil},
		{c.Actions().Get(d1).Put(d1).Put(ds1).Get(ds1), nil},
		{c.Actions().Get(d2).Replace(d1).Put(d2).Get(d1), nil},
		{c.Actions().Get(ds2).Replace(ds1).Put(ds2).Get(ds1), nil},
		// Missing keys.
		{c.Actions().Put(dn).Put(dsn), []int{0, 1}},
		{c.Actions().Get(dn).Replace(dn).Create(dn).Update(dn, Mods{"a": 1}), []int{0, 1, 3}},
		{c.Actions().Get(dsn).Replace(dsn).Create(dsn).Update(dsn, Mods{"a": 1}), []int{0, 1, 3}},
		// Duplicate documents.
		{c.Actions().Create(dn).Create(dn).Create(dsn).Create(dsn), nil}, // each Create without a key is a separate document
		{c.Actions().Create(d2).Create(ds2).Get(d2).Get(ds2).Create(d2).Put(ds2), []int{4, 5}},
		{c.Actions().Get(d1).Get(ds1).Get(d1).Get(ds1), []int{2, 3}},
		{c.Actions().Put(d1).Put(ds1).Get(d1).Get(ds1).Get(d1).Get(ds1), []int{4, 5}},
		{c.Actions().Get(d1).Get(ds1).Put(d1).Put(d2).Put(ds1).Put(ds2).Put(d1).Replace(ds1), []int{6, 7}},
		{c.Actions().Create(dn).Create(d1).Create(dsn).Create(ds1).Get(d1).Get(ds1), nil},
		// Get with field paths.
		{c.Actions().Get(d1, "a.b", "c"), nil},
		{c.Actions().Get(ds1, "name.Last", "pub_years"), nil},
		{c.Actions().Get(d1, ".c").Get(ds1, "").Get(ds2, "\xa0\xa1"), []int{0, 1, 2}}, // bad field path
		// Mods.
		{c.Actions().Update(d1, nil).Update(ds1, nil), []int{0, 1}},                                                 // empty mod
		{c.Actions().Update(d1, Mods{"a.b.c": 1, "a.b": 2, "a.b+c": 3}), []int{0}},                                  // a.b is a prefix of a.b.c
		{c.Actions().Update(d1, Mods{"": 1}).Update(ds1, Mods{".f": 2}), []int{0, 1}},                               // invalid field path
		{c.Actions().Update(d1, Mods{"a": Increment(true)}).Update(ds1, Mods{"name": Increment("b")}), []int{0, 1}}, // invalid incOp
	} {
		err := test.alist.Do(context.Background())
		if err == nil {
			if len(test.want) > 0 {
				t.Errorf("%s: got nil, want error", test.alist)
			}
			continue
		}
		var got []int
		for _, e := range err.(ActionListError) {
			if gcerrors.Code(e.Err) != gcerrors.InvalidArgument {
				t.Errorf("%s: got %v, want InvalidArgument", test.alist, e.Err)
			}
			got = append(got, e.Index)
		}
		if !cmp.Equal(got, test.want) {
			t.Errorf("%s: got %v, want %v", test.alist, got, test.want)
		}
	}
}

func TestClosedErrors(t *testing.T) {
	// Check that all collection methods return errClosed if the collection is closed.
	ctx := context.Background()
	c := NewCollection(fakeDriverCollection{})
	if err := c.Close(); err != nil {
		t.Fatalf("got %v, want nil", err)
	}

	check := func(err error) {
		t.Helper()
		if alerr, ok := err.(ActionListError); ok {
			err = alerr.Unwrap()
		}
		if err != errClosed {
			t.Errorf("got %v, want errClosed", err)
		}
	}

	doc := map[string]interface{}{"key": "k"}
	check(c.Close())
	check(c.Actions().Create(doc).Do(ctx))
	check(c.Create(ctx, doc))
	check(c.Replace(ctx, doc))
	check(c.Put(ctx, doc))
	check(c.Get(ctx, doc))
	check(c.Delete(ctx, doc))
	check(c.Update(ctx, doc, Mods{"a": 1}))
	iter := c.Query().Get(ctx)
	check(iter.Next(ctx, doc))

	// Check that DocumentIterator.Next returns errClosed if Close is called
	// in the middle of the iteration.
	c = NewCollection(fakeDriverCollection{})
	iter = c.Query().Get(ctx)
	c.Close()
	check(iter.Next(ctx, doc))
}

func TestSerializeRevisionErrors(t *testing.T) {
	c := NewCollection(fakeDriverCollection{})
	_, err := c.RevisionToString(nil)
	if got := gcerrors.Code(err); got != gcerrors.InvalidArgument {
		t.Errorf("got %v, want InvalidArgument", got)
	}
	_, err = c.StringToRevision("")
	if got := gcerrors.Code(err); got != gcerrors.InvalidArgument {
		t.Errorf("got %v, want InvalidArgument", got)
	}
}

type fakeDriverCollection struct {
	driver.Collection
}

func (fakeDriverCollection) Key(doc driver.Document) (interface{}, error) {
	key, err := doc.GetField("key")
	// TODO(#2589): remove this check once we check for empty key.
	if err != nil || driver.IsEmptyValue(reflect.ValueOf(key)) {
		return nil, err
	}
	return key, nil
}

func (fakeDriverCollection) RevisionField() string { return DefaultRevisionField }

func (fakeDriverCollection) Close() error { return nil }

func (fakeDriverCollection) RunActions(ctx context.Context, actions []*driver.Action, opts *driver.RunActionsOptions) driver.ActionListError {
	return nil
}

func (fakeDriverCollection) RunGetQuery(context.Context, *driver.Query) (driver.DocumentIterator, error) {
	return fakeDriverDocumentIterator{}, nil
}

type fakeDriverDocumentIterator struct {
	driver.DocumentIterator
}

func (fakeDriverDocumentIterator) Next(context.Context, driver.Document) error { return nil }
