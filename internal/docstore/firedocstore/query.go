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
	"fmt"
	"math"
	"path"

	"github.com/golang/protobuf/ptypes/wrappers"
	"gocloud.dev/internal/docstore/driver"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
)

func (c *collection) RunGetQuery(ctx context.Context, q *driver.Query) (driver.DocumentIterator, error) {
	return c.newDocIterator(ctx, q)
}

func (c *collection) newDocIterator(ctx context.Context, q *driver.Query) (*docIterator, error) {
	sq, err := c.queryToProto(q)
	if err != nil {
		return nil, err
	}
	req := &pb.RunQueryRequest{
		Parent:    path.Dir(c.collPath),
		QueryType: &pb.RunQueryRequest_StructuredQuery{sq},
	}
	ctx, cancel := context.WithCancel(ctx)
	sc, err := c.client.RunQuery(ctx, req)
	if err != nil {
		return nil, err
	}
	return &docIterator{streamClient: sc, nameField: c.nameField, cancel: cancel}, nil
}

////////////////////////////////////////////////////////////////
// The code below is adapted from cloud.google.com/go/firestore.

type docIterator struct {
	streamClient pb.Firestore_RunQueryClient
	nameField    string
	// We call cancel to make sure the stream client doesn't leak resources.
	// We don't need to call it if Recv() returns a non-nil error.
	// See https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
	cancel func()
}

func (it *docIterator) Next(ctx context.Context, doc driver.Document) error {
	var res *pb.RunQueryResponse
	var err error
	for {
		res, err = it.streamClient.Recv()
		if err != nil {
			return err
		}
		if res.Document != nil {
			break
		}

		// No document => partial progress; keep receiving.
	}
	return decodeDoc(res.Document, doc, it.nameField)
}

func (it *docIterator) Stop() { it.cancel() }

func (c *collection) queryToProto(q *driver.Query) (*pb.StructuredQuery, error) {
	// The collection ID is the last component of the collection path.
	collID := path.Base(c.collPath)
	p := &pb.StructuredQuery{
		From: []*pb.StructuredQuery_CollectionSelector{{CollectionId: collID}},
	}
	if len(q.FieldPaths) > 0 {
		p.Select = &pb.StructuredQuery_Projection{}
		for _, fp := range q.FieldPaths {
			p.Select.Fields = append(p.Select.Fields, fieldRef(fp))
		}
	}
	if q.Limit > 0 {
		p.Limit = &wrappers.Int32Value{Value: int32(q.Limit)}
	}
	// If there is only one filter, use it directly. Otherwise, construct
	// a CompositeFilter.
	var pfs []*pb.StructuredQuery_Filter
	for _, f := range q.Filters {
		pf, err := filterToProto(f)
		if err != nil {
			return nil, err
		}
		pfs = append(pfs, pf)
	}
	if len(pfs) == 1 {
		p.Where = pfs[0]
	} else if len(pfs) > 1 {
		p.Where = &pb.StructuredQuery_Filter{
			FilterType: &pb.StructuredQuery_Filter_CompositeFilter{&pb.StructuredQuery_CompositeFilter{
				Op:      pb.StructuredQuery_CompositeFilter_AND,
				Filters: pfs,
			}},
		}
	}
	// TODO(jba): order
	// TODO(jba): cursors (start/end)
	return p, nil
}

func filterToProto(f driver.Filter) (*pb.StructuredQuery_Filter, error) {
	// "= nil" and "= NaN" are handled specially.
	if uop, ok := unaryOpFor(f.Value); ok {
		if f.Op != driver.EqualOp {
			return nil, fmt.Errorf("firestore: must use '=' when comparing %v", f.Value)
		}
		return &pb.StructuredQuery_Filter{
			FilterType: &pb.StructuredQuery_Filter_UnaryFilter{
				UnaryFilter: &pb.StructuredQuery_UnaryFilter{
					OperandType: &pb.StructuredQuery_UnaryFilter_Field{
						Field: fieldRef(f.FieldPath),
					},
					Op: uop,
				},
			},
		}, nil
	}
	var op pb.StructuredQuery_FieldFilter_Operator
	switch f.Op {
	case "<":
		op = pb.StructuredQuery_FieldFilter_LESS_THAN
	case "<=":
		op = pb.StructuredQuery_FieldFilter_LESS_THAN_OR_EQUAL
	case ">":
		op = pb.StructuredQuery_FieldFilter_GREATER_THAN
	case ">=":
		op = pb.StructuredQuery_FieldFilter_GREATER_THAN_OR_EQUAL
	case driver.EqualOp:
		op = pb.StructuredQuery_FieldFilter_EQUAL
	// TODO(jba): can we support array-contains portably?
	// case "array-contains":
	// 	op = pb.StructuredQuery_FieldFilter_ARRAY_CONTAINS
	default:
		return nil, fmt.Errorf("invalid operator %q", f.Op)
	}
	pv, err := encodeValue(f.Value)
	if err != nil {
		return nil, err
	}
	return &pb.StructuredQuery_Filter{
		FilterType: &pb.StructuredQuery_Filter_FieldFilter{
			FieldFilter: &pb.StructuredQuery_FieldFilter{
				Field: fieldRef(f.FieldPath),
				Op:    op,
				Value: pv,
			},
		},
	}, nil
}

func unaryOpFor(value interface{}) (pb.StructuredQuery_UnaryFilter_Operator, bool) {
	switch {
	case value == nil:
		return pb.StructuredQuery_UnaryFilter_IS_NULL, true
	case isNaN(value):
		return pb.StructuredQuery_UnaryFilter_IS_NAN, true
	default:
		return pb.StructuredQuery_UnaryFilter_OPERATOR_UNSPECIFIED, false
	}
}

func isNaN(x interface{}) bool {
	switch x := x.(type) {
	case float32:
		return math.IsNaN(float64(x))
	case float64:
		return math.IsNaN(x)
	default:
		return false
	}
}

func fieldRef(fp []string) *pb.StructuredQuery_FieldReference {
	return &pb.StructuredQuery_FieldReference{FieldPath: toServiceFieldPath(fp)}
}
