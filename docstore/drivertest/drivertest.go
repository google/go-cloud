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

// Package drivertest provides a conformance test for implementations of
// driver.
package drivertest // import "gocloud.dev/docstore/drivertest"

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gocloud.dev/docstore"
	ds "gocloud.dev/docstore"
	"gocloud.dev/docstore/driver"
	"gocloud.dev/gcerrors"
)

// CollectionKind describes the kind of testing collection to create.
type CollectionKind int

const (
	// A collection with a single primary key field of type string named
	// drivertest.KeyField.
	SingleKey CollectionKind = iota

	// A collection that will consist entirely of HighScore structs (see below),
	// whose two primary key fields are "Game" and "Player", both strings. Use
	// drivertest.HighScoreKey as the key function.
	TwoKey

	// The collection should behave like a SingleKey collection, except
	// that the revision field should be drivertest.AlternateRevisionField.
	AltRev

	// The collection's documents will not have a revision field.
	NoRev
)

// Harness descibes the functionality test harnesses must provide to run
// conformance tests.
type Harness interface {
	// MakeCollection makes a driver.Collection for testing.
	MakeCollection(context.Context, CollectionKind) (driver.Collection, error)

	// BeforeDoTypes should return a list of values whose types are valid for the as
	// function given to BeforeDo. For example, if the driver converts Get actions
	// to *GetRequests and write actions to *WriteRequests, then BeforeDoTypes should
	// return []interface{}{&GetRequest{}, &WriteRequest{}}.
	// TODO(jba): consider splitting these by action kind.
	BeforeDoTypes() []interface{}

	// BeforeQueryTypes should return a list of values whose types are valid for the as
	// function given to BeforeQuery.
	BeforeQueryTypes() []interface{}

	// RevisionsEqual reports whether two revisions are equal.
	RevisionsEqual(rev1, rev2 interface{}) bool

	// Close closes resources used by the harness.
	Close()
}

// HarnessMaker describes functions that construct a harness for running tests.
// It is called exactly once per test; Harness.Close() will be called when the test is complete.
type HarnessMaker func(ctx context.Context, t *testing.T) (Harness, error)

// UnsupportedType is an enum for types not supported by native codecs. We chose
// to describe this negatively (types that aren't supported rather than types
// that are) to make the more inclusive cases easier to write. A driver can
// return nil for CodecTester.UnsupportedTypes, then add values from this enum
// one by one until all tests pass.
type UnsupportedType int

// These are known unsupported types by one or more driver. Each of them
// corresponses to an unsupported type specific test which if the driver
// actually supports.
const (
	// Native codec doesn't support any unsigned integer type
	Uint UnsupportedType = iota
	// Native codec doesn't support arrays
	Arrays
	// Native codec doesn't support full time precision
	NanosecondTimes
	// Native codec doesn't support [][]byte
	BinarySet
)

// CodecTester describes functions that encode and decode values using both the
// docstore codec for a driver, and that driver's own "native" codec.
type CodecTester interface {
	UnsupportedTypes() []UnsupportedType
	NativeEncode(interface{}) (interface{}, error)
	NativeDecode(value, dest interface{}) error
	DocstoreEncode(interface{}) (interface{}, error)
	DocstoreDecode(value, dest interface{}) error
}

// AsTest represents a test of As functionality.
type AsTest interface {
	// Name should return a descriptive name for the test.
	Name() string
	// CollectionCheck will be called to allow verification of Collection.As.
	CollectionCheck(coll *docstore.Collection) error
	// QueryCheck will be called after calling Query. It should call it.As and
	// verify the results.
	QueryCheck(it *docstore.DocumentIterator) error
	// ErrorCheck is called to allow verification of Collection.ErrorAs.
	ErrorCheck(c *docstore.Collection, err error) error
}

type verifyAsFailsOnNil struct{}

func (verifyAsFailsOnNil) Name() string {
	return "verify As returns false when passed nil"
}

func (verifyAsFailsOnNil) CollectionCheck(coll *docstore.Collection) error {
	if coll.As(nil) {
		return errors.New("want Collection.As to return false when passed nil")
	}
	return nil
}

func (verifyAsFailsOnNil) QueryCheck(it *docstore.DocumentIterator) error {
	if it.As(nil) {
		return errors.New("want DocumentIterator.As to return false when passed nil")
	}
	return nil
}

func (v verifyAsFailsOnNil) ErrorCheck(c *docstore.Collection, err error) (ret error) {
	defer func() {
		if recover() == nil {
			ret = errors.New("want ErrorAs to panic when passed nil")
		}
	}()
	c.ErrorAs(err, nil)
	return nil
}

// RunConformanceTests runs conformance tests for driver implementations of docstore.
func RunConformanceTests(t *testing.T, newHarness HarnessMaker, ct CodecTester, asTests []AsTest) {
	t.Run("TypeDrivenCodec", func(t *testing.T) { testTypeDrivenDecode(t, ct) })
	t.Run("BlindCodec", func(t *testing.T) { testBlindDecode(t, ct) })

	t.Run("Create", func(t *testing.T) { withRevCollections(t, newHarness, testCreate) })
	t.Run("Put", func(t *testing.T) { withRevCollections(t, newHarness, testPut) })
	t.Run("Replace", func(t *testing.T) { withRevCollections(t, newHarness, testReplace) })
	t.Run("Get", func(t *testing.T) { withRevCollections(t, newHarness, testGet) })
	t.Run("Delete", func(t *testing.T) { withRevCollections(t, newHarness, testDelete) })
	t.Run("Update", func(t *testing.T) { withRevCollections(t, newHarness, testUpdate) })
	t.Run("Data", func(t *testing.T) { withCollection(t, newHarness, SingleKey, testData) })
	t.Run("MultipleActions", func(t *testing.T) { withRevCollections(t, newHarness, testMultipleActions) })
	t.Run("GetQueryKeyField", func(t *testing.T) { withRevCollections(t, newHarness, testGetQueryKeyField) })
	t.Run("SerializeRevision", func(t *testing.T) { withCollection(t, newHarness, SingleKey, testSerializeRevision) })
	t.Run("ActionsOnStructNoRev", func(t *testing.T) {
		withCollection(t, newHarness, NoRev, testActionsOnStructNoRev)
	})
	t.Run("ActionsWithCompositeID", func(t *testing.T) { withCollection(t, newHarness, TwoKey, testActionsWithCompositeID) })
	t.Run("GetQuery", func(t *testing.T) { withCollection(t, newHarness, TwoKey, testGetQuery) })

	t.Run("ExampleInDoc", func(t *testing.T) { withCollection(t, newHarness, NoRev, testExampleInDoc) })

	t.Run("BeforeDo", func(t *testing.T) { testBeforeDo(t, newHarness) })
	t.Run("BeforeQuery", func(t *testing.T) { testBeforeQuery(t, newHarness) })

	asTests = append(asTests, verifyAsFailsOnNil{})
	t.Run("As", func(t *testing.T) {
		for _, st := range asTests {
			if st.Name() == "" {
				t.Fatalf("AsTest.Name is required")
			}
			t.Run(st.Name(), func(t *testing.T) {
				withCollection(t, newHarness, TwoKey, func(t *testing.T, _ Harness, coll *docstore.Collection) {
					testAs(t, coll, st)
				})
			})
		}
	})
}

// withCollection calls f with a fresh harness and an empty collection of the given kind.
func withCollection(t *testing.T, newHarness HarnessMaker, kind CollectionKind, f func(*testing.T, Harness, *ds.Collection)) {
	ctx := context.Background()
	h, err := newHarness(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	withColl(t, h, kind, f)
}

// withRevCollections calls f twice: once with the SingleKey collection, using documents and code that expect
// the standard revision field; and once with the AltRev collection, that uses an alternative revisionf field
// name.
func withRevCollections(t *testing.T, newHarness HarnessMaker, f func(*testing.T, *ds.Collection, string)) {
	ctx := context.Background()
	h, err := newHarness(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	t.Run("StdRev", func(t *testing.T) {
		withColl(t, h, SingleKey, func(t *testing.T, _ Harness, coll *ds.Collection) {
			f(t, coll, ds.DefaultRevisionField)
		})
	})
	t.Run("AltRev", func(t *testing.T) {
		withColl(t, h, AltRev, func(t *testing.T, _ Harness, coll *ds.Collection) {
			f(t, coll, AlternateRevisionField)
		})
	})
}

// withColl calls f with h and an empty collection of the given kind. It takes care of closing
// the collection after f returns.
func withColl(t *testing.T, h Harness, kind CollectionKind, f func(*testing.T, Harness, *ds.Collection)) {
	ctx := context.Background()
	dc, err := h.MakeCollection(ctx, kind)
	if err != nil {
		t.Fatal(err)
	}
	coll := ds.NewCollection(dc)
	defer coll.Close()
	clearCollection(t, coll)
	f(t, h, coll)
}

// KeyField is the primary key field for the main test collection.
const KeyField = "name"

// AlternateRevisionField is used for testing the option to provide a different
// name for the revision field.
const AlternateRevisionField = "Etag"

type docmap = map[string]interface{}

func newDoc(doc interface{}) interface{} {
	switch v := doc.(type) {
	case docmap:
		return docmap{KeyField: v[KeyField]}
	case *docstruct:
		return &docstruct{Name: v.Name}
	}
	return nil
}

func key(doc interface{}) interface{} {
	switch d := doc.(type) {
	case docmap:
		return d[KeyField]
	case *docstruct:
		return d.Name
	}
	return nil
}

func setKey(doc, key interface{}) {
	switch d := doc.(type) {
	case docmap:
		d[KeyField] = key
	case *docstruct:
		d.Name = key
	}
}

func revision(doc interface{}, revField string) interface{} {
	switch d := doc.(type) {
	case docmap:
		return d[revField]
	case *docstruct:
		if revField == docstore.DefaultRevisionField {
			return d.DocstoreRevision
		}
		return d.Etag
	}
	return nil
}

func setRevision(doc, rev interface{}, revField string) {
	switch d := doc.(type) {
	case docmap:
		d[revField] = rev
	case *docstruct:
		if revField == docstore.DefaultRevisionField {
			d.DocstoreRevision = rev
		} else {
			d.Etag = rev
		}
	}
}

type docstruct struct {
	Name             interface{} `docstore:"name"`
	DocstoreRevision interface{}
	Etag             interface{}

	I  int
	U  uint
	F  float64
	St string
	B  bool
	M  map[string]interface{}
}

func nonexistentDoc() docmap { return docmap{KeyField: "doesNotExist"} }

func testCreate(t *testing.T, coll *ds.Collection, revField string) {
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		doc     interface{}
		wantErr gcerrors.ErrorCode
	}{
		{
			name: "named map",
			doc:  docmap{KeyField: "testCreateMap", "b": true, revField: nil},
		},
		{
			name:    "existing",
			doc:     docmap{KeyField: "testCreateMap", revField: nil},
			wantErr: gcerrors.AlreadyExists,
		},
		{
			name: "unnamed map",
			doc:  docmap{"b": true, revField: nil},
		},
		{
			name: "named struct",
			doc:  &docstruct{Name: "testCreateStruct", B: true},
		},
		{
			name: "unnamed struct",
			doc:  &docstruct{B: true},
		},
		{
			name: "empty named struct",
			doc:  &docstruct{Name: "", B: true},
		},
		{
			name:    "with non-nil revision",
			doc:     docmap{KeyField: "testCreate2", revField: 0},
			wantErr: gcerrors.InvalidArgument,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantErr == gcerrors.OK {
				checkNoRevisionField(t, tc.doc, revField)
				if err := coll.Create(ctx, tc.doc); err != nil {
					t.Fatalf("Create: %v", err)
				}
				checkHasRevisionField(t, tc.doc, revField)

				got := newDoc(tc.doc)
				if err := coll.Get(ctx, got); err != nil {
					t.Fatalf("Get: %v", err)
				}
				if diff := cmpDiff(got, tc.doc); diff != "" {
					t.Fatal(diff)
				}
			} else {
				err := coll.Create(ctx, tc.doc)
				checkCode(t, err, tc.wantErr)
			}
		})
	}
}

func testPut(t *testing.T, coll *ds.Collection, revField string) {
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	var maprev, strmap interface{}

	for _, tc := range []struct {
		name string
		doc  interface{}
		rev  bool
	}{
		{
			name: "create map",
			doc:  docmap{KeyField: "testPutMap", "b": true, revField: nil},
		},
		{
			name: "create struct",
			doc:  &docstruct{Name: "testPutStruct", B: true},
		},
		{
			name: "replace map",
			doc:  docmap{KeyField: "testPutMap", "b": false, revField: nil},
			rev:  true,
		},
		{
			name: "replace struct",
			doc:  &docstruct{Name: "testPutStruct", B: false},
			rev:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkNoRevisionField(t, tc.doc, revField)
			must(coll.Put(ctx, tc.doc))
			checkHasRevisionField(t, tc.doc, revField)
			got := newDoc(tc.doc)
			must(coll.Get(ctx, got))
			if diff := cmpDiff(got, tc.doc); diff != "" {
				t.Fatalf(diff)
			}
			if tc.rev {
				switch v := tc.doc.(type) {
				case docmap:
					maprev = v[revField]
				case *docstruct:
					if revField == docstore.DefaultRevisionField {
						strmap = v.DocstoreRevision
					} else {
						strmap = v.Etag
					}
				}
			}
		})
	}

	// Putting a doc with a revision field is the same as replace, meaning
	// it will fail if the document doesn't exist.
	for _, tc := range []struct {
		name string
		doc  interface{}
	}{
		{
			name: "replace map wrong key",
			doc:  docmap{KeyField: "testPutMap2", revField: maprev},
		},
		{
			name: "replace struct wrong key",
			doc:  &docstruct{Name: "testPutStruct2", DocstoreRevision: strmap, Etag: strmap},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := coll.Put(ctx, tc.doc)
			if c := gcerrors.Code(err); c != gcerrors.NotFound && c != gcerrors.FailedPrecondition {
				t.Errorf("got %v, want NotFound or FailedPrecondition", err)
			}
		})
	}

	t.Run("revision", func(t *testing.T) {
		testRevisionField(t, coll, revField, func(doc interface{}) error {
			return coll.Put(ctx, doc)
		})
	})

	err := coll.Put(ctx, &docstruct{Name: ""})
	checkCode(t, err, gcerrors.InvalidArgument)
}

func testReplace(t *testing.T, coll *ds.Collection, revField string) {
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range []struct {
		name       string
		doc1, doc2 interface{}
	}{
		{
			name: "replace map",
			doc1: docmap{KeyField: "testReplaceMap", "s": "a", revField: nil},
			doc2: docmap{KeyField: "testReplaceMap", "s": "b", revField: nil},
		},
		{
			name: "replace struct",
			doc1: &docstruct{Name: "testReplaceStruct", St: "a"},
			doc2: &docstruct{Name: "testReplaceStruct", St: "b"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			must(coll.Put(ctx, tc.doc1))
			checkNoRevisionField(t, tc.doc2, revField)
			must(coll.Replace(ctx, tc.doc2))
			checkHasRevisionField(t, tc.doc2, revField)
			got := newDoc(tc.doc2)
			must(coll.Get(ctx, got))
			if diff := cmpDiff(got, tc.doc2); diff != "" {
				t.Fatalf(diff)
			}
		})
	}

	// Can't replace a nonexistent doc.
	checkCode(t, coll.Replace(ctx, nonexistentDoc()), gcerrors.NotFound)

	t.Run("revision", func(t *testing.T) {
		testRevisionField(t, coll, revField, func(doc interface{}) error {
			return coll.Replace(ctx, doc)
		})
	})
}

// Check that doc does not have a revision field (or has a nil one).
func checkNoRevisionField(t *testing.T, doc interface{}, revField string) {
	t.Helper()
	ddoc, err := driver.NewDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if rev, _ := ddoc.GetField(revField); rev != nil {
		t.Fatal("doc has revision field")
	}
}

// Check that doc has a non-nil revision field.
func checkHasRevisionField(t *testing.T, doc interface{}, revField string) {
	t.Helper()
	ddoc, err := driver.NewDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if rev, err := ddoc.GetField(revField); err != nil || rev == nil {
		t.Fatalf("doc missing revision field (error = %v)", err)
	}
}

func testGet(t *testing.T, coll *ds.Collection, revField string) {
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range []struct {
		name string
		doc  interface{}
		fps  []docstore.FieldPath
		want interface{}
	}{
		// If Get is called with no field paths, the full document is populated.
		{
			name: "get map",
			doc: docmap{
				KeyField: "testGetMap",
				"s":      "a string",
				"i":      int64(95),
				"f":      32.3,
				"m":      map[string]interface{}{"a": "one", "b": "two"},
				revField: nil,
			},
		},
		{
			name: "get struct",
			doc: &docstruct{
				Name: "testGetStruct",
				St:   "a string",
				I:    95,
				F:    32.3,
				M:    map[string]interface{}{"a": "one", "b": "two"},
			},
		},
		// If Get is called with field paths, the resulting document has only those fields.
		{
			name: "get map with field path",
			doc: docmap{
				KeyField: "testGetMapFP",
				"s":      "a string",
				"i":      int64(95),
				"f":      32.3,
				"m":      map[string]interface{}{"a": "one", "b": "two"},
				revField: nil,
			},
			fps: []docstore.FieldPath{"f", "m.b", ds.FieldPath(revField)},
			want: docmap{
				KeyField: "testGetMapFP",
				"f":      32.3,
				"m":      map[string]interface{}{"b": "two"},
			},
		},
		{
			name: "get struct with field path",
			doc: &docstruct{
				Name: "testGetStructFP",
				St:   "a string",
				I:    95,
				F:    32.3,
				M:    map[string]interface{}{"a": "one", "b": "two"},
			},
			fps: []docstore.FieldPath{"St", "M.a", ds.FieldPath(revField)},
			want: &docstruct{
				Name: "testGetStructFP",
				St:   "a string",
				M:    map[string]interface{}{"a": "one"},
			},
		},
		{
			name: "get struct wrong case",
			doc: &docstruct{
				Name: "testGetStructWC",
				St:   "a string",
				I:    95,
				F:    32.3,
				M:    map[string]interface{}{"a": "one", "b": "two"},
			},
			fps: []docstore.FieldPath{"st", "m.a"},
			want: &docstruct{
				Name: "testGetStructWC",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			must(coll.Put(ctx, tc.doc))
			got := newDoc(tc.doc)
			must(coll.Get(ctx, got, tc.fps...))
			if tc.want == nil {
				tc.want = tc.doc
			}
			setRevision(tc.want, revision(got, revField), revField)
			if diff := cmpDiff(got, tc.want); diff != "" {
				t.Error("Get with field paths:\n", diff)
			}
		})
	}

	err := coll.Get(ctx, nonexistentDoc())
	checkCode(t, err, gcerrors.NotFound)

	err = coll.Get(ctx, &docstruct{Name: ""})
	checkCode(t, err, gcerrors.InvalidArgument)
}

func testDelete(t *testing.T, coll *ds.Collection, revField string) {
	ctx := context.Background()
	var rev interface{}

	for _, tc := range []struct {
		name    string
		doc     interface{}
		wantErr gcerrors.ErrorCode
	}{
		{
			name: "delete map",
			doc:  docmap{KeyField: "testDeleteMap", revField: nil},
		},
		{
			name:    "delete map wrong rev",
			doc:     docmap{KeyField: "testDeleteMap", "b": true, revField: nil},
			wantErr: gcerrors.FailedPrecondition,
		},
		{
			name: "delete struct",
			doc:  &docstruct{Name: "testDeleteStruct"},
		},
		{
			name:    "delete struct wrong rev",
			doc:     &docstruct{Name: "testDeleteStruct", B: true},
			wantErr: gcerrors.FailedPrecondition,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := coll.Put(ctx, tc.doc); err != nil {
				t.Fatal(err)
			}
			if tc.wantErr == gcerrors.OK {
				rev = revision(tc.doc, revField)
				if err := coll.Delete(ctx, tc.doc); err != nil {
					t.Fatal(err)
				}
				// The document should no longer exist.
				if err := coll.Get(ctx, tc.doc); err == nil {
					t.Error("want error, got nil")
				}
			} else {
				setRevision(tc.doc, rev, revField)
				checkCode(t, coll.Delete(ctx, tc.doc), gcerrors.FailedPrecondition)
			}
		})
	}
	// Delete doesn't fail if the doc doesn't exist.
	if err := coll.Delete(ctx, nonexistentDoc()); err != nil {
		t.Errorf("delete nonexistent doc: want nil, got %v", err)
	}

	err := coll.Delete(ctx, &docstruct{Name: ""})
	checkCode(t, err, gcerrors.InvalidArgument)
}

func testUpdate(t *testing.T, coll *ds.Collection, revField string) {
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		doc  interface{}
		mods ds.Mods
		want interface{}
	}{
		{
			name: "update map",
			doc:  docmap{KeyField: "testUpdateMap", "a": "A", "b": "B", "n": 3.5, "i": 1, revField: nil},
			mods: ds.Mods{
				"a": "X",
				"b": nil,
				"c": "C",
				"n": docstore.Increment(-1),
				"i": nil,
				"m": 3,
			},
			want: docmap{KeyField: "testUpdateMap", "a": "X", "c": "C", "n": 2.5, "m": int64(3)},
		},
		{
			name: "update map overwrite only",
			doc:  docmap{KeyField: "testUpdateMapWrt", "a": "A", revField: nil},
			mods: ds.Mods{
				"a": "X",
				"b": nil,
				"m": 3,
			},
			want: docmap{KeyField: "testUpdateMapWrt", "a": "X", "m": int64(3)},
		},
		{
			name: "update map increment only",
			doc:  docmap{KeyField: "testUpdateMapInc", "a": "A", "n": 3.5, "i": 1, revField: nil},
			mods: ds.Mods{
				"n": docstore.Increment(-1),
				"i": docstore.Increment(2.5),
				"m": docstore.Increment(3),
			},
			want: docmap{KeyField: "testUpdateMapInc", "a": "A", "n": 2.5, "i": 3.5, "m": int64(3)},
		},
		{
			name: "update struct",
			doc:  &docstruct{Name: "testUpdateStruct", St: "st", I: 1, F: 3.5},
			mods: ds.Mods{
				"St": "str",
				"I":  nil,
				"U":  4,
				"F":  docstore.Increment(-3),
			},
			want: &docstruct{Name: "testUpdateStruct", St: "str", U: 4, F: 0.5},
		},
		{
			name: "update struct overwrite only",
			doc:  &docstruct{Name: "testUpdateStructWrt", St: "st", I: 1},
			mods: ds.Mods{
				"St": "str",
				"I":  nil,
				"U":  4,
			},
			want: &docstruct{Name: "testUpdateStructWrt", St: "str", U: 4},
		},
		{
			name: "update struct increment only",
			doc:  &docstruct{Name: "testUpdateStructInc", St: "st", I: 1, F: 3.5},
			mods: ds.Mods{
				"U": docstore.Increment(4),
				"F": docstore.Increment(-3),
			},
			want: &docstruct{Name: "testUpdateStructInc", St: "st", U: 4, I: 1, F: 0.5},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := coll.Put(ctx, tc.doc); err != nil {
				t.Fatal(err)
			}
			setRevision(tc.doc, nil, revField)
			got := newDoc(tc.doc)
			checkNoRevisionField(t, tc.doc, revField)
			errs := coll.Actions().Update(tc.doc, tc.mods).Get(got).Do(ctx)
			if errs != nil {
				t.Fatal(errs)
			}
			checkHasRevisionField(t, tc.doc, revField)
			setRevision(tc.want, revision(got, revField), revField)
			if diff := cmp.Diff(got, tc.want, cmpopts.IgnoreUnexported(timestamp.Timestamp{})); diff != "" {
				t.Error(diff)
			}
		})
	}

	// Can't update a nonexistent doc.
	if err := coll.Update(ctx, nonexistentDoc(), ds.Mods{"x": "y"}); err == nil {
		t.Error("nonexistent document: got nil, want error")
	}

	// Bad increment value.
	err := coll.Update(ctx, docmap{KeyField: "update invalid"}, ds.Mods{"x": ds.Increment("3")})
	checkCode(t, err, gcerrors.InvalidArgument)

	t.Run("revision", func(t *testing.T) {
		testRevisionField(t, coll, revField, func(doc interface{}) error {
			return coll.Update(ctx, doc, ds.Mods{"s": "c"})
		})
	})
}

// Test that:
// - Writing a document with a revision field succeeds if the document hasn't changed.
// - Writing a document with a revision field fails if the document has changed.
func testRevisionField(t *testing.T, coll *ds.Collection, revField string, write func(interface{}) error) {
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name string
		doc  interface{}
	}{
		{
			name: "map revision",
			doc:  docmap{KeyField: "testRevisionMap", "s": "a", revField: nil},
		},
		{
			name: "struct revision",
			doc:  &docstruct{Name: "testRevisionStruct", St: "a"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			must(coll.Put(ctx, tc.doc))
			got := newDoc(tc.doc)
			must(coll.Get(ctx, got))
			rev := revision(got, revField)
			if rev == nil {
				t.Fatal("missing revision field")
			}
			// A write should succeed, because the document hasn't changed since it was gotten.
			if err := write(tc.doc); err != nil {
				t.Fatalf("write with revision field got %v, want nil", err)
			}
			// This write should fail: got's revision field hasn't changed, but the stored document has.
			err := write(got)
			if c := gcerrors.Code(err); c != gcerrors.FailedPrecondition && c != gcerrors.NotFound {
				t.Errorf("write with old revision field: got %v, wanted FailedPrecondition or NotFound", err)
			}
		})
	}
}

// Verify that the driver can serialize and deserialize revisions.
func testSerializeRevision(t *testing.T, h Harness, coll *ds.Collection) {
	ctx := context.Background()
	doc := docmap{KeyField: "testSerializeRevision", "x": 1, docstore.DefaultRevisionField: nil}
	if err := coll.Create(ctx, doc); err != nil {
		t.Fatal(err)
	}
	want := doc[docstore.DefaultRevisionField]
	if want == nil {
		t.Fatal("nil revision")
	}
	s, err := coll.RevisionToString(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := coll.StringToRevision(s)
	if err != nil {
		t.Fatal(err)
	}
	if !h.RevisionsEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// Test all Go integer types are supported, and they all come back as int64.
func testData(t *testing.T, _ Harness, coll *ds.Collection) {
	ctx := context.Background()
	for _, test := range []struct {
		in, want interface{}
	}{
		{int(-1), int64(-1)},
		{int8(-8), int64(-8)},
		{int16(-16), int64(-16)},
		{int32(-32), int64(-32)},
		{int64(-64), int64(-64)},
		{uint(1), int64(1)},
		{uint8(8), int64(8)},
		{uint16(16), int64(16)},
		{uint32(32), int64(32)},
		{uint64(64), int64(64)},
		{float32(3.5), float64(3.5)},
		{[]byte{0, 1, 2}, []byte{0, 1, 2}},
	} {
		doc := docmap{KeyField: "testData", "val": test.in}
		got := docmap{KeyField: doc[KeyField]}
		if errs := coll.Actions().Put(doc).Get(got).Do(ctx); errs != nil {
			t.Fatal(errs)
		}
		want := docmap{
			"val":    test.want,
			KeyField: doc[KeyField],
		}
		if len(got) != len(want) {
			t.Errorf("%v: got %v, want %v", test.in, got, want)
		} else if g := got["val"]; !cmp.Equal(g, test.want) {
			t.Errorf("%v: got %v (%T), want %v (%T)", test.in, g, g, test.want, test.want)
		}
	}

	// TODO: strings: valid vs. invalid unicode

}

var (
	// A time with non-zero milliseconds, but zero nanoseconds.
	milliTime = time.Date(2019, time.March, 27, 0, 0, 0, 5*1e6, time.UTC)
	// A time with non-zero nanoseconds.
	nanoTime = time.Date(2019, time.March, 27, 0, 0, 0, 5*1e6+7, time.UTC)
)

// Test that encoding from a struct and then decoding into the same struct works properly.
// The decoding is "type-driven" because the decoder knows the expected type of the value
// it is decoding--it is the type of a struct field.
func testTypeDrivenDecode(t *testing.T, ct CodecTester) {
	if ct == nil {
		t.Skip("no CodecTester")
	}
	check := func(in, dec interface{}, encode func(interface{}) (interface{}, error), decode func(interface{}, interface{}) error) {
		t.Helper()
		enc, err := encode(in)
		if err != nil {
			t.Fatalf("%+v", err)
		}
		if err := decode(enc, dec); err != nil {
			t.Fatalf("%+v", err)
		}
		if diff := cmp.Diff(in, dec); diff != "" {
			t.Error(diff)
		}
	}

	s := "bar"
	dsrt := &docstoreRoundTrip{
		N:  nil,
		I:  1,
		U:  2,
		F:  2.5,
		St: "foo",
		B:  true,
		L:  []int{3, 4, 5},
		A:  [2]int{6, 7},
		M:  map[string]bool{"a": true, "b": false},
		By: []byte{6, 7, 8},
		P:  &s,
		T:  milliTime,
	}

	check(dsrt, &docstoreRoundTrip{}, ct.DocstoreEncode, ct.DocstoreDecode)

	// Test native-to-docstore and docstore-to-native round trips with a smaller set
	// of types.
	nm := &nativeMinimal{
		N:  nil,
		I:  1,
		F:  2.5,
		St: "foo",
		B:  true,
		L:  []int{3, 4, 5},
		M:  map[string]bool{"a": true, "b": false},
		By: []byte{6, 7, 8},
		P:  &s,
		T:  milliTime,
		LF: []float64{18.8, -19.9, 20},
		LS: []string{"foo", "bar"},
	}
	check(nm, &nativeMinimal{}, ct.DocstoreEncode, ct.NativeDecode)
	check(nm, &nativeMinimal{}, ct.NativeEncode, ct.DocstoreDecode)

	// Test various other types, unless they are unsupported.
	unsupported := map[UnsupportedType]bool{}
	for _, u := range ct.UnsupportedTypes() {
		unsupported[u] = true
	}

	// Unsigned integers.
	if !unsupported[Uint] {
		type Uint struct {
			U uint
		}
		u := &Uint{10}
		check(u, &Uint{}, ct.DocstoreEncode, ct.NativeDecode)
		check(u, &Uint{}, ct.NativeEncode, ct.DocstoreDecode)
	}

	// Arrays.
	if !unsupported[Arrays] {
		type Arrays struct {
			A [2]int
		}
		a := &Arrays{[2]int{13, 14}}
		check(a, &Arrays{}, ct.DocstoreEncode, ct.NativeDecode)
		check(a, &Arrays{}, ct.NativeEncode, ct.DocstoreDecode)
	}
	// Nanosecond-precision time.
	type NT struct {
		T time.Time
	}

	nt := &NT{nanoTime}
	if unsupported[NanosecondTimes] {
		// Expect rounding to the nearest millisecond.
		check := func(encode func(interface{}) (interface{}, error), decode func(interface{}, interface{}) error) {
			enc, err := encode(nt)
			if err != nil {
				t.Fatalf("%+v", err)
			}
			var got NT
			if err := decode(enc, &got); err != nil {
				t.Fatalf("%+v", err)
			}
			want := nt.T.Round(time.Millisecond)
			if !got.T.Equal(want) {
				t.Errorf("got %v, want %v", got.T, want)
			}
		}
		check(ct.DocstoreEncode, ct.NativeDecode)
		check(ct.NativeEncode, ct.DocstoreDecode)
	} else {
		// Expect perfect round-tripping of nanosecond times.
		check(nt, &NT{}, ct.DocstoreEncode, ct.NativeDecode)
		check(nt, &NT{}, ct.NativeEncode, ct.DocstoreDecode)
	}

	// Binary sets.
	if !unsupported[BinarySet] {
		type BinarySet struct {
			B [][]byte
		}
		b := &BinarySet{[][]byte{{15}, {16}, {17}}}
		check(b, &BinarySet{}, ct.DocstoreEncode, ct.NativeDecode)
		check(b, &BinarySet{}, ct.NativeEncode, ct.DocstoreDecode)
	}
}

// Test decoding into an interface{}, where the decoder doesn't know the type of the
// result and must return some Go type that accurately represents the value.
// This is implemented by the AsInterface method of driver.Decoder.
// Since it's fine for different drivers to return different types in this case,
// each test case compares against a list of possible values.
func testBlindDecode(t *testing.T, ct CodecTester) {
	if ct == nil {
		t.Skip("no CodecTester")
	}
	t.Run("DocstoreEncode", func(t *testing.T) { testBlindDecode1(t, ct.DocstoreEncode, ct.DocstoreDecode) })
	t.Run("NativeEncode", func(t *testing.T) { testBlindDecode1(t, ct.NativeEncode, ct.DocstoreDecode) })
}

func testBlindDecode1(t *testing.T, encode func(interface{}) (interface{}, error), decode func(_, _ interface{}) error) {
	// Encode and decode expect a document, so use this struct to hold the values.
	type S struct{ X interface{} }

	for _, test := range []struct {
		in    interface{} // the value to be encoded
		want  interface{} // one possibility
		want2 interface{} // a second possibility
	}{
		{in: nil, want: nil},
		{in: true, want: true},
		{in: "foo", want: "foo"},
		{in: 'c', want: 'c', want2: int64('c')},
		{in: int(3), want: int32(3), want2: int64(3)},
		{in: int8(3), want: int32(3), want2: int64(3)},
		{in: int(-3), want: int32(-3), want2: int64(-3)},
		{in: int64(math.MaxInt32 + 1), want: int64(math.MaxInt32 + 1)},
		{in: float32(1.5), want: float64(1.5)},
		{in: float64(1.5), want: float64(1.5)},
		{in: []byte{1, 2}, want: []byte{1, 2}},
		{in: []int{1, 2},
			want:  []interface{}{int32(1), int32(2)},
			want2: []interface{}{int64(1), int64(2)}},
		{in: []float32{1.5, 2.5}, want: []interface{}{float64(1.5), float64(2.5)}},
		{in: []float64{1.5, 2.5}, want: []interface{}{float64(1.5), float64(2.5)}},
		{in: milliTime, want: milliTime, want2: "2019-03-27T00:00:00.005Z"},
		{in: []time.Time{milliTime},
			want:  []interface{}{milliTime},
			want2: []interface{}{"2019-03-27T00:00:00.005Z"},
		},
		{in: map[string]int{"a": 1},
			want:  map[string]interface{}{"a": int64(1)},
			want2: map[string]interface{}{"a": int32(1)},
		},
		{in: map[string][]byte{"a": {1, 2}}, want: map[string]interface{}{"a": []byte{1, 2}}},
	} {
		enc, err := encode(&S{test.in})
		if err != nil {
			t.Fatalf("encoding %T: %v", test.in, err)
		}
		var got S
		if err := decode(enc, &got); err != nil {
			t.Fatalf("decoding %T: %v", test.in, err)
		}
		matched := false
		wants := []interface{}{test.want}
		if test.want2 != nil {
			wants = append(wants, test.want2)
		}
		for _, w := range wants {
			if cmp.Equal(got.X, w) {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("%T: got %#v (%T), not equal to %#v or %#v", test.in, got.X, got.X, test.want, test.want2)
		}
	}
}

// A round trip with the docstore codec should work for all docstore-supported types,
// regardless of native driver support.
type docstoreRoundTrip struct {
	N  *int
	I  int
	U  uint
	F  float64
	St string
	B  bool
	By []byte
	L  []int
	A  [2]int
	M  map[string]bool
	P  *string
	T  time.Time
}

// TODO(jba): add more fields: structs; embedding.

// All native codecs should support these types. If one doesn't, remove it from this
// struct and make a new single-field struct for it.
type nativeMinimal struct {
	N  *int
	I  int
	F  float64
	St string
	B  bool
	By []byte
	L  []int
	M  map[string]bool
	P  *string
	T  time.Time
	LF []float64
	LS []string
}

// The following is the schema for the collection where the ID is composed from
// multiple fields instead of one. It can be used for query testing.
// It is loosely borrowed from the DynamoDB documentation.
// It is rich enough to require indexes for some drivers.

// A HighScore records one user's high score in a particular game.
// The primary key fields are Game and Player.
type HighScore struct {
	Game             string
	Player           string
	Score            int
	Time             time.Time
	DocstoreRevision interface{}
}

func newHighScore() interface{} { return &HighScore{} }

// HighScoreKey constructs a single primary key from a HighScore struct or a map
// with the same fields by concatenating the Game and Player fields.
func HighScoreKey(doc docstore.Document) interface{} {
	switch d := doc.(type) {
	case *HighScore:
		return d.key()
	case map[string]interface{}:
		return barConcat(d["Game"], d["Player"])
	default:
		panic("bad arg")
	}
}

func (h *HighScore) key() string {
	if h.Game == "" || h.Player == "" {
		return ""
	}
	return barConcat(h.Game, h.Player)
}

func barConcat(a, b interface{}) string { return fmt.Sprintf("%v|%v", a, b) }

func highScoreLess(h1, h2 *HighScore) bool { return h1.key() < h2.key() }

func (h *HighScore) String() string {
	return fmt.Sprintf("%s=%d@%s", h.key(), h.Score, h.Time.Format("01/02"))
}

func date(month, day int) time.Time {
	return time.Date(2019, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

const (
	game1 = "Praise All Monsters"
	game2 = "Zombie DMV"
	game3 = "Days Gone"
)

var highScores = []*HighScore{
	{game1, "pat", 49, date(3, 13), nil},
	{game1, "mel", 60, date(4, 10), nil},
	{game1, "andy", 81, date(2, 1), nil},
	{game1, "fran", 33, date(3, 19), nil},
	{game2, "pat", 120, date(4, 1), nil},
	{game2, "billie", 111, date(4, 10), nil},
	{game2, "mel", 190, date(4, 18), nil},
	{game2, "fran", 33, date(3, 20), nil},
}

func addHighScores(t *testing.T, coll *ds.Collection) {
	alist := coll.Actions()
	for _, doc := range highScores {
		d := *doc
		alist.Put(&d)
	}
	if err := alist.Do(context.Background()); err != nil {
		t.Fatalf("%+v", err)
	}
}

func testGetQueryKeyField(t *testing.T, coll *ds.Collection, revField string) {
	// Query the key field of a collection that has one.
	// (The collection used for testGetQuery uses a key function rather than a key field.)
	ctx := context.Background()
	docs := []docmap{
		{KeyField: "qkf1", "a": "one", revField: nil},
		{KeyField: "qkf2", "a": "two", revField: nil},
		{KeyField: "qkf3", "a": "three", revField: nil},
	}
	al := coll.Actions()
	for _, d := range docs {
		al.Put(d)
	}
	if err := al.Do(ctx); err != nil {
		t.Fatal(err)
	}
	iter := coll.Query().Where(KeyField, "<", "qkf3").Get(ctx)
	defer iter.Stop()
	got := mustCollect(ctx, t, iter)
	want := docs[:2]
	diff := cmpDiff(got, want, cmpopts.SortSlices(sortByKeyField))
	if diff != "" {
		t.Error(diff)
	}

	// Test that queries with selected fields always return the key.
	iter = coll.Query().Get(ctx, "a", ds.FieldPath(revField))
	defer iter.Stop()
	got = mustCollect(ctx, t, iter)
	for _, d := range docs {
		checkHasRevisionField(t, d, revField)
	}
	diff = cmpDiff(got, docs, cmpopts.SortSlices(sortByKeyField))
	if diff != "" {
		t.Error(diff)
	}
}

func sortByKeyField(d1, d2 docmap) bool { return d1[KeyField].(string) < d2[KeyField].(string) }

// TODO(shantuo): consider add this test to all action tests, like the AltRev
// ones.
func testActionsWithCompositeID(t *testing.T, _ Harness, coll *ds.Collection) {
	ctx := context.Background()
	// Create cannot generate an ID for the document when using IDFunc.
	checkCode(t, coll.Create(ctx, &HighScore{}), gcerrors.InvalidArgument)
	checkCode(t, coll.Get(ctx, &HighScore{}), gcerrors.InvalidArgument)

	// Put
	addHighScores(t, coll)
	// Get
	gots := make([]*HighScore, len(highScores))
	actions := coll.Actions()
	for i, doc := range highScores {
		gots[i] = &HighScore{Game: doc.Game, Player: doc.Player}
		actions.Get(gots[i])
	}
	if err := actions.Do(ctx); err != nil {
		t.Fatal(err)
	}
	for i, got := range gots {
		if got.DocstoreRevision == nil {
			t.Errorf("%v missing DocstoreRevision", got)
		} else {
			got.DocstoreRevision = nil
		}
		if diff := cmp.Diff(got, highScores[i]); diff != "" {
			t.Error(diff)
		}
	}
}

func testGetQuery(t *testing.T, _ Harness, coll *ds.Collection) {
	ctx := context.Background()
	addHighScores(t, coll)

	// Query filters should have the same behavior when doing string and number
	// comparison.
	tests := []struct {
		name   string
		q      *ds.Query
		fields []docstore.FieldPath       // fields to get
		want   func(*HighScore) bool      // filters highScores
		before func(x, y *HighScore) bool // if present, checks result order
	}{
		{
			name: "All",
			q:    coll.Query(),
			want: func(*HighScore) bool { return true },
		},
		{
			name: "Game",
			q:    coll.Query().Where("Game", "=", game2),
			want: func(h *HighScore) bool { return h.Game == game2 },
		},
		{
			name: "Score",
			q:    coll.Query().Where("Score", ">", 100),
			want: func(h *HighScore) bool { return h.Score > 100 },
		},
		{
			name: "Player",
			q:    coll.Query().Where("Player", "=", "billie"),
			want: func(h *HighScore) bool { return h.Player == "billie" },
		},
		{
			name: "GamePlayer",
			q:    coll.Query().Where("Player", "=", "andy").Where("Game", "=", game1),
			want: func(h *HighScore) bool { return h.Player == "andy" && h.Game == game1 },
		},
		{
			name: "PlayerScore",
			q:    coll.Query().Where("Player", "=", "pat").Where("Score", "<", 100),
			want: func(h *HighScore) bool { return h.Player == "pat" && h.Score < 100 },
		},
		{
			name: "GameScore",
			q:    coll.Query().Where("Game", "=", game1).Where("Score", ">=", 50),
			want: func(h *HighScore) bool { return h.Game == game1 && h.Score >= 50 },
		},
		{
			name: "PlayerTime",
			q:    coll.Query().Where("Player", "=", "mel").Where("Time", ">", date(4, 1)),
			want: func(h *HighScore) bool { return h.Player == "mel" && h.Time.After(date(4, 1)) },
		},
		{
			name: "ScoreTime",
			q:    coll.Query().Where("Score", ">=", 50).Where("Time", ">", date(4, 1)),
			want: func(h *HighScore) bool { return h.Score >= 50 && h.Time.After(date(4, 1)) },
		},
		{
			name:   "AllByPlayerAsc",
			q:      coll.Query().OrderBy("Player", docstore.Ascending),
			want:   func(h *HighScore) bool { return true },
			before: func(h1, h2 *HighScore) bool { return h1.Player < h2.Player },
		},
		{
			name:   "AllByPlayerDesc",
			q:      coll.Query().OrderBy("Player", docstore.Descending),
			want:   func(h *HighScore) bool { return true },
			before: func(h1, h2 *HighScore) bool { return h1.Player > h2.Player },
		},
		{
			name: "GameByPlayerAsc",
			// We need a filter on Player, and it can't be the empty string (DynamoDB limitation).
			// So pick any string that sorts less than all valid player names.
			q: coll.Query().Where("Game", "=", game1).Where("Player", ">", ".").
				OrderBy("Player", docstore.Ascending),
			want:   func(h *HighScore) bool { return h.Game == game1 },
			before: func(h1, h2 *HighScore) bool { return h1.Player < h2.Player },
		},
		{
			// Same as above, but descending.
			name: "GameByPlayerDesc",
			q: coll.Query().Where("Game", "=", game1).Where("Player", ">", ".").
				OrderBy("Player", docstore.Descending),
			want:   func(h *HighScore) bool { return h.Game == game1 },
			before: func(h1, h2 *HighScore) bool { return h1.Player > h2.Player },
		},
		// TODO(jba): add more OrderBy tests.
		{
			name:   "AllWithKeyFields",
			q:      coll.Query(),
			fields: []docstore.FieldPath{"Game", "Player", ds.FieldPath(ds.DefaultRevisionField)},
			want: func(h *HighScore) bool {
				h.Score = 0
				h.Time = time.Time{}
				return true
			},
		},
		{
			name:   "AllWithScore",
			q:      coll.Query(),
			fields: []docstore.FieldPath{"Game", "Player", "Score", ds.FieldPath(ds.DefaultRevisionField)},
			want: func(h *HighScore) bool {
				h.Time = time.Time{}
				return true
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := collectHighScores(ctx, tc.q.Get(ctx, tc.fields...))
			if err != nil {
				t.Fatal(err)
			}
			for _, g := range got {
				if g.DocstoreRevision == nil {
					t.Errorf("%v missing DocstoreRevision", g)
				} else {
					g.DocstoreRevision = nil
				}
			}
			want := filterHighScores(highScores, tc.want)
			_, err = tc.q.Plan()
			if err != nil {
				t.Fatal(err)
			}
			diff := cmp.Diff(got, want, cmpopts.SortSlices(highScoreLess))
			if diff != "" {
				t.Fatal(diff)
			}
			if tc.before != nil {
				// Verify that the results are sorted according to tc.less.
				for i := 1; i < len(got); i++ {
					if tc.before(got[i], got[i-1]) {
						t.Errorf("%s at %d sorts before previous %s", got[i], i, got[i-1])
					}
				}
			}
			// We can't assume anything about the query plan. Just verify that Plan returns
			// successfully.
			if _, err := tc.q.Plan(KeyField); err != nil {
				t.Fatal(err)
			}
		})
	}
	t.Run("Limit", func(t *testing.T) {
		// For limit, we can't be sure which documents will be returned, only their count.
		limitQ := coll.Query().Limit(2)
		got := mustCollectHighScores(ctx, t, limitQ.Get(ctx))
		if len(got) != 2 {
			t.Errorf("got %v, wanted two documents", got)
		}
	})
}

func filterHighScores(hs []*HighScore, f func(*HighScore) bool) []*HighScore {
	var res []*HighScore
	for _, h := range hs {
		c := *h // Copy in case f modifies its argument.
		if f(&c) {
			res = append(res, &c)
		}
	}
	return res
}

// clearCollection delete all documents from this collection after test.
func clearCollection(fataler interface{ Fatalf(string, ...interface{}) }, coll *docstore.Collection) {
	ctx := context.Background()
	iter := coll.Query().Get(ctx)
	dels := coll.Actions()
	for {
		doc := map[string]interface{}{}
		err := iter.Next(ctx, doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			fataler.Fatalf("%+v", err)
		}
		dels.Delete(doc)
	}
	if err := dels.Do(ctx); err != nil {
		fataler.Fatalf("%+v", err)
	}
}

func forEach(ctx context.Context, iter *ds.DocumentIterator, create func() interface{}, handle func(interface{}) error) error {
	for {
		doc := create()
		err := iter.Next(ctx, doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if err := handle(doc); err != nil {
			return err
		}
	}
	return nil
}

func mustCollect(ctx context.Context, t *testing.T, iter *ds.DocumentIterator) []docmap {
	var ms []docmap
	newDocmap := func() interface{} { return docmap{} }
	collect := func(m interface{}) error { ms = append(ms, m.(docmap)); return nil }
	if err := forEach(ctx, iter, newDocmap, collect); err != nil {
		t.Fatal(err)
	}
	return ms
}

func mustCollectHighScores(ctx context.Context, t *testing.T, iter *ds.DocumentIterator) []*HighScore {
	hs, err := collectHighScores(ctx, iter)
	if err != nil {
		t.Fatal(err)
	}
	return hs
}

func collectHighScores(ctx context.Context, iter *ds.DocumentIterator) ([]*HighScore, error) {
	var hs []*HighScore
	collect := func(h interface{}) error { hs = append(hs, h.(*HighScore)); return nil }
	if err := forEach(ctx, iter, newHighScore, collect); err != nil {
		return nil, err
	}
	return hs, nil
}

func testMultipleActions(t *testing.T, coll *ds.Collection, revField string) {
	ctx := context.Background()

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	var docs []docmap
	for i := 0; i < 9; i++ {
		docs = append(docs, docmap{
			KeyField: fmt.Sprintf("testUnorderedActions%d", i),
			"s":      fmt.Sprint(i),
			revField: nil,
		})
	}

	compare := func(gots, wants []docmap) {
		t.Helper()
		for i := 0; i < len(gots); i++ {
			got := gots[i]
			want := clone(wants[i])
			want[revField] = got[revField]
			if !cmp.Equal(got, want, cmpopts.IgnoreUnexported(timestamp.Timestamp{})) {
				t.Errorf("index #%d:\ngot  %v\nwant %v", i, got, want)
			}
		}
	}

	// Put the first three docs.
	actions := coll.Actions()
	for i := 0; i < 6; i++ {
		actions.Create(docs[i])
	}
	must(actions.Do(ctx))

	// Replace the first three and put six more.
	actions = coll.Actions()
	for i := 0; i < 3; i++ {
		docs[i]["s"] = fmt.Sprintf("%d'", i)
		actions.Replace(docs[i])
	}
	for i := 3; i < 9; i++ {
		actions.Put(docs[i])
	}
	must(actions.Do(ctx))

	// Delete the first three, get the second three, and put three more.
	gdocs := []docmap{
		{KeyField: docs[3][KeyField]},
		{KeyField: docs[4][KeyField]},
		{KeyField: docs[5][KeyField]},
	}
	actions = coll.Actions()
	actions.Update(docs[6], ds.Mods{"s": "6'", "n": ds.Increment(1)})
	actions.Get(gdocs[0])
	actions.Delete(docs[0])
	actions.Delete(docs[1])
	actions.Update(docs[7], ds.Mods{"s": "7'"})
	actions.Get(gdocs[1])
	actions.Delete(docs[2])
	actions.Get(gdocs[2])
	actions.Update(docs[8], ds.Mods{"n": ds.Increment(-1)})
	must(actions.Do(ctx))
	compare(gdocs, docs[3:6])

	// At this point, the existing documents are 3 - 9.

	// Get the first four, try to create one that already exists, delete a
	// nonexistent doc, and put one. Only the Get of #3, the Delete and the Put
	// should succeed.
	actions = coll.Actions()
	for _, doc := range []docmap{
		{KeyField: docs[0][KeyField]},
		{KeyField: docs[1][KeyField]},
		{KeyField: docs[2][KeyField]},
		{KeyField: docs[3][KeyField]},
	} {
		actions.Get(doc)
	}
	docs[4][revField] = nil
	actions.Create(docs[4]) // create existing doc
	actions.Put(docs[5])
	// TODO(jba): Understand why the following line is necessary for dynamo but not the others.
	docs[0][revField] = nil
	actions.Delete(docs[0]) // delete nonexistent doc
	err := actions.Do(ctx)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	alerr, ok := err.(docstore.ActionListError)
	if !ok {
		t.Fatalf("got %v (%T), want ActionListError", alerr, alerr)
	}
	for _, e := range alerr {
		switch i := e.Index; i {
		case 3, 5, 6:
			t.Errorf("index %d: got %v, want nil", i, e.Err)

		case 4, -1: // -1 for mongodb issue, see https://jira.mongodb.org/browse/GODRIVER-1028
			if ec := gcerrors.Code(e.Err); ec != gcerrors.AlreadyExists &&
				ec != gcerrors.FailedPrecondition { // TODO(shantuo): distinguish this case for dyanmo
				t.Errorf("index 4: create an existing document: got %v, want error", e.Err)
			}

		default:
			if gcerrors.Code(e.Err) != gcerrors.NotFound {
				t.Errorf("index %d: got %v, want NotFound", i, e.Err)
			}
		}
	}
}

func testActionsOnStructNoRev(t *testing.T, _ Harness, coll *ds.Collection) {
	type item struct {
		Name string `docstore:"name"`
		I    int
	}
	doc1 := item{Name: "createandreplace"}
	doc2 := item{Name: "putandupdate"}
	ctx := context.Background()

	got1 := item{Name: doc1.Name}
	got2 := map[string]interface{}{"name": doc2.Name}
	if err := coll.Actions().
		Create(&doc1).Put(&doc2).
		Get(&got1).Get(got2).
		Do(ctx); err != nil {
		t.Fatal(err)
	}
	checkNoRevisionField(t, got2, ds.DefaultRevisionField)

	got3 := map[string]interface{}{"name": doc1.Name}
	got4 := item{Name: doc2.Name}
	if err := coll.Actions().
		Replace(&doc1).Update(&item{Name: doc2.Name}, ds.Mods{"I": 1}).
		Get(got3, "I").Get(&got4, "I").
		Do(ctx); err != nil {
		t.Fatal(err)
	}
	checkNoRevisionField(t, got3, ds.DefaultRevisionField)
}

func testExampleInDoc(t *testing.T, _ Harness, coll *ds.Collection) {
	type Name struct {
		First, Last string
	}
	type Book struct {
		Title            string `docstore:"name"`
		Author           Name   `docstore:"author"`
		PublicationYears []int  `docstore:"pub_years,omitempty"`
		NumPublications  int    `docstore:"-"`
	}

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	checkFieldEqual := func(got, want interface{}, field string) {
		t.Helper()
		fvg, err := MustDocument(got).GetField(field)
		must(err)
		fvw, err := MustDocument(want).GetField(field)
		must(err)
		if !cmp.Equal(fvg, fvw) {
			t.Errorf("%s: got %v want %v", field, fvg, fvw)
		}
	}

	doc1 := &Book{
		Title: "The Master and Margarita",
		Author: Name{
			First: "Mikhail",
			Last:  "Bulgakov",
		},
		PublicationYears: []int{1967, 1973},
		NumPublications:  2,
	}

	doc2 := map[string]interface{}{
		KeyField: "The Heart of a Dog",
		"author": map[string]interface{}{
			"First": "Mikhail",
			"Last":  "Bulgakov",
		},
		"pub_years": []int{1968, 1987},
	}

	ctx := context.Background()
	must(coll.Actions().Create(doc1).Put(doc2).Do(ctx))
	got1 := &Book{Title: doc1.Title}
	got2 := &Book{Title: doc2[KeyField].(string)}
	must(coll.Actions().Get(got1).Get(got2).Do(ctx))

	if got1.NumPublications != 0 {
		t.Errorf("docstore:\"-\" tagged field isn't ignored")
	}
	checkFieldEqual(got1, doc1, "author")
	checkFieldEqual(got2, doc2, "pub_years")

	gots := mustCollect(ctx, t, coll.Query().Where("author.Last", "=", "Bulgakov").Get(ctx))
	if len(gots) != 2 {
		t.Errorf("got %v want all two results", gots)
	}
	must(coll.Actions().Delete(doc1).Delete(doc2).Do(ctx))
}

// Verify that BeforeDo is invoked, and its as function behaves as expected.
func testBeforeDo(t *testing.T, newHarness HarnessMaker) {
	ctx := context.Background()
	withCollection(t, newHarness, SingleKey, func(t *testing.T, h Harness, coll *ds.Collection) {
		var called bool
		beforeDo := func(asFunc func(interface{}) bool) error {
			called = true
			if asFunc(nil) {
				return errors.New("asFunc returned true when called with nil, want false")
			}
			// At least one of the expected types must return true. Special case: if
			// there are no types, then the as function never returns true, so skip the
			// check.
			if len(h.BeforeDoTypes()) > 0 {
				found := false
				for _, b := range h.BeforeDoTypes() {
					v := reflect.New(reflect.TypeOf(b)).Interface()
					if asFunc(v) {
						found = true
						break
					}
				}
				if !found {
					return errors.New("none of the BeforeDoTypes works with the as function")
				}
			}
			return nil
		}

		check := func(f func(*ds.ActionList)) {
			t.Helper()
			// First, verify that if a BeforeDo function returns an error, so does ActionList.Do.
			// We depend on that for the rest of the test.
			al := coll.Actions().BeforeDo(func(func(interface{}) bool) error { return errors.New("") })
			f(al)
			if err := al.Do(ctx); err == nil {
				t.Error("beforeDo returning error: got nil from Do, want error")
				return
			}
			called = false
			al = coll.Actions().BeforeDo(beforeDo)
			f(al)
			if err := al.Do(ctx); err != nil {
				t.Error(err)
				return
			}
			if !called {
				t.Error("BeforeDo function never called")
			}
		}

		doc := docmap{KeyField: "testBeforeDo"}
		check(func(l *docstore.ActionList) { l.Create(doc) })
		check(func(l *docstore.ActionList) { l.Replace(doc) })
		check(func(l *docstore.ActionList) { l.Put(doc) })
		check(func(l *docstore.ActionList) { l.Update(doc, docstore.Mods{"a": 1}) })
		check(func(l *docstore.ActionList) { l.Get(doc) })
		check(func(l *docstore.ActionList) { l.Delete(doc) })
	})
}

// Verify that BeforeQuery is invoked, and its as function behaves as expected.
func testBeforeQuery(t *testing.T, newHarness HarnessMaker) {
	ctx := context.Background()
	withCollection(t, newHarness, SingleKey, func(t *testing.T, h Harness, coll *ds.Collection) {
		var called bool
		beforeQuery := func(asFunc func(interface{}) bool) error {
			called = true
			if asFunc(nil) {
				return errors.New("asFunc returned true when called with nil, want false")
			}
			// At least one of the expected types must return true. Special case: if
			// there are no types, then the as function never returns true, so skip the
			// check.
			if len(h.BeforeQueryTypes()) > 0 {
				found := false
				for _, b := range h.BeforeQueryTypes() {
					v := reflect.New(reflect.TypeOf(b)).Interface()
					if asFunc(v) {
						found = true
						break
					}
				}
				if !found {
					return errors.New("none of the BeforeQueryTypes works with the as function")
				}
			}
			return nil
		}

		iter := coll.Query().BeforeQuery(beforeQuery).Get(ctx)
		if err := iter.Next(ctx, docmap{}); err != io.EOF {
			t.Fatalf("got %v, wanted io.EOF", err)
		}
		if !called {
			t.Error("BeforeQuery function never called for Get")
		}
	})
}

func testAs(t *testing.T, coll *ds.Collection, st AsTest) {
	// Verify Collection.As
	if err := st.CollectionCheck(coll); err != nil {
		t.Error(err)
	}

	ctx := context.Background()

	// Query
	qs := []*docstore.Query{
		coll.Query().Where("Game", "=", game3),
		// Note: don't use filter on Player, the test table has Player as the
		// partition key of a Global Secondary Index, which doesn't support
		// ConsistentRead mode, which is what the As test does in its BeforeQuery
		// function.
		coll.Query().Where("Score", ">", 50),
	}
	for _, q := range qs {
		iter := q.Get(ctx)
		if err := st.QueryCheck(iter); err != nil {
			t.Error(err)
		}
	}

	// ErrorCheck
	doc := &HighScore{game3, "steph", 24, date(4, 25), nil}
	if err := coll.Create(ctx, doc); err != nil {
		t.Fatal(err)
	}
	doc.DocstoreRevision = nil
	if err := coll.Create(ctx, doc); err == nil {
		t.Fatal("got nil error from creating an existing item, want an error")
	} else {
		if alerr, ok := err.(docstore.ActionListError); ok {
			for _, aerr := range alerr {
				if checkerr := st.ErrorCheck(coll, aerr.Err); checkerr != nil {
					t.Error(checkerr)
				}
			}
		} else if checkerr := st.ErrorCheck(coll, err); checkerr != nil {
			t.Error(checkerr)
		}
	}
}

func clone(m docmap) docmap {
	r := docmap{}
	for k, v := range m {
		r[k] = v
	}
	return r
}

func cmpDiff(a, b interface{}, opts ...cmp.Option) string {
	// Firestore revisions can be protos.
	return cmp.Diff(a, b, append([]cmp.Option{cmp.Comparer(proto.Equal)}, opts...)...)
}

func checkCode(t *testing.T, err error, code gcerrors.ErrorCode) {
	t.Helper()
	if gcerrors.Code(err) != code {
		t.Errorf("got %v, want %s", err, code)
	}
}
