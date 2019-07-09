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

package memdocstore

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/docstore"
	"gocloud.dev/docstore/driver"
	"gocloud.dev/docstore/drivertest"
)

type harness struct{}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	return &harness{}, nil
}

func (h *harness) MakeCollection(context.Context) (driver.Collection, error) {
	return newCollection(drivertest.KeyField, nil, nil)
}

func (h *harness) MakeTwoKeyCollection(context.Context) (driver.Collection, error) {
	return newCollection("", drivertest.HighScoreKey, nil)
}

func (h *harness) MakeAlternateRevisionFieldCollection(context.Context) (driver.Collection, error) {
	return newCollection(drivertest.KeyField, nil, &Options{RevisionField: drivertest.AlternateRevisionField})
}

func (*harness) BeforeDoTypes() []interface{}    { return nil }
func (*harness) BeforeQueryTypes() []interface{} { return nil }

func (*harness) RevisionsEqual(rev1, rev2 interface{}) bool { return rev1 == rev2 }

func (*harness) Close() {}

func TestConformance(t *testing.T) {
	// CodecTester is nil because memdocstore has no native representation.
	drivertest.RunConformanceTests(t, newHarness, nil, nil)
}

type docmap = map[string]interface{}

func TestUpdateEncodesValues(t *testing.T) {
	// Check that update encodes the values in mods.
	ctx := context.Background()
	dc, err := newCollection(drivertest.KeyField, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	coll := docstore.NewCollection(dc)
	defer coll.Close()
	doc := docmap{drivertest.KeyField: "testUpdateEncodes", "a": 1}
	if err := coll.Put(ctx, doc); err != nil {
		t.Fatal(err)
	}
	if err := coll.Update(ctx, doc, docstore.Mods{"a": 2}); err != nil {
		t.Fatal(err)
	}
	got := docmap{drivertest.KeyField: doc[drivertest.KeyField]}
	// This Get will fail if the int value 2 in the above mod was not encoded to an int64.
	if err := coll.Get(ctx, got); err != nil {
		t.Fatal(err)
	}
	want := docmap{
		drivertest.KeyField: doc[drivertest.KeyField],
		"a":                 int64(2),
		dc.RevisionField():  got[dc.RevisionField()],
	}
	if !cmp.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestUpdateAtomic(t *testing.T) {
	// Check that update is atomic.
	ctx := context.Background()
	dc, err := newCollection(drivertest.KeyField, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	coll := docstore.NewCollection(dc)
	defer coll.Close()
	doc := docmap{drivertest.KeyField: "testUpdateAtomic", "a": "A", "b": "B"}

	mods := docstore.Mods{"a": "Y", "b.c": "Z"} // "b" is not a map, so "b.c" is an error
	if err := coll.Put(ctx, doc); err != nil {
		t.Fatal(err)
	}
	if errs := coll.Actions().Update(doc, mods).Do(ctx); errs == nil {
		t.Fatal("got nil, want errors")
	}
	got := docmap{drivertest.KeyField: doc[drivertest.KeyField]}
	if err := coll.Get(ctx, got); err != nil {
		t.Fatal(err)
	}
	want := docmap{
		drivertest.KeyField: doc[drivertest.KeyField],
		dc.RevisionField():  got[dc.RevisionField()],
		"a":                 "A",
		"b":                 "B",
	}
	if !cmp.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMissingKeyCreateFailsWithKeyFunc(t *testing.T) {
	dc, err := newCollection("", func(docstore.Document) interface{} { return nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	c := docstore.NewCollection(dc)
	defer c.Close()
	err = c.Create(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Error("got nil, want error")
	}
}

func TestSortDocs(t *testing.T) {
	newDocs := func() []docmap {
		return []docmap{
			{"a": int64(1), "b": "1", "c": 3.0},
			{"a": int64(2), "b": "2", "c": 4.0},
			{"a": int64(3), "b": "3"}, // missing "c"
		}
	}
	inorder := newDocs()
	reversed := newDocs()
	for i := 0; i < len(reversed)/2; i++ {
		j := len(reversed) - i - 1
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}

	for _, test := range []struct {
		field     string
		ascending bool
		want      []docmap
	}{
		{"a", true, inorder},
		{"a", false, reversed},
		{"b", true, inorder},
		{"b", false, reversed},
		{"c", true, inorder},
		{"c", false, []docmap{inorder[1], inorder[0], inorder[2]}},
	} {
		got := newDocs()
		sortDocs(got, test.field, test.ascending)
		if diff := cmp.Diff(got, test.want); diff != "" {
			t.Errorf("%q, asc=%t:\n%s", test.field, test.ascending, diff)
		}
	}
}

func TestExampleInDoc(t *testing.T) {
	// This is the document used as an example in ../docstore/doc.go.
	doc := map[string]interface{}{
		"Title": "The Master and Margarita",
		"Author": map[string]interface{}{
			"First": "Mikhail",
			"Last":  "Bulgakov",
		},
		"PublicationYears": []int{1967, 1973},
	}

	dc, err := newCollection("Title", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	coll := docstore.NewCollection(dc)
	defer coll.Close()
	ctx := context.Background()
	if err := coll.Put(ctx, doc); err != nil {
		t.Fatal(err)
	}
	got := map[string]interface{}{"Title": "The Master and Margarita"}
	if err := coll.Get(ctx, got); err != nil {
		t.Fatal(err)
	}
	want := map[string]interface{}{
		"Title": "The Master and Margarita",
		"Author": map[string]interface{}{
			"First": "Mikhail",
			"Last":  "Bulgakov",
		},
		"PublicationYears": []interface{}{int64(1967), int64(1973)},
		"DocstoreRevision": int64(1),
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Error(diff)
	}
}

func TestSaveAndLoad(t *testing.T) {
	f, err := ioutil.TempFile("", "testSaveAndLoad")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	docs := map[interface{}]map[string]interface{}{
		"k1": {"key": "k1", "a": 1},
		"k2": {"key": "k2", "b": 2},
	}
	if err := saveDocs(f.Name(), docs); err != nil {
		t.Fatal(err)
	}
	got, err := loadDocs(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !cmp.Equal(got, docs) {
		t.Errorf("\ngot  %v\nwant %v", got, docs)
	}
}
