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
package drivertest // import "gocloud.dev/internal/docstore/drivertest"

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/docstore"
	ds "gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/driver"
)

// TODO(jba): Test RunActions with unordered=true. We can't actually test the ordering,
// but we can check that all actions are executed even if some fail.

// TODO(jba): test an ordered list of actions, with an expected error in the middle,
// and check that we get the right error back and that the actions after the error
// aren't executed.

// TODO(jba): test that a malformed action is returned as an error and none of the
// actions are executed.

// Harness descibes the functionality test harnesses must provide to run
// conformance tests.
type Harness interface {
	// MakeCollection makes a driver.Collection for testing.
	// The collection should have a single primary key field of type string named
	// drivertest.KeyField.
	MakeCollection(context.Context) (driver.Collection, error)

	// MakeTwoKeyCollection makes a driver.Collection for testing.
	// The collection will consist entirely of HighScore structs (see below), whose
	// two primary key fields are "Game" and "Player", both strings. Use
	// drivertest.HighScoreKey as the key function.
	MakeTwoKeyCollection(ctx context.Context) (driver.Collection, error)

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
	// Native codec doesn't support any complex type
	Complex
	// Native codec doesn't support arrays
	Arrays
	// Native codec doesn't support full time precision
	NanosecondTimes
	// Native codec doesn't support [][]byte
	BinarySet
)

// CodecTester describes functions that encode and decode values using both the
// docstore codec for a provider, and that provider's own "native" codec.
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
	// BeforeDo will be passed directly to ActionList.BeforeDo as part of running
	// the test actions.
	BeforeDo(as func(interface{}) bool) error
	// BeforeQuery will be passed directly to Query.BeforeQuery as part of doing
	// the test query.
	BeforeQuery(as func(interface{}) bool) error
	// QueryCheck will be called after calling Query. It should call it.As and
	// verify the results.
	QueryCheck(it *docstore.DocumentIterator) error
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

func (verifyAsFailsOnNil) BeforeDo(as func(interface{}) bool) error {
	if as(nil) {
		return errors.New("want ActionList.As to return false when passed nil")
	}
	return nil
}

func (verifyAsFailsOnNil) BeforeQuery(as func(interface{}) bool) error {
	if as(nil) {
		return errors.New("want Query.As to return false when passed nil")
	}
	return nil
}

func (verifyAsFailsOnNil) QueryCheck(it *docstore.DocumentIterator) error {
	if it.As(nil) {
		return errors.New("want DocumentIterator.As to return false when passed nil")
	}
	return nil
}

// RunConformanceTests runs conformance tests for provider implementations of docstore.
func RunConformanceTests(t *testing.T, newHarness HarnessMaker, ct CodecTester, asTests []AsTest) {
	t.Run("TypeDrivenCodec", func(t *testing.T) { testTypeDrivenDecode(t, ct) })
	t.Run("BlindCodec", func(t *testing.T) { testBlindDecode(t, ct) })

	t.Run("Create", func(t *testing.T) { withCollection(t, newHarness, testCreate) })
	t.Run("Put", func(t *testing.T) { withCollection(t, newHarness, testPut) })
	t.Run("Replace", func(t *testing.T) { withCollection(t, newHarness, testReplace) })
	t.Run("Get", func(t *testing.T) { withCollection(t, newHarness, testGet) })
	t.Run("Delete", func(t *testing.T) { withCollection(t, newHarness, testDelete) })
	t.Run("Update", func(t *testing.T) { withCollection(t, newHarness, testUpdate) })
	t.Run("Data", func(t *testing.T) { withCollection(t, newHarness, testData) })
	t.Run("MultipleActions", func(t *testing.T) { withCollection(t, newHarness, testMultipleActions) })
	t.Run("UnorderedActions", func(t *testing.T) { withCollection(t, newHarness, testUnorderedActions) })
	t.Run("GetQueryKeyField", func(t *testing.T) { withCollection(t, newHarness, testGetQueryKeyField) })

	t.Run("GetQuery", func(t *testing.T) { withTwoKeyCollection(t, newHarness, testGetQuery) })
	t.Run("DeleteQuery", func(t *testing.T) { withTwoKeyCollection(t, newHarness, testDeleteQuery) })
	t.Run("UpdateQuery", func(t *testing.T) { withTwoKeyCollection(t, newHarness, testUpdateQuery) })

	asTests = append(asTests, verifyAsFailsOnNil{})
	t.Run("As", func(t *testing.T) {
		for _, st := range asTests {
			if st.Name() == "" {
				t.Fatalf("AsTest.Name is required")
			}
			t.Run(st.Name(), func(t *testing.T) {
				withTwoKeyCollection(t, newHarness, func(t *testing.T, coll *docstore.Collection) {
					testAs(t, coll, st)
				})
			})
		}
	})
}

func withCollection(t *testing.T, newHarness HarnessMaker, f func(*testing.T, *ds.Collection)) {
	ctx := context.Background()
	h, err := newHarness(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	dc, err := h.MakeCollection(ctx)
	if err != nil {
		t.Fatal(err)
	}
	coll := ds.NewCollection(dc)
	cleanUpTable(t, coll)
	f(t, coll)
}

func withTwoKeyCollection(t *testing.T, newHarness HarnessMaker, f func(*testing.T, *ds.Collection)) {
	ctx := context.Background()
	h, err := newHarness(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	dc, err := h.MakeTwoKeyCollection(ctx)
	if err != nil {
		t.Fatal(err)
	}
	coll := ds.NewCollection(dc)
	cleanUpTable(t, coll)
	f(t, coll)
}

// KeyField is the primary key field for the main test collection.
const KeyField = "name"

type docmap = map[string]interface{}

var nonexistentDoc = docmap{KeyField: "doesNotExist"}

func testCreate(t *testing.T, coll *ds.Collection) {
	ctx := context.Background()
	named := docmap{KeyField: "testCreate1", "b": true}
	// TODO(#1857): dynamodb requires the sort key field when it is defined. We
	// don't generate random sort key so we need to skip the unnamed test and
	// figure out what to do in this situation.
	// unnamed := docmap{"b": false}
	// Attempt to clean up
	defer func() {
		_ = coll.Actions().Delete(named).
			// Delete(unnamed).
			Do(ctx)
	}()

	createThenGet := func(doc docmap) {
		t.Helper()
		if err := coll.Create(ctx, doc); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got := docmap{KeyField: doc[KeyField]}
		if err := coll.Get(ctx, got); err != nil {
			t.Fatalf("Get: %v", err)
		}
		// got has a revision field that doc doesn't have.
		doc[ds.RevisionField] = got[ds.RevisionField]
		if diff := cmp.Diff(got, doc); diff != "" {
			t.Fatal(diff)
		}
	}

	createThenGet(named)
	// createThenGet(unnamed)

	// Can't create an existing doc.
	if err := coll.Create(ctx, named); err == nil {
		t.Error("got nil, want error")
	}
}

func testPut(t *testing.T, coll *ds.Collection) {
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	named := docmap{KeyField: "testPut1", "b": true}
	// Create a new doc.
	must(coll.Put(ctx, named))
	got := docmap{KeyField: named[KeyField]}
	must(coll.Get(ctx, got))
	named[ds.RevisionField] = got[ds.RevisionField] // copy returned revision field
	if diff := cmp.Diff(got, named); diff != "" {
		t.Fatalf(diff)
	}

	// Replace an existing doc.
	named["b"] = false
	must(coll.Put(ctx, named))
	must(coll.Get(ctx, got))
	named[ds.RevisionField] = got[ds.RevisionField]
	if diff := cmp.Diff(got, named); diff != "" {
		t.Fatalf(diff)
	}

	t.Run("revision", func(t *testing.T) {
		testRevisionField(t, coll, func(dm docmap) error {
			return coll.Put(ctx, dm)
		})
	})
}

func testReplace(t *testing.T, coll *ds.Collection) {
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	doc1 := docmap{KeyField: "testReplace", "s": "a"}
	must(coll.Put(ctx, doc1))
	doc1["s"] = "b"
	must(coll.Replace(ctx, doc1))
	got := docmap{KeyField: doc1[KeyField]}
	must(coll.Get(ctx, got))
	doc1[ds.RevisionField] = got[ds.RevisionField] // copy returned revision field
	if diff := cmp.Diff(got, doc1); diff != "" {
		t.Fatalf(diff)
	}
	// Can't replace a nonexistent doc.
	if err := coll.Replace(ctx, nonexistentDoc); err == nil {
		t.Fatal("got nil, want error")
	}

	t.Run("revision", func(t *testing.T) {
		testRevisionField(t, coll, func(dm docmap) error {
			return coll.Replace(ctx, dm)
		})
	})

}

func testGet(t *testing.T, coll *ds.Collection) {
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	doc := docmap{
		KeyField: "testGet1",
		"s":      "a string",
		"i":      int64(95),
		"f":      32.3,
		"m":      map[string]interface{}{"a": "one", "b": "two"},
	}
	must(coll.Put(ctx, doc))
	// If Get is called with no field paths, the full document is populated.
	got := docmap{KeyField: doc[KeyField]}
	must(coll.Get(ctx, got))
	doc[ds.RevisionField] = got[ds.RevisionField] // copy returned revision field
	if diff := cmp.Diff(got, doc); diff != "" {
		t.Error(diff)
	}

	// If Get is called with field paths, the resulting document has only those fields.
	got = docmap{KeyField: doc[KeyField]}
	must(coll.Get(ctx, got, "f", "m.b"))
	want := docmap{
		KeyField:         doc[KeyField],
		ds.RevisionField: got[ds.RevisionField], // copy returned revision field
		"f":              32.3,
		"m":              docmap{"b": "two"},
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Error("Get with field paths:\n", diff)
	}

	err := coll.Get(ctx, nonexistentDoc)
	if gcerrors.Code(err) != gcerrors.NotFound {
		t.Errorf("Get of nonexistent doc: got %v, want NotFound", err)
	}
}

func testDelete(t *testing.T, coll *ds.Collection) {
	ctx := context.Background()
	doc := docmap{KeyField: "testDelete"}
	if err := coll.Put(ctx, doc); err != nil {
		t.Fatal(err)
	}
	if errs := coll.Actions().Delete(doc).Do(ctx); errs != nil {
		t.Fatal(errs)
	}
	// The document should no longer exist.
	if err := coll.Get(ctx, doc); err == nil {
		t.Error("want error, got nil")
	}
	// Delete doesn't fail if the doc doesn't exist.
	if err := coll.Delete(ctx, nonexistentDoc); err != nil {
		t.Errorf("delete nonexistent doc: want nil, got %v", err)
	}

	// Delete will fail if the revision field is mismatched.
	got := docmap{KeyField: doc[KeyField]}
	if errs := coll.Actions().Put(doc).Get(got).Do(ctx); errs != nil {
		t.Fatal(errs)
	}
	doc["x"] = "y"
	if err := coll.Put(ctx, doc); err != nil {
		t.Fatal(err)
	}
	if err := coll.Delete(ctx, got); gcerrors.Code(err) != gcerrors.FailedPrecondition {
		t.Errorf("got %v, want FailedPrecondition", err)
	}
}

func testUpdate(t *testing.T, coll *ds.Collection) {
	ctx := context.Background()
	doc := docmap{KeyField: "testUpdate", "a": "A", "b": "B", "n": 3.5, "i": 1}
	if err := coll.Put(ctx, doc); err != nil {
		t.Fatal(err)
	}
	got := docmap{KeyField: doc[KeyField]}
	errs := coll.Actions().Update(doc, ds.Mods{
		"a": "X",
		"b": nil,
		"c": "C",
		"n": docstore.Increment(-1),
		"i": docstore.Increment(2.5), // incrementing an integer with a float works
		"m": docstore.Increment(3),   // increment of a nonexistent field is like set
	}).Get(got).Do(ctx)
	if errs != nil {
		t.Fatal(errs)
	}
	want := docmap{
		KeyField:         doc[KeyField],
		ds.RevisionField: got[ds.RevisionField],
		"a":              "X",
		"c":              "C",
		"n":              2.5,
		"i":              3.5,
		"m":              int64(3),
	}
	if !cmp.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	// TODO(jba): test that empty mods is a no-op.

	// Can't update a nonexistent doc.
	if err := coll.Update(ctx, nonexistentDoc, ds.Mods{"x": "y"}); err == nil {
		t.Error("nonexistent document: got nil, want error")
	}

	// Bad increment value.
	err := coll.Update(ctx, doc, ds.Mods{"x": ds.Increment("3")})
	if gcerrors.Code(err) != gcerrors.InvalidArgument {
		t.Errorf("bad increment: got %v, want InvalidArgument", err)
	}

	t.Run("revision", func(t *testing.T) {
		testRevisionField(t, coll, func(dm docmap) error {
			return coll.Update(ctx, dm, ds.Mods{"s": "c"})
		})
	})
}

func testRevisionField(t *testing.T, coll *ds.Collection, write func(docmap) error) {
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	doc1 := docmap{KeyField: "testRevisionField", "s": "a"}
	must(coll.Put(ctx, doc1))
	got := docmap{KeyField: doc1[KeyField]}
	must(coll.Get(ctx, got))
	rev, ok := got[ds.RevisionField]
	if !ok || rev == nil {
		t.Fatal("missing revision field")
	}
	got["s"] = "b"
	// A write should succeed, because the document hasn't changed since it was gotten.
	if err := write(got); err != nil {
		t.Fatalf("write with revision field got %v, want nil", err)
	}
	// This write should fail: got's revision field hasn't changed, but the stored document has.
	err := write(got)
	if c := gcerrors.Code(err); c != gcerrors.FailedPrecondition && c != gcerrors.NotFound {
		t.Errorf("write with old revision field: got %v, wanted FailedPrecondition or NotFound", err)
	}
}

func testData(t *testing.T, coll *ds.Collection) {
	// All Go integer types are supported, but they all come back as int64.
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
			"val":            test.want,
			KeyField:         doc[KeyField],
			ds.RevisionField: got[ds.RevisionField],
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
		C:  complex(3.0, 4.0),
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

	// Complex numbers.
	if !unsupported[Complex] {
		type Complex struct {
			C complex128
		}
		c := &Complex{complex(11, 12)}
		check(c, &Complex{}, ct.DocstoreEncode, ct.NativeDecode)
		check(c, &Complex{}, ct.NativeEncode, ct.DocstoreDecode)
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
// Since it's fine for different providers to return different types in this case,
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
	C  complex128
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

// The following is the schema for the collection used for query testing.
// It is loosely borrowed from the DynamoDB documentation.
// It is rich enough to require indexes for some providers.

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

// HighScoreKey constructs a single primary key from a HighScore struct
// by concatenating the Game and Player fields.
func HighScoreKey(doc docstore.Document) interface{} {
	h := doc.(*HighScore)
	return h.Game + "|" + h.Player
}

func (h *HighScore) String() string {
	return fmt.Sprintf("%s|%s=%d@%s", h.Game, h.Player, h.Score, h.Time.Format("01/02"))
}

func date(month, day int) time.Time {
	return time.Date(2019, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

const (
	game1 = "Praise All Monsters"
	game2 = "Zombie DMV"
	game3 = "Days Gone"
)

var queryDocuments = []*HighScore{
	{game1, "pat", 49, date(3, 13), nil},
	{game1, "mel", 60, date(4, 10), nil},
	{game1, "andy", 81, date(2, 1), nil},
	{game1, "fran", 33, date(3, 19), nil},
	{game2, "pat", 120, date(4, 1), nil},
	{game2, "billie", 111, date(4, 10), nil},
	{game2, "mel", 190, date(4, 18), nil},
	{game2, "fran", 33, date(3, 20), nil},
}

func addQueryDocuments(t *testing.T, coll *ds.Collection) {
	alist := coll.Actions().Unordered()
	for _, doc := range queryDocuments {
		alist.Put(doc)
	}
	if err := alist.Do(context.Background()); err != nil {
		t.Fatalf("%+v", err)
	}
}

func testGetQueryKeyField(t *testing.T, coll *ds.Collection) {
	// Query the key field of a collection that has one.
	// (The collection used for testGetQuery uses a key function rather than a key field.)
	ctx := context.Background()
	docs := []docmap{
		{KeyField: "qkf1"},
		{KeyField: "qkf2"},
		{KeyField: "qkf3"},
	}
	al := coll.Actions().Unordered()
	for _, d := range docs {
		al.Put(d)
	}
	if err := al.Do(ctx); err != nil {
		t.Fatal(err)
	}
	iter := coll.Query().Where(KeyField, "<", "qkf3").Get(ctx)
	got := mustCollect(ctx, t, iter)
	for _, g := range got {
		delete(g, docstore.RevisionField)
	}
	want := docs[:2]
	diff := cmp.Diff(got, want, cmpopts.SortSlices(func(d1, d2 docmap) bool {
		return d1[KeyField].(string) < d2[KeyField].(string)
	}))
	if diff != "" {
		t.Error(diff)
	}

	// TODO(jba): test that queries with selected fields always return the key and revision fields.
}

func testGetQuery(t *testing.T, coll *ds.Collection) {
	ctx := context.Background()
	addQueryDocuments(t, coll)

	// TODO(jba): test that queries with selected fields always return the revision field when there is one.

	// Query filters should have the same behavior when doing string and number
	// comparison.
	tests := []struct {
		name   string
		q      *ds.Query
		fields []docstore.FieldPath       // fields to get
		want   func(*HighScore) bool      // filters queryDocuments
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
			name: "GameByPlayer",
			q: coll.Query().Where("Game", "=", game1).Where("Player", ">", "").
				OrderBy("Player", docstore.Ascending),
			want:   func(h *HighScore) bool { return h.Game == game1 },
			before: func(h1, h2 *HighScore) bool { return h1.Player < h2.Player },
		},
		// TODO(jba): add more OrderBy tests.

		{
			name:   "AllWithKeyFields",
			q:      coll.Query(),
			fields: []docstore.FieldPath{"Game", "Player"},
			want: func(h *HighScore) bool {
				h.Score = 0
				h.Time = time.Time{}
				return true
			},
		},
		{
			name:   "AllWithScore",
			q:      coll.Query(),
			fields: []docstore.FieldPath{"Game", "Player", "Score"},
			want: func(h *HighScore) bool {
				h.Time = time.Time{}
				return true
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := collectHighScores(ctx, tc.q.Get(ctx, tc.fields...))
			if gcerrors.Code(err) == gcerrors.Unimplemented {
				t.Skip("unimplemented")
			}
			for _, g := range got {
				g.DocstoreRevision = nil
			}
			want := filterHighScores(queryDocuments, tc.want)
			_, err = tc.q.Plan()
			if err != nil {
				t.Fatal(err)
			}
			diff := cmp.Diff(got, want, cmpopts.SortSlices(func(h1, h2 *HighScore) bool {
				return h1.Game+"|"+h1.Player < h2.Game+"|"+h2.Player
			}))
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

	// For limit, we can't be sure which documents will be returned, only their count.
	limitQ := coll.Query().Limit(2)
	got := mustCollectHighScores(ctx, t, limitQ.Get(ctx))
	if len(got) != 2 {
		t.Errorf("got %v, wanted two documents", got)
	}

	// Errors are returned from the iterator's Next method.
	// TODO(jba): move this test to docstore_test, because it's checked in docstore.go.
	iter := coll.Query().Where("Game", "!=", "").Get(ctx) // != is disallowed
	var h HighScore
	err := iter.Next(ctx, &h)
	if c := gcerrors.Code(err); c != gcerrors.InvalidArgument {
		t.Errorf("got %v, want InvalidArgument", err)
	}
}

func testDeleteQuery(t *testing.T, coll *ds.Collection) {
	ctx := context.Background()

	addQueryDocuments(t, coll)

	// Note: these tests are cumulative. If the first test deletes a document, that
	// change will persist for the second test.
	tests := []struct {
		name string
		q    *ds.Query
		want func(*HighScore) bool // filters queryDocuments
	}{
		{
			name: "Player",
			q:    coll.Query().Where("Player", "=", "andy"),
			want: func(h *HighScore) bool { return h.Player != "andy" },
		},
		{
			name: "Score",
			q:    coll.Query().Where("Score", ">", 100),
			want: func(h *HighScore) bool { return h.Score <= 100 },
		},
		{
			name: "All",
			q:    coll.Query(),
			want: func(h *HighScore) bool { return false },
		},
		// TODO(jba): add a case that requires Firestore to evaluate filters on the client.
	}
	prevWant := queryDocuments
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.q.Delete(ctx); err != nil {
				t.Fatal(err)
			}
			got := mustCollectHighScores(ctx, t, coll.Query().Get(ctx))
			for _, g := range got {
				g.DocstoreRevision = nil
			}
			want := filterHighScores(prevWant, tc.want)
			prevWant = want
			diff := cmp.Diff(got, want, cmpopts.SortSlices(func(h1, h2 *HighScore) bool {
				return h1.Game+"|"+h1.Player < h2.Game+"|"+h2.Player
			}))
			if diff != "" {
				t.Error(diff)
			}
		})
	}

	// Using Limit with DeleteQuery should be an error.
	err := coll.Query().Where("Player", "=", "mel").Limit(1).Delete(ctx)
	if err == nil {
		t.Fatal("want error for Limit, got nil")
	}
}

func testUpdateQuery(t *testing.T, coll *ds.Collection) {
	ctx := context.Background()
	if err := coll.Query().Update(ctx, docstore.Mods{"a": 1}); gcerrors.Code(err) == gcerrors.Unimplemented {
		t.Skip("update queries not yet implemented")
	}

	addQueryDocuments(t, coll)

	err := coll.Query().Where("Player", "=", "fran").Update(ctx, docstore.Mods{"Score": 13, "Time": nil})
	if err != nil {
		t.Fatal(err)
	}
	got := mustCollectHighScores(ctx, t, coll.Query().Get(ctx))
	for _, g := range got {
		g.DocstoreRevision = nil
	}

	want := filterHighScores(queryDocuments, func(h *HighScore) bool {
		if h.Player == "fran" {
			h.Score = 13
			h.Time = time.Time{}
		}
		return true
	})
	diff := cmp.Diff(got, want, cmpopts.SortSlices(func(h1, h2 *HighScore) bool {
		return h1.Game+"|"+h1.Player < h2.Game+"|"+h2.Player
	}))
	if diff != "" {
		t.Error(diff)
	}
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

// cleanUpTable delete all documents from this collection after test.
func cleanUpTable(fataler interface{ Fatalf(string, ...interface{}) }, coll *docstore.Collection) {
	if err := coll.Query().Delete(context.Background()); err != nil {
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

func testMultipleActions(t *testing.T, coll *ds.Collection) {
	ctx := context.Background()

	docs := []docmap{
		{KeyField: "testMultipleActions1", "s": "a"},
		{KeyField: "testMultipleActions2", "s": "b"},
		{KeyField: "testMultipleActions3", "s": "c"},
		{KeyField: "testMultipleActions4", "s": "d"},
		{KeyField: "testMultipleActions5", "s": "e"},
		{KeyField: "testMultipleActions6", "s": "f"},
		{KeyField: "testMultipleActions7", "s": "g"},
		{KeyField: "testMultipleActions8", "s": "h"},
		{KeyField: "testMultipleActions9", "s": "i"},
		{KeyField: "testMultipleActions10", "s": "j"},
		{KeyField: "testMultipleActions11", "s": "k"},
		{KeyField: "testMultipleActions12", "s": "l"},
	}

	actions := coll.Actions()
	// Writes
	for i := 0; i < 6; i++ {
		actions.Create(docs[i])
	}
	for i := 6; i < len(docs); i++ {
		actions.Put(docs[i])
	}

	// Reads
	gots := make([]docmap, len(docs))
	for i, doc := range docs {
		gots[i] = docmap{KeyField: doc[KeyField]}
		actions.Get(gots[i], docstore.FieldPath("s"))
	}
	if err := actions.Do(ctx); err != nil {
		t.Fatal(err)
	}
	for i, got := range gots {
		docs[i][docstore.RevisionField] = got[docstore.RevisionField] // copy the revision
		if diff := cmp.Diff(got, docs[i]); diff != "" {
			t.Error(diff)
		}
	}

	// Deletes
	dels := coll.Actions()
	for _, got := range gots {
		dels.Delete(docmap{KeyField: got[KeyField]})
	}
	if err := dels.Do(ctx); err != nil {
		t.Fatal(err)
	}
}

func testUnorderedActions(t *testing.T, coll *ds.Collection) {
	ctx := context.Background()

	defer cleanUpTable(t, coll)

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	var docs []docmap
	for i := 0; i < 9; i++ {
		docs = append(docs, docmap{KeyField: fmt.Sprintf("testUnorderedActions%d", i), "s": fmt.Sprint(i)})
	}

	compare := func(gots, wants []docmap) {
		t.Helper()
		for i := 0; i < len(gots); i++ {
			got := gots[i]
			want := docmap{}
			for k, v := range wants[i] {
				want[k] = v
			}
			want[docstore.RevisionField] = got[docstore.RevisionField]
			if !cmp.Equal(got, want) {
				t.Errorf("index #%d:\ngot  %v\nwant %v", i, got, want)
			}
		}
	}

	// Put the first three docs.
	actions := coll.Actions().Unordered()
	for i := 0; i < 6; i++ {
		actions.Create(docs[i])
	}
	must(actions.Do(ctx))

	// Replace the first three and put six more.
	actions = coll.Actions().Unordered()
	for i := 0; i < 3; i++ {
		doc := docmap{KeyField: docs[i][KeyField], "s": fmt.Sprintf("%d'", i)}
		actions.Replace(doc)
	}
	for i := 3; i < 9; i++ {
		actions.Put(docs[i])
	}
	must(actions.Do(ctx))

	// Delete the first three, get the second three, and put three more.
	actions = coll.Actions().Unordered()
	gdocs := []docmap{
		{KeyField: docs[3][KeyField]},
		{KeyField: docs[4][KeyField]},
		{KeyField: docs[5][KeyField]},
	}
	actions.Update(docs[6], ds.Mods{"s": "6'"})
	actions.Get(gdocs[0])
	actions.Delete(docs[0])
	actions.Delete(docs[1])
	actions.Update(docs[7], ds.Mods{"s": "7'"})
	actions.Get(gdocs[1])
	actions.Delete(docs[2])
	actions.Get(gdocs[2])
	actions.Update(docs[8], ds.Mods{"s": "8'"})
	must(actions.Do(ctx))
	compare(gdocs, docs[3:6])

	// Get the first four, and try to create one that already exists. Only the
	// fourth should succeed.
	actions = coll.Actions().Unordered()
	gdocs = []docmap{
		{KeyField: docs[0][KeyField]},
		{KeyField: docs[1][KeyField]},
		{KeyField: docs[2][KeyField]},
		{KeyField: docs[3][KeyField]},
	}
	for i := 0; i < len(gdocs); i++ {
		actions.Get(gdocs[i])
	}
	actions.Create(docs[4])
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
		case 3:
			if e.Err != nil {
				t.Errorf("index 3: got %v, want nil", e.Err)
			}
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

func testAs(t *testing.T, coll *ds.Collection, st AsTest) {
	docs := []*HighScore{
		{game3, "steph", 24, date(4, 25), nil},
		{game3, "mia", 99, date(4, 26), nil},
	}

	// Verify Collection.As
	if err := st.CollectionCheck(coll); err != nil {
		t.Error(err)
	}

	ctx := context.Background()
	actions := coll.Actions()
	// Create docs
	for _, doc := range docs {
		actions.Put(doc)
	}
	if err := actions.BeforeDo(st.BeforeDo).Do(ctx); err != nil {
		t.Fatal(err)
	}

	// Get docs
	gets := coll.Actions()
	want := docs[0]
	got := &HighScore{Game: want.Game, Player: want.Player}
	gets.Get(got)
	if err := gets.BeforeDo(st.BeforeDo).Do(ctx); err != nil {
		t.Fatal(err)
	}
	want.DocstoreRevision = got.DocstoreRevision
	if diff := cmp.Diff(got, want); diff != "" {
		t.Error(diff)
	}

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
		iter := q.BeforeQuery(st.BeforeQuery).Get(ctx)
		if err := st.QueryCheck(iter); err != nil {
			t.Error(err)
		}
	}
}
