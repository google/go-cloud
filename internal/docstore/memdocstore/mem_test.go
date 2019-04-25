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
	"testing"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/driver"
	"gocloud.dev/internal/docstore/drivertest"
)

type harness struct{}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	return &harness{}, nil
}

func (h *harness) MakeCollection(context.Context) (driver.Collection, error) {
	return newCollection(drivertest.KeyField, nil)
}

func (h *harness) Close() {}

func TestConformance(t *testing.T) {
	// CodecTester is nil because memdocstore has no native representation.
	drivertest.RunConformanceTests(t, newHarness, nil)
}

type docmap = map[string]interface{}

func TestUpdateAtomic(t *testing.T) {
	// Check that update is atomic.
	ctx := context.Background()
	dc, err := newCollection(drivertest.KeyField, nil)
	if err != nil {
		t.Fatal(err)
	}
	coll := docstore.NewCollection(dc)
	doc := docmap{drivertest.KeyField: "testUpdateAtomic", "a": "A", "b": "B"}

	mods := docstore.Mods{"a": "Y", "b.c": "Z"} // "b" is not a map, so "b.c" is an error
	if errs := coll.Actions().Put(doc).Update(doc, mods).Do(ctx); errs == nil {
		t.Fatal("got nil, want errors")
	}
	got := docmap{drivertest.KeyField: doc[drivertest.KeyField]}
	if err := coll.Get(ctx, got); err != nil {
		t.Fatal(err)
	}
	want := docmap{
		drivertest.KeyField:    doc[drivertest.KeyField],
		docstore.RevisionField: got[docstore.RevisionField],
		"a":                    "A",
		"b":                    "B",
	}
	if !cmp.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestOpenCollectionFromURL(t *testing.T) {
	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"mem://_id", false},
		{"mem://foo.bar", false},
		{"mem://", true},             // missing key
		{"mem://?param=value", true}, // invalid parameter
	}
	ctx := context.Background()
	for _, test := range tests {
		_, err := docstore.OpenCollection(ctx, test.URL)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
	}
}

func TestMissingKeyCreateFailsWithKeyFunc(t *testing.T) {
	dc, err := newCollection("", func(docstore.Document) interface{} { return nil })
	if err != nil {
		t.Fatal(err)
	}
	c := docstore.NewCollection(dc)
	err = c.Create(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Error("got nil, want error")
	}
}
