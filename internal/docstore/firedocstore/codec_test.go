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
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	ts "github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/go-cmp/cmp"
	"gocloud.dev/internal/docstore/driver"
	"google.golang.org/genproto/googleapis/type/latlng"
)

// Test that special types round-trip.
// These aren't tested in the docstore-wide conformance tests.
func TestCodecSpecial(t *testing.T) {
	mustdoc := func(x interface{}) driver.Document {
		t.Helper()
		doc, err := driver.NewDocument(x)
		if err != nil {
			t.Fatal(err)
		}
		return doc
	}

	type S struct {
		T       time.Time
		TS, TSn *ts.Timestamp
		LL, LLn *latlng.LatLng
	}
	tm := time.Date(2019, 3, 14, 0, 0, 0, 0, time.UTC)
	ts, err := ptypes.TimestampProto(tm)
	if err != nil {
		t.Fatal(err)
	}
	in := &S{
		T:   tm,
		TS:  ts,
		TSn: nil,
		LL:  &latlng.LatLng{Latitude: 3, Longitude: 4},
		LLn: nil,
	}
	var got S

	enc, err := encodeDoc(mustdoc(in))
	if err != nil {
		t.Fatal(err)
	}
	gotdoc := mustdoc(&got)
	// Test type-driven decoding (where the types of the struct fields are availableO).
	if err := decodeDoc(enc, gotdoc); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(&got, in); diff != "" {
		t.Error(diff)
	}

	// Test type-blind decoding.
	gotmap := map[string]interface{}{}
	gotmapdoc := mustdoc(gotmap)
	if err := decodeDoc(enc, gotmapdoc); err != nil {
		t.Fatal(err)
	}
	wantmap := map[string]interface{}{
		"T":   in.T,
		"TS":  in.T, // timestamps always decode as time.Time
		"TSn": nil,
		"LL":  in.LL,
		"LLn": nil,
	}
	if diff := cmp.Diff(gotmap, wantmap); diff != "" {
		t.Error(diff)
	}
}
