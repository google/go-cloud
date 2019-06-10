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

package firedocstore

import (
	"context"
	"errors"
	"testing"

	vkit "cloud.google.com/go/firestore/apiv1"
	"github.com/golang/protobuf/proto"
	"gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/driver"
	"gocloud.dev/internal/docstore/drivertest"
	"gocloud.dev/internal/testing/setup"
	"google.golang.org/api/option"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
	"google.golang.org/grpc/status"
)

const (
	// projectID is the project ID that was used during the last test run using --record.
	projectID       = "go-cloud-test-216917"
	collectionName1 = "docstore-test-1"
	collectionName2 = "docstore-test-2"
	collectionName3 = "docstore-test-3"
	endPoint        = "firestore.googleapis.com:443"
)

type harness struct {
	client *vkit.Client
	done   func()
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	conn, done := setup.NewGCPgRPCConn(ctx, t, endPoint, "docstore")
	client, err := vkit.NewClient(ctx, option.WithGRPCConn(conn))
	if err != nil {
		done()
		return nil, err
	}
	return &harness{client, done}, nil
}

func (h *harness) MakeCollection(context.Context) (driver.Collection, error) {
	return newCollection(h.client, projectID, collectionName1, drivertest.KeyField, nil, nil)
}

func (h *harness) MakeTwoKeyCollection(context.Context) (driver.Collection, error) {
	return newCollection(h.client, projectID, collectionName2, "", func(doc docstore.Document) string {
		return drivertest.HighScoreKey(doc).(string)
	}, &Options{AllowLocalFilters: true})
}

func (h *harness) MakeAlternateRevisionFieldCollection(context.Context) (driver.Collection, error) {
	return newCollection(h.client, projectID, collectionName1, drivertest.KeyField, nil,
		&Options{RevisionField: drivertest.AlternateRevisionField})
}

func (*harness) BeforeDoTypes() []interface{} {
	return []interface{}{&pb.BatchGetDocumentsRequest{}, &pb.CommitRequest{}}
}

func (h *harness) Close() {
	_ = h.client.Close()
	h.done()
}

// codecTester implements drivertest.CodecTester.
type codecTester struct {
	nc *nativeCodec
}

func (*codecTester) UnsupportedTypes() []drivertest.UnsupportedType {
	return []drivertest.UnsupportedType{drivertest.Uint, drivertest.Arrays}
}

func (c *codecTester) NativeEncode(x interface{}) (interface{}, error) {
	return c.nc.Encode(x)
}

func (c *codecTester) NativeDecode(value, dest interface{}) error {
	return c.nc.Decode(value.(*pb.Document), dest)
}

func (c *codecTester) DocstoreEncode(x interface{}) (interface{}, error) {
	var e encoder
	if err := drivertest.MustDocument(x).Encode(&e); err != nil {
		return nil, err
	}
	return &pb.Document{Fields: e.pv.GetMapValue().Fields}, nil
}

func (c *codecTester) DocstoreDecode(value, dest interface{}) error {
	mv := &pb.Value{ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{
		Fields: value.(*pb.Document).Fields,
	}}}
	return drivertest.MustDocument(dest).Decode(decoder{mv})
}

type verifyAs struct{}

func (verifyAs) Name() string {
	return "verify As"
}

func (verifyAs) CollectionCheck(coll *docstore.Collection) error {
	var fc *vkit.Client
	if !coll.As(&fc) {
		return errors.New("Collection.As failed")
	}
	return nil
}

func (verifyAs) BeforeQuery(as func(i interface{}) bool) error {
	var req *pb.RunQueryRequest
	if !as(&req) {
		return errors.New("Query.BeforeQuery failed")
	}
	_ = req.GetStructuredQuery()
	return nil
}

func (verifyAs) QueryCheck(it *docstore.DocumentIterator) error {
	var c pb.Firestore_RunQueryClient
	if !it.As(&c) {
		return errors.New("DocumentIterator.As failed")
	}
	_, err := c.Header()
	return err
}

func (v verifyAs) ErrorCheck(c *docstore.Collection, err error) error {
	var s *status.Status
	if !c.ErrorAs(err, &s) {
		return errors.New("Collection.ErrorAs failed")
	}
	return nil
}

func TestConformance(t *testing.T) {
	drivertest.MakeUniqueStringDeterministicForTesting(1)
	maxWritesPerRPC = 2 // to test RPC batching behavior
	nc, err := newNativeCodec()
	if err != nil {
		t.Fatal(err)
	}
	drivertest.RunConformanceTests(t, newHarness, &codecTester{nc}, []drivertest.AsTest{verifyAs{}})
}

func BenchmarkConformance(b *testing.B) {
	ctx := context.Background()
	client, err := vkit.NewClient(ctx)
	if err != nil {
		b.Fatal(err)
	}
	coll, err := newCollection(client, projectID, collectionName3, drivertest.KeyField, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	drivertest.RunBenchmarks(b, docstore.NewCollection(coll))
}

// Firedocstore-specific tests.

func TestNewGetRequest(t *testing.T) {
	c := &collection{dbPath: "dbPath", collPath: "collPath", nameField: "name"}
	actions := []*driver.Action{
		{Kind: driver.Get, Key: "a", Doc: drivertest.MustDocument(map[string]interface{}{"name": "a"})},
		{Kind: driver.Get, Key: "b", Doc: drivertest.MustDocument(map[string]interface{}{"name": "b"})},
	}
	got, err := c.newGetRequest(actions)
	if err != nil {
		t.Fatal(err)
	}
	want := &pb.BatchGetDocumentsRequest{
		Database:  "dbPath",
		Documents: []string{"collPath/a", "collPath/b"},
	}
	if !proto.Equal(got, want) {
		t.Errorf("\ngot  %v\nwant %v", got, want)
	}
}
