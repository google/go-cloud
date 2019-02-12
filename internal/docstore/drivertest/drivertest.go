// Copyright 2019 The Go Cloud Authors
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
	//	"math"
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

// RunConformanceTests runs conformance tests for provider implementations of docstore.
func RunConformanceTests(t *testing.T, newHarness HarnessMaker) {
	t.Run("Create", func(t *testing.T) { withCollection(t, newHarness, testCreate) })
	t.Run("Put", func(t *testing.T) { withCollection(t, newHarness, testPut) })
	t.Run("Replace", func(t *testing.T) { withCollection(t, newHarness, testReplace) })
	t.Run("Get", func(t *testing.T) { withCollection(t, newHarness, testGet) })
	t.Run("Delete", func(t *testing.T) { withCollection(t, newHarness, testDelete) })
	t.Run("Update", func(t *testing.T) { withCollection(t, newHarness, testUpdate) })
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
	named := docmap{KeyField: "testCreate1", "b": true}
	unnamed := docmap{"b": false}
	// Attempt to clean up
	defer func() {
		_, _ = coll.Actions().Delete(named).Delete(unnamed).Do(context.Background())
	}()

	mustRunAndCompare(t, coll, named, coll.Actions().Create(named))
	mustRunAndCompare(t, coll, unnamed, coll.Actions().Create(unnamed))
	// Can't create an existing doc.
	shouldFail(t, coll.Actions().Create(named))
}

func testPut(t *testing.T, coll *ds.Collection) {
	named := docmap{KeyField: "testPut1", "b": true}
	mustRunAndCompare(t, coll, named, coll.Actions().Put(named)) // create new
	named["b"] = false
	mustRunAndCompare(t, coll, named, coll.Actions().Put(named)) // replace existing
}

func testReplace(t *testing.T, coll *ds.Collection) {
	doc1 := docmap{KeyField: "testReplace", "s": "a"}
	mustRun(t, coll.Actions().Put(doc1))

	doc1["s"] = "b"
	mustRunAndCompare(t, coll, doc1, coll.Actions().Replace(doc1))

	// Can't replace a nonexistent doc.
	shouldFail(t, coll.Actions().Replace(nonexistentDoc))
}

func testGet(t *testing.T, coll *ds.Collection) {
	doc := docmap{
		KeyField: "testGet1",
		"s":      "a string",
		"i":      int64(95),
		"f":      32.3,
	}
	mustRun(t, coll.Actions().Put(doc))

	checkGet := func(got, want docmap) {
		t.Helper()
		mustRun(t, coll.Actions().Get(got))
		if diff := cmp.Diff(got, want); diff != "" {
			t.Errorf("got=-, want=+: %s", diff)
		}
	}

	// If only the key fields are present, the full document is populated.
	checkGet(docmap{KeyField: doc[KeyField]}, doc)
}

func testDelete(t *testing.T, coll *ds.Collection) {
	doc := docmap{KeyField: "testDelete"}
	mustRun(t, coll.Actions().Put(doc).Delete(doc))
	shouldFail(t, coll.Actions().Get(doc))
	// Delete doesn't fail if the doc doesn't exist.
	mustRun(t, coll.Actions().Delete(nonexistentDoc))
}

func testUpdate(t *testing.T, coll *ds.Collection) {
	doc := docmap{KeyField: "testUpdate", "a": "A", "b": "B"}
	mustRun(t, coll.Actions().Put(doc))

	got := docmap{KeyField: doc[KeyField]}
	mustRun(t, coll.Actions().Update(doc, ds.Mods{
		"a": "X",
		"b": nil,
		"c": "C",
	}).Get(got))
	want := docmap{
		KeyField: doc[KeyField],
		"a":      "X",
		"c":      "C",
	}
	if !cmp.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	// Can't update a nonexistent doc
	shouldFail(t, coll.Actions().Update(nonexistentDoc, ds.Mods{}))
}

func mustRun(t *testing.T, al *ds.ActionList) {
	t.Helper()
	if _, err := al.Do(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func mustRunAndCompare(t *testing.T, coll *ds.Collection, doc docmap, al *ds.ActionList) {
	t.Helper()
	mustRun(t, al)
	got := docmap{KeyField: doc[KeyField]}
	mustRun(t, coll.Actions().Get(got))
	if diff := cmp.Diff(got, doc); diff != "" {
		t.Fatalf(diff)
	}
}

func shouldFail(t *testing.T, al *ds.ActionList) {
	t.Helper()
	if _, err := al.Do(context.Background()); err == nil {
		t.Error("got nil, want error")
	}
}
