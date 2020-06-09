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

package gcpfirestore

import (
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gocloud.dev/docstore"
	"gocloud.dev/docstore/drivertest"
	"google.golang.org/genproto/googleapis/type/latlng"
)

// Test that special types round-trip.
// These aren't tested in the docstore-wide conformance tests.
func TestCodecSpecial(t *testing.T) {
	const nameField = "Name"

	type S struct {
		Name    string
		T       time.Time
		TS, TSn *timestamp.Timestamp
		LL, LLn *latlng.LatLng
	}
	tm := time.Date(2019, 3, 14, 0, 0, 0, 0, time.UTC)
	ts, err := ptypes.TimestampProto(tm)
	if err != nil {
		t.Fatal(err)
	}
	in := &S{
		Name: "name",
		T:    tm,
		TS:   ts,
		TSn:  nil,
		LL:   &latlng.LatLng{Latitude: 3, Longitude: 4},
		LLn:  nil,
	}
	var got S

	enc, err := encodeDoc(drivertest.MustDocument(in), nameField)
	if err != nil {
		t.Fatal(err)
	}
	enc.Name = "collPath/" + in.Name
	gotdoc := drivertest.MustDocument(&got)
	// Test type-driven decoding (where the types of the struct fields are available).
	if err := decodeDoc(enc, gotdoc, nameField, docstore.DefaultRevisionField); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(&got, in, cmpopts.IgnoreUnexported(timestamp.Timestamp{}, latlng.LatLng{})); diff != "" {
		t.Error(diff)
	}

	// Test type-blind decoding.
	gotmap := map[string]interface{}{}
	gotmapdoc := drivertest.MustDocument(gotmap)
	if err := decodeDoc(enc, gotmapdoc, nameField, docstore.DefaultRevisionField); err != nil {
		t.Fatal(err)
	}
	wantmap := map[string]interface{}{
		"Name": "name",
		"T":    in.T,
		"TS":   in.T, // timestamps always decode as time.Time
		"TSn":  nil,
		"LL":   in.LL,
		"LLn":  nil,
	}
	if diff := cmp.Diff(gotmap, wantmap, cmpopts.IgnoreUnexported(latlng.LatLng{})); diff != "" {
		t.Error(diff)
	}
}
