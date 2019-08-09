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

package docstore

import (
	"context"
	"io"
	"reflect"
	"time"

	"gocloud.dev/docstore/driver"
	"gocloud.dev/internal/gcerr"
)

// Query represents a query over a collection.
type Query struct {
	coll *Collection
	dq   *driver.Query
	err  error
}

// Query creates a new Query over the collection.
func (c *Collection) Query() *Query {
	return &Query{coll: c, dq: &driver.Query{}}
}

// Where expresses a condition on the query.
// Valid ops are: "=", ">", "<", ">=", "<=".
// Valid values are strings, integers, floating-point numbers, and time.Time values.
func (q *Query) Where(fp FieldPath, op string, value interface{}) *Query {
	if q.err != nil {
		return q
	}
	pfp, err := parseFieldPath(fp)
	if err != nil {
		q.err = err
		return q
	}
	if !validOp[op] {
		return q.invalidf("invalid filter operator: %q. Use one of: =, >, <, >=, <=", op)
	}
	if !validFilterValue(value) {
		return q.invalidf("invalid filter value: %v", value)
	}
	q.dq.Filters = append(q.dq.Filters, driver.Filter{
		FieldPath: pfp,
		Op:        op,
		Value:     value,
	})
	return q
}

var validOp = map[string]bool{
	"=":  true,
	">":  true,
	"<":  true,
	">=": true,
	"<=": true,
}

func validFilterValue(v interface{}) bool {
	if v == nil {
		return false
	}
	if _, ok := v.(time.Time); ok {
		return true
	}
	switch reflect.TypeOf(v).Kind() {
	case reflect.String:
		return true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	case reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

// Limit will limit the results to at most n documents.
// n must be positive.
// It is an error to specify Limit more than once in a Get query, or
// at all in a Delete or Update query.
func (q *Query) Limit(n int) *Query {
	if q.err != nil {
		return q
	}
	if n <= 0 {
		return q.invalidf("limit value of %d must be greater than zero", n)
	}
	if q.dq.Limit > 0 {
		return q.invalidf("query can have at most one limit clause")
	}
	q.dq.Limit = n
	return q
}

// Ascending and Descending are constants for use in the OrderBy method.
const (
	Ascending  = "asc"
	Descending = "desc"
)

// OrderBy specifies that the returned documents appear sorted by the given field in
// the given direction.
// A query can have at most one OrderBy clause. If it has none, the order of returned
// documents is unspecified.
// If a query has a Where clause and an OrderBy clause, the OrderBy clause's field
// must appear in a Where clause.
// It is an error to specify OrderBy in a Delete or Update query.
func (q *Query) OrderBy(field, direction string) *Query {
	if q.err != nil {
		return q
	}
	if field == "" {
		return q.invalidf("OrderBy: empty field")
	}
	if direction != Ascending && direction != Descending {
		return q.invalidf("OrderBy: direction must be one of %q or %q", Ascending, Descending)
	}
	if q.dq.OrderByField != "" {
		return q.invalidf("a query can have at most one OrderBy")
	}
	q.dq.OrderByField = field
	q.dq.OrderAscending = (direction == Ascending)
	return q
}

// BeforeQuery takes a callback function that will be called before the Query is
// executed to the underlying service's query functionality. The callback takes
// a parameter, asFunc, that converts its argument to driver-specific types.
// See https://gocloud.dev/concepts/as/ for background information.
func (q *Query) BeforeQuery(f func(asFunc func(interface{}) bool) error) *Query {
	q.dq.BeforeQuery = f
	return q
}

// Get returns an iterator for retrieving the documents specified by the query. If
// field paths are provided, only those paths are set in the resulting documents.
//
// Call Stop on the iterator when finished.
func (q *Query) Get(ctx context.Context, fps ...FieldPath) *DocumentIterator {
	return q.get(ctx, true, fps...)
}

// get implements Get, with optional OpenCensus tracing so it can be used internally.
func (q *Query) get(ctx context.Context, oc bool, fps ...FieldPath) *DocumentIterator {
	dcoll := q.coll.driver
	if err := q.initGet(fps); err != nil {
		return &DocumentIterator{err: wrapError(dcoll, err)}
	}

	var err error
	if oc {
		ctx = q.coll.tracer.Start(ctx, "Query.Get")
		defer func() { q.coll.tracer.End(ctx, err) }()
	}
	it, err := dcoll.RunGetQuery(ctx, q.dq)
	return &DocumentIterator{iter: it, coll: q.coll, err: wrapError(dcoll, err)}
}

func (q *Query) initGet(fps []FieldPath) error {
	if q.err != nil {
		return q.err
	}
	if err := q.coll.checkClosed(); err != nil {
		return errClosed
	}
	pfps, err := parseFieldPaths(fps)
	if err != nil {
		return err
	}
	q.dq.FieldPaths = pfps
	if q.dq.OrderByField != "" && len(q.dq.Filters) > 0 {
		found := false
		for _, f := range q.dq.Filters {
			if len(f.FieldPath) == 1 && f.FieldPath[0] == q.dq.OrderByField {
				found = true
				break
			}
		}
		if !found {
			return gcerr.Newf(gcerr.InvalidArgument, nil, "OrderBy field %s must appear in a Where clause",
				q.dq.OrderByField)
		}
	}
	return nil
}

func (q *Query) invalidf(format string, args ...interface{}) *Query {
	q.err = gcerr.Newf(gcerr.InvalidArgument, nil, format, args...)
	return q
}

// DocumentIterator iterates over documents.
//
// Always call Stop on the iterator.
type DocumentIterator struct {
	iter driver.DocumentIterator
	coll *Collection
	err  error // already wrapped
}

// Next stores the next document in dst. It returns io.EOF if there are no more
// documents.
// Once Next returns an error, it will always return the same error.
func (it *DocumentIterator) Next(ctx context.Context, dst Document) error {
	if it.err != nil {
		return it.err
	}
	if err := it.coll.checkClosed(); err != nil {
		it.err = err
		return it.err
	}
	ddoc, err := driver.NewDocument(dst)
	if err != nil {
		it.err = wrapError(it.coll.driver, err)
		return it.err
	}
	it.err = wrapError(it.coll.driver, it.iter.Next(ctx, ddoc))
	return it.err
}

// Stop stops the iterator. Calling Next on a stopped iterator will return io.EOF, or
// the error that Next previously returned.
func (it *DocumentIterator) Stop() {
	if it.err != nil {
		return
	}
	it.err = io.EOF
	it.iter.Stop()
}

// As converts i to driver-specific types.
// See https://gocloud.dev/concepts/as/ for background information, the "As"
// examples in this package for examples, and the driver package
// documentation for the specific types supported for that driver.
func (it *DocumentIterator) As(i interface{}) bool {
	if i == nil || it.iter == nil {
		return false
	}
	return it.iter.As(i)
}

// Plan describes how the query would be executed if its Get method were called with
// the given field paths. Plan uses only information available to the client, so it
// cannot know whether a service uses indexes or scans internally.
func (q *Query) Plan(fps ...FieldPath) (string, error) {
	if err := q.initGet(fps); err != nil {
		return "", err
	}
	return q.coll.driver.QueryPlan(q.dq)
}
