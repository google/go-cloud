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
	"math"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	"gocloud.dev/docstore/driver"
	"gocloud.dev/docstore/drivertest"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
)

func TestFilterToProto(t *testing.T) {
	c := &collection{nameField: "name", collPath: "collPath"}
	for _, test := range []struct {
		in   driver.Filter
		want *pb.StructuredQuery_Filter
	}{
		{
			driver.Filter{[]string{"a"}, ">", 1},
			&pb.StructuredQuery_Filter{FilterType: &pb.StructuredQuery_Filter_FieldFilter{
				FieldFilter: &pb.StructuredQuery_FieldFilter{
					Field: &pb.StructuredQuery_FieldReference{FieldPath: "a"},
					Op:    pb.StructuredQuery_FieldFilter_GREATER_THAN,
					Value: &pb.Value{ValueType: &pb.Value_IntegerValue{1}},
				},
			}},
		},
		{
			driver.Filter{[]string{"a"}, driver.EqualOp, nil},
			&pb.StructuredQuery_Filter{FilterType: &pb.StructuredQuery_Filter_UnaryFilter{
				UnaryFilter: &pb.StructuredQuery_UnaryFilter{
					OperandType: &pb.StructuredQuery_UnaryFilter_Field{
						Field: &pb.StructuredQuery_FieldReference{FieldPath: "a"},
					},
					Op: pb.StructuredQuery_UnaryFilter_IS_NULL,
				},
			}},
		},
		{
			driver.Filter{[]string{"a"}, driver.EqualOp, math.NaN()},
			&pb.StructuredQuery_Filter{FilterType: &pb.StructuredQuery_Filter_UnaryFilter{
				UnaryFilter: &pb.StructuredQuery_UnaryFilter{
					OperandType: &pb.StructuredQuery_UnaryFilter_Field{
						Field: &pb.StructuredQuery_FieldReference{FieldPath: "a"},
					},
					Op: pb.StructuredQuery_UnaryFilter_IS_NAN,
				},
			}},
		},
		{
			driver.Filter{[]string{"name"}, "<", "foo"},
			&pb.StructuredQuery_Filter{FilterType: &pb.StructuredQuery_Filter_FieldFilter{
				FieldFilter: &pb.StructuredQuery_FieldFilter{
					Field: &pb.StructuredQuery_FieldReference{FieldPath: "__name__"},
					Op:    pb.StructuredQuery_FieldFilter_LESS_THAN,
					Value: &pb.Value{ValueType: &pb.Value_ReferenceValue{"collPath/foo"}},
				},
			}},
		},
	} {
		got, err := c.filterToProto(test.in)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(got, test.want, cmp.Comparer(proto.Equal)); diff != "" {
			t.Errorf("%+v: %s", test.in, diff)
		}
	}
}

func TestSplitFilters(t *testing.T) {
	aEqual := driver.Filter{[]string{"a"}, "=", 1}
	aLess := driver.Filter{[]string{"a"}, "<", 1}
	aGreater := driver.Filter{[]string{"a"}, ">", 1}
	bEqual := driver.Filter{[]string{"b"}, "=", 1}
	bLess := driver.Filter{[]string{"b"}, "<", 1}

	for _, test := range []struct {
		in                  []driver.Filter
		wantSend, wantLocal []driver.Filter
	}{
		{
			in:        nil,
			wantSend:  nil,
			wantLocal: nil,
		},
		{
			in:        []driver.Filter{aEqual},
			wantSend:  []driver.Filter{aEqual},
			wantLocal: nil,
		},
		{
			in:        []driver.Filter{aLess},
			wantSend:  []driver.Filter{aLess},
			wantLocal: nil,
		},
		{
			in:        []driver.Filter{aLess, aGreater},
			wantSend:  []driver.Filter{aLess, aGreater},
			wantLocal: nil,
		},
		{
			in:        []driver.Filter{aLess, bEqual, aGreater},
			wantSend:  []driver.Filter{aLess, bEqual, aGreater},
			wantLocal: nil,
		},
		{
			in:        []driver.Filter{aLess, bLess, aGreater},
			wantSend:  []driver.Filter{aLess, aGreater},
			wantLocal: []driver.Filter{bLess},
		},
		{
			in:        []driver.Filter{aEqual, aLess, bLess, aGreater, bEqual},
			wantSend:  []driver.Filter{aEqual, aLess, aGreater, bEqual},
			wantLocal: []driver.Filter{bLess},
		},
	} {
		gotSend, gotLocal := splitFilters(test.in)
		if diff := cmp.Diff(gotSend, test.wantSend); diff != "" {
			t.Errorf("%v, send:\n%s", test.in, diff)
		}
		if diff := cmp.Diff(gotLocal, test.wantLocal); diff != "" {
			t.Errorf("%v, local:\n%s", test.in, diff)
		}
	}
}

func TestEvaluateFilter(t *testing.T) {
	m := map[string]interface{}{
		"i":  32,
		"f":  5.5,
		"f2": 5.0,
		"s":  "32",
		"t":  time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		"b":  true,
		"mi": int64(math.MaxInt64),
	}
	doc := drivertest.MustDocument(m)
	for _, test := range []struct {
		field, op string
		value     interface{}
		want      bool
	}{
		// Firestore compares numbers to each other ignoring type (int vs. float).
		// Just a few simple tests here; see driver.TestCompareNumbers for more.
		{"i", "=", 32, true},
		{"i", ">", 32, false},
		{"i", "<", 32, false},
		{"i", "=", 32.0, true},
		{"i", ">", 32.0, false},
		{"i", "<", 32.0, false},
		{"f", "=", 5.5, true},
		{"f", "<", 5.5, false},
		{"f2", "=", 5, true},
		{"f2", ">", 5, false},
		// Firestore compares strings to each other, but not to numbers.
		{"s", "=", "32", true},
		{"s", ">", "32", false},
		{"s", "<", "32", false},
		{"s", ">", "3", true},
		{"i", "=", "32", false},
		{"i", ">", "32", false},
		{"i", "<", "32", false},
		{"f", "=", "5.5", false},
		{"f", ">", "5.5", false},
		{"f", "<", "5.5", false},
		// Firestore compares times to each other.
		{"t", "<", time.Date(2014, 1, 1, 0, 0, 0, 0, time.UTC), true},
		// Comparisons with other types fail.
		{"b", "=", "true", false},
		{"b", ">", "true", false},
		{"b", "<", "true", false},
		{"t", "=", 0, false},
		{"t", ">", 0, false},
		{"t", "<", 0, false},
	} {
		f := driver.Filter{FieldPath: []string{test.field}, Op: test.op, Value: test.value}
		got := evaluateFilter(f, doc)
		if got != test.want {
			t.Errorf("%s %s %v: got %t, want %t", test.field, test.op, test.value, got, test.want)
		}
	}
}
