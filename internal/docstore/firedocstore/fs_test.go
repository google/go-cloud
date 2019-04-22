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
	"testing"

	vkit "cloud.google.com/go/firestore/apiv1"
	"gocloud.dev/internal/docstore/driver"
	"gocloud.dev/internal/docstore/drivertest"
	"gocloud.dev/internal/testing/setup"
	"google.golang.org/api/option"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
)

const (
	// projectID is the project ID that was used during the last test run using --record.
	projectID      = "go-cloud-test-216917"
	collectionName = "docstore-test"
	keyName        = "_id"
	endPoint       = "firestore.googleapis.com:443"
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
	return newCollection(h.client, projectID, collectionName, Options{NameField: keyName})
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
	return []drivertest.UnsupportedType{drivertest.Uint, drivertest.Complex, drivertest.Arrays}
}

func (c *codecTester) NativeEncode(x interface{}) (interface{}, error) {
	return c.nc.Encode(x)
}

func (c *codecTester) NativeDecode(value, dest interface{}) error {
	return c.nc.Decode(value.(*pb.Document), dest)
}

func (c *codecTester) DocstoreEncode(x interface{}) (interface{}, error) {
	doc, err := driver.NewDocument(x)
	if err != nil {
		return nil, err
	}
	var e encoder
	if err := doc.Encode(&e); err != nil {
		return nil, err
	}
	return &pb.Document{Fields: e.pv.GetMapValue().Fields}, nil
}

func (c *codecTester) DocstoreDecode(value, dest interface{}) error {
	doc, err := driver.NewDocument(dest)
	if err != nil {
		return err
	}
	mv := &pb.Value{ValueType: &pb.Value_MapValue{&pb.MapValue{Fields: value.(*pb.Document).Fields}}}
	return doc.Decode(decoder{mv})
}

func TestConformance(t *testing.T) {
	drivertest.MakeUniqueStringDeterministicForTesting(1)
	nc, err := newNativeCodec()
	if err != nil {
		t.Fatal(err)
	}
	drivertest.RunConformanceTests(t, newHarness, &codecTester{nc})
}
