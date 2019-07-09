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
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/docstore/driver"
	"gocloud.dev/gcerrors"
)

func TestToDriverMods(t *testing.T) {
	for _, test := range []struct {
		mods    Mods
		want    []driver.Mod
		wantErr bool
	}{
		{
			Mods{"a": 1, "b": nil},
			[]driver.Mod{{[]string{"a"}, 1}, {[]string{"b"}, nil}},
			false,
		},
		{
			Mods{"a.b": 1, "b.c": nil},
			[]driver.Mod{{[]string{"a", "b"}, 1}, {[]string{"b", "c"}, nil}},
			false,
		},
		// empty mods are an error
		{Mods{}, nil, true},
		// prefixes are not allowed
		{Mods{"a.b.c": 1, "a.b": 2, "a.b+c": 3}, nil, true},
	} {
		got, gotErr := toDriverMods(test.mods)
		if test.wantErr {
			if gcerrors.Code(gotErr) != gcerrors.InvalidArgument {
				t.Errorf("%v: got error '%v', want InvalidArgument", test.mods, gotErr)
			}

		} else if !cmp.Equal(got, test.want) {
			t.Errorf("%v: got %v, want %v", test.mods, got, test.want)
		}
	}
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

func TestToDriverActionsErrors(t *testing.T) {
	c := &Collection{driver: fakeDriverCollection{}}
	dn := map[string]interface{}{"key": nil}
	d1 := map[string]interface{}{"key": 1}
	d2 := map[string]interface{}{"key": 2}

	for _, test := range []struct {
		alist *ActionList
		want  []int // error indexes; nil if no error
	}{
		// Missing keys.
		{c.Actions().Put(dn), []int{0}},
		{c.Actions().Get(dn).Replace(dn).Create(dn).Update(dn, Mods{"a": 1}), []int{0, 1, 3}},
		// Duplicate documents.
		{c.Actions().Get(d1).Get(d2), nil},
		{c.Actions().Get(d1).Put(d1), nil},
		{c.Actions().Get(d2).Replace(d1).Put(d2).Get(d1), nil},
		{c.Actions().Get(d1).Get(d1), []int{1}},
		{c.Actions().Put(d1).Get(d1).Get(d1), []int{2}},
		{c.Actions().Get(d1).Put(d1).Get(d1).Put(d2).Put(d1), []int{2, 4}},
		{c.Actions().Create(d2).Get(d2).Create(d2), []int{2}},
		{c.Actions().Create(dn).Create(dn), nil}, // each Create without a key is a separate document
		{c.Actions().Create(dn).Create(d1).Get(d1), nil},
		{c.Actions().Put(d1).Create(dn).Create(d1).Get(d1), []int{2}},
		{c.Actions().Put(d1).Create(dn).Create(d1).Get(d1), []int{2}},
		{c.Actions().Update(d1, nil), []int{0}}, // empty mod
		// Other errors with mods are tested in TestToDriverMods.
		{c.Actions().Get(d1, "a.b", "c"), nil},
		{c.Actions().Get(d1, ".c"), []int{0}}, // bad field path
	} {
		_, err := test.alist.toDriverActions()
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
	c := newCollection(fakeDriverCollection{})
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
	check(c.Query().Delete(ctx))
	check(c.Query().Update(ctx, Mods{"a": 1}))
	iter := c.Query().Get(ctx)
	check(iter.Next(ctx, doc))

	// Check that DocumentIterator.Next returns errClosed if Close is called
	// in the middle of the iteration.
	c = newCollection(fakeDriverCollection{})
	iter = c.Query().Get(ctx)
	c.Close()
	check(iter.Next(ctx, doc))
}

func TestSerializeRevisionErrors(t *testing.T) {
	c := newCollection(fakeDriverCollection{})
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
	return doc.GetField("key")
}

func (fakeDriverCollection) RevisionField() string { return DefaultRevisionField }

func (fakeDriverCollection) Close() error { return nil }

func (fakeDriverCollection) RunGetQuery(context.Context, *driver.Query) (driver.DocumentIterator, error) {
	return fakeDriverDocumentIterator{}, nil
}

type fakeDriverDocumentIterator struct {
	driver.DocumentIterator
}

func (fakeDriverDocumentIterator) Next(context.Context, driver.Document) error { return nil }
