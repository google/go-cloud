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

	"gocloud.dev/internal/docstore/driver"
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
func (q *Query) Where(fieldpath, op string, value interface{}) *Query {
	fp, err := parseFieldPath(FieldPath(fieldpath))
	if err != nil {
		q.err = err
	}
	if !validOp[op] {
		q.err = gcerr.Newf(gcerr.InvalidArgument, nil, "invalid filter operator: %q. Use one of: =, >, <, >=, <=", op)
	}
	if !validFilterValue(value) {
		q.err = gcerr.Newf(gcerr.InvalidArgument, nil, "invalid filter value: %v", value)
	}
	if q.err != nil {
		return q
	}
	q.dq.Filters = append(q.dq.Filters, driver.Filter{
		FieldPath: fp,
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
func (q *Query) Limit(n int) *Query {
	q.dq.Limit = n
	return q
}

// BeforeQuery takes a callback function that will be called before the Query is
// executed to the underlying provider's query functionality. The callback takes
// a parameter, asFunc, that converts its argument to provider-specific types.
// See https://godoc.org/gocloud.dev#hdr-As for background information.
func (q *Query) BeforeQuery(f func(asFunc func(interface{}) bool) error) *Query {
	q.dq.BeforeQuery = f
	return q
}

// Get returns an iterator for retrieving the documents specified by the query. If
// field paths are provided, only those paths are set in the resulting documents.
//
// Call Stop on the iterator when finished.
func (q *Query) Get(ctx context.Context, fps ...FieldPath) *DocumentIterator {
	dcoll := q.coll.driver
	wrapErr := func(err error) error { return wrapError(dcoll, err) }
	if err := q.init(fps); err != nil {
		return &DocumentIterator{err: wrapErr(err)}
	}
	it, err := dcoll.RunGetQuery(ctx, q.dq)
	return &DocumentIterator{iter: it, wrapError: wrapErr, err: wrapErr(err)}
}

// Delete deletes all the documents specified by the query.
// It is an error if the query has a limit.
func (q *Query) Delete(ctx context.Context) error {
	if err := q.init(nil); err != nil {
		return err
	}
	if q.dq.Limit > 0 {
		return gcerr.Newf(gcerr.InvalidArgument, nil, "delete queries cannot have a limit")
	}
	return q.coll.driver.RunDeleteQuery(ctx, q.dq)
}

func (q *Query) init(fps []FieldPath) error {
	if q.err != nil {
		return q.err
	}
	if q.dq.FieldPaths == nil {
		for _, fp := range fps {
			fp, err := parseFieldPath(fp)
			if err != nil {
				q.err = err
				return err
			}
			q.dq.FieldPaths = append(q.dq.FieldPaths, fp)
		}
	}
	return nil
}

// DocumentIterator iterates over documents.
//
// Always call Stop on the iterator.
type DocumentIterator struct {
	iter      driver.DocumentIterator
	wrapError func(error) error
	err       error // already wrapped
}

// Next stores the next document in dst. It returns io.EOF if there are no more
// documents.
// Once Next returns an error, it will always return the same error.
func (it *DocumentIterator) Next(ctx context.Context, dst Document) error {
	if it.err != nil {
		return it.err
	}
	ddoc, err := driver.NewDocument(dst)
	if err != nil {
		it.err = it.wrapError(err)
		return it.err
	}
	it.err = it.wrapError(it.iter.Next(ctx, ddoc))
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

// As converts i to provider-specific types.
// See https://godoc.org/gocloud.dev#hdr-As for background information, the "As"
// examples in this package for examples, and the provider-specific package
// documentation for the specific types supported for that provider.
func (it *DocumentIterator) As(i interface{}) bool {
	if i == nil {
		return false
	}
	return it.iter.As(i)
}

// Plan describes how the query would be executed if its Get method were called with
// the given field paths. Plan uses only information available to the client, so it
// cannot know whether a service uses indexes or scans internally.
func (q *Query) Plan(fps ...FieldPath) (string, error) {
	if err := q.init(fps); err != nil {
		return "", err
	}
	return q.coll.driver.QueryPlan(q.dq)
}
