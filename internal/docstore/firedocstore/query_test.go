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
	"math"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	"gocloud.dev/internal/docstore/driver"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
)

func TestFilterToProto(t *testing.T) {
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
	} {
		got, err := filterToProto(test.in)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(got, test.want, cmp.Comparer(proto.Equal)); diff != "" {
			t.Errorf("%+v: %s", test.in, diff)
		}
	}
}
