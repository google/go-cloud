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

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/driver"
	"gocloud.dev/internal/docstore/drivertest"
)

const (
	serverURI       = "mongodb://localhost"
	dbName          = "docstore-test"
	collectionName1 = "docstore-test-1"
	collectionName2 = "docstore-test-2"
)

type harness struct {
	db *mongo.Database
}

func (h *harness) MakeCollection(ctx context.Context) (driver.Collection, error) {
	coll := newCollection(h.db.Collection(collectionName1), drivertest.KeyField, nil)
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
	coll := newCollection(h.db.Collection(collectionName2), "", drivertest.HighScoreKey)
	return coll, nil
}

func (h *harness) Close() {}

type codecTester struct{}

func (codecTester) UnsupportedTypes() []drivertest.UnsupportedType {
	return []drivertest.UnsupportedType{
		drivertest.Complex, drivertest.NanosecondTimes}
}

func (codecTester) DocstoreEncode(x interface{}) (interface{}, error) {
	doc, err := driver.NewDocument(x)
	if err != nil {
		return nil, err
	}
	m, err := encodeDoc(doc)
	if err != nil {
		return nil, err
	}
	return bson.Marshal(m)
}

func (codecTester) DocstoreDecode(value, dest interface{}) error {
	doc, err := driver.NewDocument(dest)
	if err != nil {
		return err
	}
	var m map[string]interface{}
	if err := bson.Unmarshal(value.([]byte), &m); err != nil {
		return err
	}
	return decodeDoc(m, doc, mongoIDField)
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

func (verifyAs) BeforeQuery(as func(i interface{}) bool) error {
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
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := Dial(ctx, serverURI)
	if err != nil {
		t.Fatalf("dialing to %s: %v", serverURI, err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		if err == context.DeadlineExceeded {
			t.Skip("could not connect to local mongoDB server (connection timed out)")
		}
		t.Fatalf("connecting to %s: %v", serverURI, err)
	}
	defer func() { client.Disconnect(context.Background()) }()

	newHarness := func(context.Context, *testing.T) (drivertest.Harness, error) {
		return &harness{client.Database(dbName)}, nil
	}
	drivertest.RunConformanceTests(t, newHarness, codecTester{}, []drivertest.AsTest{verifyAs{}})
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
