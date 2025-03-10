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

package mongodocstore

// To run these tests against a real MongoDB server, first run ./localmongo.sh.
// Then wait a few seconds for the server to be ready.

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gocloud.dev/docstore"
	"gocloud.dev/docstore/driver"
	"gocloud.dev/docstore/drivertest"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/internal/testing/setup"
)

const (
	serverURIV3     = "mongodb://localhost:27020"
	serverURIV4     = "mongodb://localhost:27017/?directConnection=true"
	dbName          = "docstore-test"
	collectionName1 = "docstore-test-1"
	collectionName2 = "docstore-test-2"
	collectionName3 = "docstore-test-3"
)

type harness struct {
	db                  *mongo.Database
	supportAtomicWrites bool
}

func (h *harness) MakeCollection(ctx context.Context, kind drivertest.CollectionKind) (driver.Collection, error) {
	var coll *collection
	var err error
	switch kind {
	case drivertest.SingleKey:
		coll, err = newCollection(h.db.Collection(collectionName1), drivertest.KeyField, nil, nil)
	case drivertest.TwoKey:
		coll, err = newCollection(h.db.Collection(collectionName2), "", drivertest.HighScoreKey, nil)
	case drivertest.AltRev:
		coll, err = newCollection(h.db.Collection(collectionName1), drivertest.KeyField, nil,
			&Options{RevisionField: drivertest.AlternateRevisionField})
	case drivertest.NoRev:
		coll, err = newCollection(h.db.Collection(collectionName1), drivertest.KeyField, nil,
			&Options{NoWriteQueryUpdateRevisions: true})
	default:
		panic("bad kind")
	}
	if err != nil {
		return nil, err
	}
	// It seems that the client doesn't actually connect until the first RPC. So
	// time out quickly if there's a problem.
	tctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := coll.coll.Drop(tctx); err != nil {
		return nil, err
	}
	return coll, nil
}

func (h *harness) SupportsAtomicWrites() bool { return h.supportAtomicWrites }

func (*harness) BeforeDoTypes() []interface{} {
	return []interface{}{
		[]mongo.WriteModel{},
		&options.FindOptions{},
	}
}

func (*harness) BeforeQueryTypes() []interface{} {
	return []interface{}{&options.FindOptions{}, bson.D{}}
}

func (*harness) RevisionsEqual(rev1, rev2 interface{}) bool {
	return rev1 == rev2
}

func (*harness) Close() {}

type codecTester struct{}

func (codecTester) UnsupportedTypes() []drivertest.UnsupportedType {
	return []drivertest.UnsupportedType{drivertest.NanosecondTimes}
}

func (codecTester) DocstoreEncode(x interface{}) (interface{}, error) {
	m, err := encodeDoc(drivertest.MustDocument(x), true)
	if err != nil {
		return nil, err
	}
	return bson.Marshal(m)
}

func (codecTester) DocstoreDecode(value, dest interface{}) error {
	var m map[string]interface{}
	if err := bson.Unmarshal(value.([]byte), &m); err != nil {
		return err
	}
	return decodeDoc(m, drivertest.MustDocument(dest), mongoIDField, true)
}

func (codecTester) NativeEncode(x interface{}) (interface{}, error) {
	return bson.Marshal(x)
}

func (codecTester) NativeDecode(value, dest interface{}) error {
	return bson.Unmarshal(value.([]byte), dest)
}

type verifyAs struct{}

func (verifyAs) Name() string {
	return "verify As"
}

func (verifyAs) CollectionCheck(coll *docstore.Collection) error {
	var mc *mongo.Collection
	if !coll.As(&mc) {
		return errors.New("Collection.As failed")
	}
	return nil
}

func (verifyAs) QueryCheck(it *docstore.DocumentIterator) error {
	var c *mongo.Cursor
	if !it.As(&c) {
		return errors.New("DocumentIterator.As failed")
	}
	return nil
}

func (verifyAs) ErrorCheck(c *docstore.Collection, err error) error {
	var cmderr mongo.CommandError
	var bwerr mongo.BulkWriteError
	var bwexc mongo.BulkWriteException
	if !c.ErrorAs(err, &cmderr) && !c.ErrorAs(err, &bwerr) && !c.ErrorAs(err, &bwexc) {
		if e, ok := err.(*gcerr.Error); ok {
			err = e.Unwrap()
		}
		return fmt.Errorf("Collection.ErrorAs failed, got %T", err)
	}
	return nil
}

func TestConformance(t *testing.T) {
	// mongo 3 doesn't support atomic writes
	t.Run("V3", func(t *testing.T) { testConformance(t, serverURIV3, false) })
	t.Run("V4", func(t *testing.T) { testConformance(t, serverURIV4, true) })
}

func testConformance(t *testing.T, serverURI string, supportsAtomicWrites bool) {
	client := newTestClient(t, serverURI)
	defer client.Disconnect(context.Background())

	newHarness := func(context.Context, *testing.T) (drivertest.Harness, error) {
		return &harness{client.Database(dbName), supportsAtomicWrites}, nil
	}
	drivertest.RunConformanceTests(t, newHarness, codecTester{}, []drivertest.AsTest{verifyAs{}})
}

func newTestClient(t *testing.T, serverURI string) *mongo.Client {
	if !setup.HasDockerTestEnvironment() {
		t.Skip("Skipping Mongo tests since the Mongo server is not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := Dial(ctx, serverURI)
	if err != nil {
		t.Fatalf("dialing to %s: %v", serverURI, err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		t.Fatalf("connecting to %s: %v", serverURI, err)
	}
	return client
}

func BenchmarkConformance(b *testing.B) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Only run benchmark once (against MongoDB v4).
	client, err := Dial(ctx, serverURIV4)
	if err != nil {
		b.Fatalf("dialing to %s: %v", serverURIV4, err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		b.Fatalf("connecting to %s: %v", serverURIV4, err)
	}
	defer func() { client.Disconnect(context.Background()) }()

	db := client.Database(dbName)
	coll, err := newCollection(db.Collection(collectionName3), drivertest.KeyField, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	drivertest.RunBenchmarks(b, docstore.NewCollection(coll))
}

// Mongo-specific tests.

// Testing the driver lower all fields in lowercaseFields mode to match the
// behavior of the official Go MongoDB client library.
func TestLowercaseFields(t *testing.T) {
	// Verify that the LowercaseFields option works in all cases.
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	ctx := context.Background()
	client := newTestClient(t, serverURIV4)
	defer func() { client.Disconnect(ctx) }()
	db := client.Database(dbName)
	dc, err := newCollection(db.Collection("lowercase-fields"), "id", nil, &Options{LowercaseFields: true})
	if err != nil {
		t.Fatal(err)
	}
	coll := docstore.NewCollection(dc)
	defer coll.Close()
	defer drivertest.ClearCollection(t, coll)

	type S struct {
		ID, F, G         int
		DocstoreRevision interface{}
	}

	// driver.Document.GetField is case-insensitive on structs.
	doc := drivertest.MustDocument(&S{ID: 1, DocstoreRevision: 1})
	for _, f := range []string{"ID", "Id", "id", "DocstoreRevision", "docstorerevision"} {
		got, err := doc.GetField(f)
		if err != nil {
			t.Errorf("%s: %v", f, err)
		}
		if got != 1 {
			t.Errorf("got %q, want 1", got)
		}
	}

	check := func(got, want interface{}) {
		t.Helper()
		if !cmp.Equal(got, want) {
			t.Errorf("\ngot  %+v\nwant %+v", got, want)
		}
	}

	sdoc := &S{ID: 1, F: 2, G: 3}
	must(coll.Put(ctx, sdoc))
	if sdoc.DocstoreRevision == nil {
		t.Fatal("revision is nil")
	}

	// Get with a struct.
	got := S{ID: 1}
	must(coll.Get(ctx, &got))
	check(got, S{ID: 1, F: 2, G: 3, DocstoreRevision: sdoc.DocstoreRevision})

	// Get with map.
	got2 := map[string]interface{}{"id": 1}
	must(coll.Get(ctx, got2))
	check(got2, map[string]interface{}{
		"id": int64(1), "f": int64(2), "g": int64(3),
		"docstorerevision": sdoc.DocstoreRevision,
	})

	// Field paths in Get.
	got3 := S{ID: 1}
	must(coll.Get(ctx, &got3, "G"))
	check(got3, S{ID: 1, F: 0, G: 3, DocstoreRevision: sdoc.DocstoreRevision})

	// Field paths in Update.
	got4 := map[string]interface{}{"id": 1}
	udoc := &S{ID: 1}
	must(coll.Actions().Update(udoc, docstore.Mods{"F": 4}).Get(got4).Do(ctx))
	check(got4, map[string]interface{}{
		"id": int64(1), "f": int64(4), "g": int64(3),
		"docstorerevision": udoc.DocstoreRevision,
	})

	// Query filters.
	var got5 S
	must(coll.Query().Where("ID", "=", 1).Where("G", ">", 2).Get(ctx).Next(ctx, &got5))
	check(got5, S{ID: 1, F: 4, G: 3, DocstoreRevision: udoc.DocstoreRevision})

	// Query orders.
	sdoc2 := &S{ID: 2, F: 5, G: 6}
	must(coll.Put(ctx, sdoc2))
	var got6 S
	must(coll.Query().OrderBy("G", docstore.Descending).Get(ctx).Next(ctx, &got6))
	check(got6, *sdoc2)

	// List queries
	// select F from coll WHERE G IN (50, 51) ORDER BY G DESC
	// test that F is 99
	sdoc3 := &S{ID: 3, F: 99, G: 50}
	sdoc4 := &S{ID: 4, F: 99, G: 51}
	must(coll.Put(ctx, sdoc3))
	must(coll.Put(ctx, sdoc4))
	var got7, got8 S
	iter := coll.Query().Where("G", "in", []int{50, 51}).OrderBy("G", docstore.Descending).Get(ctx)
	must(iter.Next(ctx, &got7))
	must(iter.Next(ctx, &got8))
	check(got7, *sdoc4)
	check(got8, *sdoc3)

	// same query with not-in, expect to get sdoc2 back even though G is higher for sdoc3 and sdoc4
	var got9 S
	must(coll.Query().Where("G", "not-in", []int{50, 51}).OrderBy("G", docstore.Descending).Get(ctx).Next(ctx, &got9))
	check(got9, *sdoc2)
}
