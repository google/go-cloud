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
	"testing"

	"github.com/google/go-cmp/cmp"
	ds "gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/driver"
)

// Harness descibes the functionality test harnesses must provide to run
// conformance tests.
type Harness interface {
	// MakeCollection makes a driver.Collection for testing.
	MakeCollection(context.Context) (driver.Collection, error)

	// Close closes resources used by the harness.
	Close()
}

// HarnessMaker describes functions that construct a harness for running tests.
// It is called exactly once per test; Harness.Close() will be called when the test is complete.
type HarnessMaker func(ctx context.Context, t *testing.T) (Harness, error)

// CodecTester describes functions that encode and decode values using both the
// docstore codec for a provider, and that provider's own "native" codec.
type CodecTester interface {
	NativeEncode(interface{}) (interface{}, error)
	NativeDecode(value, dest interface{}) error
	DocstoreEncode(interface{}) (interface{}, error)
	DocstoreDecode(value, dest interface{}) error
}

// RunConformanceTests runs conformance tests for provider implementations of docstore.
func RunConformanceTests(t *testing.T, newHarness HarnessMaker, ct CodecTester) {
	t.Run("Create", func(t *testing.T) { withCollection(t, newHarness, testCreate) })
	t.Run("Put", func(t *testing.T) { withCollection(t, newHarness, testPut) })
	t.Run("Replace", func(t *testing.T) { withCollection(t, newHarness, testReplace) })
	t.Run("Get", func(t *testing.T) { withCollection(t, newHarness, testGet) })
	t.Run("Delete", func(t *testing.T) { withCollection(t, newHarness, testDelete) })
	t.Run("Update", func(t *testing.T) { withCollection(t, newHarness, testUpdate) })
	t.Run("Data", func(t *testing.T) { withCollection(t, newHarness, testData) })
	t.Run("Codec", func(t *testing.T) { testCodec(t, ct) })
}

const KeyField = "_id"

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
	f(t, coll)
}

type docmap = map[string]interface{}

var nonexistentDoc = docmap{KeyField: "doesNotExist"}

func testCreate(t *testing.T, coll *ds.Collection) {
	ctx := context.Background()
	named := docmap{KeyField: "testCreate1", "b": true}
	unnamed := docmap{"b": false}
	// Attempt to clean up
	defer func() {
		_, _ = coll.Actions().Delete(named).Delete(unnamed).Do(ctx)
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
		if diff := cmp.Diff(got, doc); diff != "" {
			t.Fatal(diff)
		}
	}

	createThenGet(named)
	createThenGet(unnamed)

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
	if diff := cmp.Diff(got, named); diff != "" {
		t.Fatalf(diff)
	}

	// Replace an existing doc.
	named["b"] = false
	must(coll.Put(ctx, named))
	must(coll.Get(ctx, got))
	if diff := cmp.Diff(got, named); diff != "" {
		t.Fatalf(diff)
	}
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
	if diff := cmp.Diff(got, doc1); diff != "" {
		t.Fatalf(diff)
	}
	// Can't replace a nonexistent doc.
	if err := coll.Replace(ctx, nonexistentDoc); err == nil {
		t.Fatal("got nil, want error")
	}
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
	}
	must(coll.Put(ctx, doc))
	// If only the key fields are present, the full document is populated.
	got := docmap{KeyField: doc[KeyField]}
	must(coll.Get(ctx, got))
	if diff := cmp.Diff(got, doc); diff != "" {
		t.Error(diff)
	}
	// TODO(jba): test with field paths
}

func testDelete(t *testing.T, coll *ds.Collection) {
	ctx := context.Background()
	doc := docmap{KeyField: "testDelete"}
	if n, err := coll.Actions().Put(doc).Delete(doc).Do(ctx); err != nil {
		t.Fatalf("after %d successful actions: %v", n, err)
	}
	// The document should no longer exist.
	if err := coll.Get(ctx, doc); err == nil {
		t.Error("want error, got nil")
	}
	// Delete doesn't fail if the doc doesn't exist.
	if err := coll.Delete(ctx, nonexistentDoc); err != nil {
		t.Fatal(err)
	}
}

func testUpdate(t *testing.T, coll *ds.Collection) {
	ctx := context.Background()
	doc := docmap{KeyField: "testUpdate", "a": "A", "b": "B"}
	if err := coll.Put(ctx, doc); err != nil {
		t.Fatal(err)
	}

	got := docmap{KeyField: doc[KeyField]}
	_, err := coll.Actions().Update(doc, ds.Mods{
		"a": "X",
		"b": nil,
		"c": "C",
	}).Get(got).Do(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := docmap{
		KeyField: doc[KeyField],
		"a":      "X",
		"c":      "C",
	}
	if !cmp.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	// Can't update a nonexistent doc
	if err := coll.Update(ctx, nonexistentDoc, ds.Mods{}); err == nil {
		t.Error("got nil, want error")
	}

	// Check that update is atomic.
	doc = got
	mods := ds.Mods{"a": "Y", "c.d": "Z"} // "c" is not a map, so "c.d" is an error
	if err := coll.Update(ctx, doc, mods); err == nil {
		t.Fatal("got nil, want error")
	}
	got = docmap{KeyField: doc[KeyField]}
	if err := coll.Get(ctx, got); err != nil {
		t.Fatal(err)
	}
	// want should be unchanged
	if !cmp.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
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
		//		{uint(1), int64(1)}, TODO(jba): support uint in firestore
		{uint8(8), int64(8)},
		{uint16(16), int64(16)},
		{uint32(32), int64(32)},
		// TODO(jba): support uint64
		{float32(3.5), float64(3.5)},
		{[]byte{0, 1, 2}, []byte{0, 1, 2}},
	} {
		doc := docmap{KeyField: "testData", "val": test.in}
		got := docmap{KeyField: doc[KeyField]}
		if _, err := coll.Actions().Put(doc).Get(got).Do(ctx); err != nil {
			t.Fatal(err)
		}
		want := docmap{KeyField: doc[KeyField], "val": test.want}
		if len(got) != len(want) {
			t.Errorf("%v: got %v, want %v", test.in, got, want)
		} else if g := got["val"]; !cmp.Equal(g, test.want) {
			t.Errorf("%v: got %v (%T), want %v (%T)", test.in, g, g, test.want, test.want)
		}
	}

	// TODO: strings: valid vs. invalid unicode

}

func testCodec(t *testing.T, ct CodecTester) {
	if ct == nil {
		t.Skip("no CodecTester")
	}

	type S struct {
		N  *int
		I  int
		U  uint
		F  float64
		C  complex64
		St string
		B  bool
		By []byte
		L  []int
		M  map[string]bool
	}
	// TODO(jba): add more fields: more basic types; pointers; structs; embedding.

	in := S{
		N:  nil,
		I:  1,
		U:  2,
		F:  2.5,
		C:  complex(9, 10),
		St: "foo",
		B:  true,
		L:  []int{3, 4, 5},
		M:  map[string]bool{"a": true, "b": false},
		By: []byte{6, 7, 8},
	}

	check := func(encode func(interface{}) (interface{}, error), decode func(interface{}, interface{}) error) {
		t.Helper()
		enc, err := encode(in)
		if err != nil {
			t.Fatal(err)
		}
		var dec S
		if err := decode(enc, &dec); err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(in, dec); diff != "" {
			t.Error(diff)
		}
	}

	check(ct.DocstoreEncode, ct.DocstoreDecode)
	check(ct.DocstoreEncode, ct.NativeDecode)
	check(ct.NativeEncode, ct.DocstoreDecode)
}
