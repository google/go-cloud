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
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/driver"
	"gocloud.dev/internal/docstore/drivertest"
	"gocloud.dev/internal/testing/setup"
)

const (
	serverURI       = "mongodb://localhost"
	dbName          = "docstore-test"
	collectionName1 = "docstore-test-1"
	collectionName2 = "docstore-test-2"
	collectionName3 = "docstore-test-3"
)

type harness struct {
	db *mongo.Database
}

func (h *harness) MakeCollection(ctx context.Context) (driver.Collection, error) {
	coll, err := newCollection(h.db.Collection(collectionName1), drivertest.KeyField, nil, nil)
	if err != nil {
		return nil, err
	}
	// It seems that the client doesn't actually connect until the first RPC, which will
	// be this one. So time out quickly if there's a problem.
	tctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := coll.coll.Drop(tctx); err != nil {
		return nil, err
	}
	return coll, nil
}

func (h *harness) MakeTwoKeyCollection(ctx context.Context) (driver.Collection, error) {
	return newCollection(h.db.Collection(collectionName2), "", drivertest.HighScoreKey, nil)
}

func (h *harness) Close() {}

type codecTester struct{}

func (codecTester) UnsupportedTypes() []drivertest.UnsupportedType {
	return []drivertest.UnsupportedType{
		drivertest.Complex, drivertest.NanosecondTimes}
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
	return decodeDoc(m, drivertest.MustDocument(dest), mongoIDField)
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

func (verifyAs) BeforeDo(as func(i interface{}) bool) error {
	var find *options.FindOneOptions
	var insert *options.InsertOneOptions
	var replace *options.ReplaceOptions
	var update *options.UpdateOptions
	var del *options.DeleteOptions
	if !as(&find) && !as(&insert) && !as(&replace) && !as(&update) && !as(&del) {
		return errors.New("ActionList.BeforeDo failed")
	}
	return nil
}

func (verifyAs) BeforeQuery(as func(i interface{}) bool) error {
	var find *options.FindOptions
	if !as(&find) {
		return errors.New("Query.BeforeQuery failed")
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

func TestConformance(t *testing.T) {
	client := newTestClient(t)
	defer func() { client.Disconnect(context.Background()) }()

	newHarness := func(context.Context, *testing.T) (drivertest.Harness, error) {
		return &harness{client.Database(dbName)}, nil
	}
	drivertest.RunConformanceTests(t, newHarness, codecTester{}, []drivertest.AsTest{verifyAs{}})
}

func newTestClient(t *testing.T) *mongo.Client {
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
	client, err := Dial(ctx, serverURI)
	if err != nil {
		b.Fatalf("dialing to %s: %v", serverURI, err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		b.Fatalf("connecting to %s: %v", serverURI, err)
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

func fakeConnectionStringInEnv() func() {
	oldURLVal := os.Getenv("MONGO_SERVER_URL")
	os.Setenv("MONGO_SERVER_URL", "mongodb://localhost")
	return func() {
		os.Setenv("MONGO_SERVER_URL", oldURLVal)
	}
}

func TestOpenCollectionURL(t *testing.T) {
	cleanup := fakeConnectionStringInEnv()
	defer cleanup()

	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"mongo://mydb/mycollection", false},
		// Missing database name.
		{"mongo:///mycollection", true},
		// Missing collection name.
		{"mongo://mydb/", true},
		// Passing id_field parameter.
		{"mongo://mydb/mycollection?id_field=foo", false},
		// Invalid parameter.
		{"mongo://mydb/mycollection?param=value", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		_, err := docstore.OpenCollection(ctx, test.URL)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
	}
}

func TestLowercaseFields(t *testing.T) {
	// Verify that the LowercaseFields option works in all cases.
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	ctx := context.Background()
	client := newTestClient(t)
	defer func() { client.Disconnect(ctx) }()
	db := client.Database(dbName)
	dc, err := newCollection(db.Collection("lowercase-fields"), "id", nil, &Options{LowercaseFields: true})
	if err != nil {
		t.Fatal(err)
	}
	coll := docstore.NewCollection(dc)

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
		switch w := want.(type) {
		case S:
			w.DocstoreRevision = got.(S).DocstoreRevision
			want = w
		case map[string]interface{}:
			w["docstorerevision"] = got.(map[string]interface{})["docstorerevision"]
			want = w
		}
		if !cmp.Equal(got, want) {
			t.Errorf("\ngot  %+v\nwant %+v", got, want)
		}
	}

	must(coll.Put(ctx, &S{ID: 1, F: 2, G: 3}))

	// Get with a struct.
	got := S{ID: 1}
	must(coll.Get(ctx, &got))
	check(got, S{ID: 1, F: 2, G: 3})

	// Get with map.
	got2 := map[string]interface{}{"id": 1}
	must(coll.Get(ctx, got2))
	check(got2, map[string]interface{}{"id": int32(1), "f": int64(2), "g": int64(3)})

	// Field paths in Get.
	got3 := S{ID: 1}
	must(coll.Get(ctx, &got3, "G"))
	check(got3, S{ID: 1, F: 0, G: 3})

	// Field paths in Update.
	got4 := map[string]interface{}{"id": 1}
	must(coll.Actions().Update(&S{ID: 1}, docstore.Mods{"F": 4}).Get(got4).Do(ctx))
	check(got4, map[string]interface{}{"id": int32(1), "f": int64(4), "g": int64(3)})

	// // Query filters.
	var got5 S
	must(coll.Query().Where("ID", "=", 1).Where("G", ">", 2).Get(ctx).Next(ctx, &got5))
	check(got5, S{ID: 1, F: 4, G: 3})
}
