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

package dynamodocstore

import (
	"context"
	"fmt"
	"io"
	"strings"

	dyn "github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/expression"
	"gocloud.dev/docstore"
	"gocloud.dev/docstore/driver"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/gcerr"
)

// TODO: support parallel scans (http://docs.aws.amazon.com/amazondynamodb/latest/developerguide/Scan.html#Scan.ParallelScan)

// TODO(jba): support an empty item slice returned from an RPC: "A Query operation can
// return an empty result set and a LastEvaluatedKey if all the items read for the
// page of results are filtered out."

type avmap = map[string]*dyn.AttributeValue

func (c *collection) RunGetQuery(ctx context.Context, q *driver.Query) (driver.DocumentIterator, error) {
	qr, err := c.planQuery(q)
	if err != nil {
		if gcerrors.Code(err) == gcerrors.Unimplemented && c.opts.RunQueryFallback != nil {
			return c.opts.RunQueryFallback(ctx, q, c.RunGetQuery)
		}
		return nil, err
	}
	if err := c.checkPlan(qr); err != nil {
		return nil, err
	}
	it := &documentIterator{
		qr:    qr,
		limit: q.Limit,
		count: 0, // manually count limit since dynamodb uses "limit" as scan limit before filtering
	}
	it.items, it.last, it.asFunc, err = it.qr.run(ctx, nil)
	if err != nil {
		return nil, err
	}
	return it, nil
}

func (c *collection) checkPlan(qr *queryRunner) error {
	if qr.scanIn != nil && qr.scanIn.FilterExpression != nil && !c.opts.AllowScans {
		return gcerr.Newf(gcerr.InvalidArgument, nil, "query requires a table scan; set Options.AllowScans to true to enable")
	}
	return nil
}

func (c *collection) planQuery(q *driver.Query) (*queryRunner, error) {
	var cb expression.Builder
	cbUsed := false // It's an error to build an empty Builder.
	// Set up the projection expression.
	if len(q.FieldPaths) > 0 {
		var pb expression.ProjectionBuilder
		hasFields := map[string]bool{}
		for _, fp := range q.FieldPaths {
			if len(fp) == 1 {
				hasFields[fp[0]] = true
			}
			pb = pb.AddNames(expression.Name(strings.Join(fp, ".")))
		}
		// Always include the key and revision fields.
		for _, f := range []string{c.partitionKey, c.sortKey, c.opts.RevisionField} {
			if f != "" && !hasFields[f] {
				pb = pb.AddNames(expression.Name(f))
				q.FieldPaths = append(q.FieldPaths, []string{f})
			}
		}
		cb = cb.WithProjection(pb)
		cbUsed = true
	}

	// Find the best thing to query (table or index).
	indexName, pkey, skey := c.bestQueryable(q)
	if indexName == nil && pkey == "" {
		// No query can be done: fall back to scanning.
		if q.OrderByField != "" {
			// Scans are unordered, so we can't run this query.
			// TODO(jba): If the user specifies all the partition keys, and there is a global
			// secondary index whose sort key is the order-by field, then we can query that index
			// for every value of the partition key and merge the results.
			// TODO(jba): If the query has a reasonable limit N, then we can run a scan and keep
			// the top N documents in memory.
			return nil, gcerr.Newf(gcerr.Unimplemented, nil, "query requires a table scan, but has an ordering requirement; add an index or provide Options.RunQueryFallback")
		}
		if len(q.Filters) > 0 {
			cb = cb.WithFilter(filtersToConditionBuilder(q.Filters))
			cbUsed = true
		}
		in := &dyn.ScanInput{TableName: &c.table}
		if cbUsed {
			ce, err := cb.Build()
			if err != nil {
				return nil, err
			}
			in.ExpressionAttributeNames = ce.Names()
			in.ExpressionAttributeValues = ce.Values()
			in.FilterExpression = ce.Filter()
			in.ProjectionExpression = ce.Projection()
		}
		return &queryRunner{c: c, scanIn: in, beforeRun: q.BeforeQuery}, nil
	}

	// Do a query.
	cb = processFilters(cb, q.Filters, pkey, skey)
	ce, err := cb.Build()
	if err != nil {
		return nil, err
	}
	qIn := &dyn.QueryInput{
		TableName:                 &c.table,
		IndexName:                 indexName,
		ExpressionAttributeNames:  ce.Names(),
		ExpressionAttributeValues: ce.Values(),
		KeyConditionExpression:    ce.KeyCondition(),
		FilterExpression:          ce.Filter(),
		ProjectionExpression:      ce.Projection(),
	}
	if q.OrderByField != "" && !q.OrderAscending {
		qIn.ScanIndexForward = &q.OrderAscending
	}
	return &queryRunner{
		c:         c,
		queryIn:   qIn,
		beforeRun: q.BeforeQuery,
	}, nil
}

// Return the best choice of queryable (table or index) for this query.
// How to interpret the return values:
// - If indexName is nil but pkey is not empty, then use the table.
// - If all return values are zero, no query will work: do a scan.
func (c *collection) bestQueryable(q *driver.Query) (indexName *string, pkey, skey string) {
	// If the query has an "=" filter on the table's partition key, look at the table
	// and local indexes.
	if hasEqualityFilter(q, c.partitionKey) {
		// If the table has a sort key that's in the query, and the ordering
		// constraint works with the sort key, use the table.
		// (Query results are always ordered by sort key.)
		if hasFilter(q, c.sortKey) && orderingConsistent(q, c.sortKey) {
			return nil, c.partitionKey, c.sortKey
		}
		// Look at local indexes. They all have the same partition key as the base table.
		// If one has a sort key in the query, use it.
		for _, li := range c.description.LocalSecondaryIndexes {
			pkey, skey := keyAttributes(li.KeySchema)
			if hasFilter(q, skey) && localFieldsIncluded(q, li) && orderingConsistent(q, skey) {
				return li.IndexName, pkey, skey
			}
		}
	}
	// Consider the global indexes: if one has a matching partition and sort key, and
	// the projected fields of the index include those of the query, use it.
	for _, gi := range c.description.GlobalSecondaryIndexes {
		pkey, skey := keyAttributes(gi.KeySchema)
		if skey == "" {
			continue // We'll visit global indexes without a sort key later.
		}
		if hasEqualityFilter(q, pkey) && hasFilter(q, skey) && c.globalFieldsIncluded(q, gi) && orderingConsistent(q, skey) {
			return gi.IndexName, pkey, skey
		}
	}
	// There are no matches for both partition and sort key. Now consider matches on partition key only.
	// That will still be better than a scan.
	// First, check the table itself.
	if hasEqualityFilter(q, c.partitionKey) && orderingConsistent(q, c.sortKey) {
		return nil, c.partitionKey, c.sortKey
	}
	// No point checking local indexes: they have the same partition key as the table.
	// Check the global indexes.
	for _, gi := range c.description.GlobalSecondaryIndexes {
		pkey, skey := keyAttributes(gi.KeySchema)
		if hasEqualityFilter(q, pkey) && c.globalFieldsIncluded(q, gi) && orderingConsistent(q, skey) {
			return gi.IndexName, pkey, skey
		}
	}
	// We cannot do a query.
	// TODO: return the reason why we couldn't. At a minimum, distinguish failure due to keys
	// from failure due to projection (i.e. a global index had the right partition and sort key,
	// but didn't project the necessary fields).
	return nil, "", ""
}

// localFieldsIncluded reports whether a local index supports all the selected fields
// of a query. Since DynamoDB will read explicitly provided fields from the table if
// they are not projected into the index, the only case where a local index cannot
// be used is when the query wants all the fields, and the index projection is not ALL.
// See https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/LSI.html#LSI.Projections.
func localFieldsIncluded(q *driver.Query, li *dyn.LocalSecondaryIndexDescription) bool {
	return len(q.FieldPaths) > 0 || *li.Projection.ProjectionType == "ALL"
}

// orderingConsistent reports whether the ordering constraint is consistent with the sort key field.
// That is, either there is no OrderBy clause, or the clause specifies the sort field.
func orderingConsistent(q *driver.Query, sortField string) bool {
	return q.OrderByField == "" || q.OrderByField == sortField
}

// globalFieldsIncluded reports whether the fields selected by the query are
// projected into (that is, contained directly in) the global index. We need this
// check before using the index, because if a global index doesn't have all the
// desired fields, then a separate RPC for each returned item would be necessary to
// retrieve those fields, and we'd rather scan than do that.
func (c *collection) globalFieldsIncluded(q *driver.Query, gi *dyn.GlobalSecondaryIndexDescription) bool {
	proj := gi.Projection
	if *proj.ProjectionType == "ALL" {
		// The index has all the fields of the table: we're good.
		return true
	}
	if len(q.FieldPaths) == 0 {
		// The query wants all the fields of the table, but we can't be sure that the
		// index has them.
		return false
	}
	// The table's keys and the index's keys are always in the index.
	pkey, skey := keyAttributes(gi.KeySchema)
	indexFields := map[string]bool{c.partitionKey: true, pkey: true}
	if c.sortKey != "" {
		indexFields[c.sortKey] = true
	}
	if skey != "" {
		indexFields[skey] = true
	}
	for _, nka := range proj.NonKeyAttributes {
		indexFields[*nka] = true
	}
	// Every field path in the query must be in the index.
	for _, fp := range q.FieldPaths {
		if !indexFields[strings.Join(fp, ".")] {
			return false
		}
	}
	return true
}

// Extract the names of the partition and sort key attributes from the schema of a
// table or index.
func keyAttributes(ks []*dyn.KeySchemaElement) (pkey, skey string) {
	for _, k := range ks {
		switch *k.KeyType {
		case "HASH":
			pkey = *k.AttributeName
		case "RANGE":
			skey = *k.AttributeName
		default:
			panic("bad key type: " + *k.KeyType)
		}
	}
	return pkey, skey
}

// Reports whether q has a filter that mentions the top-level field.
func hasFilter(q *driver.Query, field string) bool {
	if field == "" {
		return false
	}
	for _, f := range q.Filters {
		if driver.FieldPathEqualsField(f.FieldPath, field) {
			return true
		}
	}
	return false
}

// Reports whether q has a filter that checks if the top-level field is equal to something.
func hasEqualityFilter(q *driver.Query, field string) bool {
	for _, f := range q.Filters {
		if f.Op == driver.EqualOp && driver.FieldPathEqualsField(f.FieldPath, field) {
			return true
		}
	}
	return false
}

type queryRunner struct {
	c         *collection
	scanIn    *dyn.ScanInput
	queryIn   *dyn.QueryInput
	beforeRun func(asFunc func(i interface{}) bool) error
}

func (qr *queryRunner) run(ctx context.Context, startAfter avmap) (items []avmap, last avmap, asFunc func(i interface{}) bool, err error) {
	if qr.scanIn != nil {
		qr.scanIn.ExclusiveStartKey = startAfter
		if qr.beforeRun != nil {
			asFunc := func(i interface{}) bool {
				p, ok := i.(**dyn.ScanInput)
				if !ok {
					return false
				}
				*p = qr.scanIn
				return true
			}
			if err := qr.beforeRun(asFunc); err != nil {
				return nil, nil, nil, err
			}
		}
		out, err := qr.c.db.ScanWithContext(ctx, qr.scanIn)
		if err != nil {
			return nil, nil, nil, err
		}
		return out.Items, out.LastEvaluatedKey,
			func(i interface{}) bool {
				p, ok := i.(**dyn.ScanOutput)
				if !ok {
					return false
				}
				*p = out
				return true
			}, nil
	}
	qr.queryIn.ExclusiveStartKey = startAfter
	if qr.beforeRun != nil {
		asFunc := func(i interface{}) bool {
			p, ok := i.(**dyn.QueryInput)
			if !ok {
				return false
			}
			*p = qr.queryIn
			return true
		}
		if err := qr.beforeRun(asFunc); err != nil {
			return nil, nil, nil, err
		}
	}
	out, err := qr.c.db.QueryWithContext(ctx, qr.queryIn)
	if err != nil {
		return nil, nil, nil, err
	}
	return out.Items, out.LastEvaluatedKey,
		func(i interface{}) bool {
			p, ok := i.(**dyn.QueryOutput)
			if !ok {
				return false
			}
			*p = out
			return true
		}, nil
}

func processFilters(cb expression.Builder, fs []driver.Filter, pkey, skey string) expression.Builder {
	var kbs []expression.KeyConditionBuilder
	var cfs []driver.Filter
	for _, f := range fs {
		if kb, ok := toKeyCondition(f, pkey, skey); ok {
			kbs = append(kbs, kb)
			continue
		}
		cfs = append(cfs, f)
	}
	keyBuilder := kbs[0]
	for i := 1; i < len(kbs); i++ {
		keyBuilder = keyBuilder.And(kbs[i])
	}
	cb = cb.WithKeyCondition(keyBuilder)
	if len(cfs) > 0 {
		cb = cb.WithFilter(filtersToConditionBuilder(cfs))
	}
	return cb
}

func filtersToConditionBuilder(fs []driver.Filter) expression.ConditionBuilder {
	if len(fs) == 0 {
		panic("no filters")
	}
	var cb expression.ConditionBuilder
	cb = toFilter(fs[0])
	for _, f := range fs[1:] {
		cb = cb.And(toFilter(f))
	}
	return cb
}

func toKeyCondition(f driver.Filter, pkey, skey string) (expression.KeyConditionBuilder, bool) {
	kp := strings.Join(f.FieldPath, ".")
	if kp == pkey || kp == skey {
		key := expression.Key(kp)
		val := expression.Value(f.Value)
		switch f.Op {
		case "<":
			return expression.KeyLessThan(key, val), true
		case "<=":
			return expression.KeyLessThanEqual(key, val), true
		case driver.EqualOp:
			return expression.KeyEqual(key, val), true
		case ">=":
			return expression.KeyGreaterThanEqual(key, val), true
		case ">":
			return expression.KeyGreaterThan(key, val), true
		default:
			panic(fmt.Sprint("invalid filter operation:", f.Op))
		}
	}
	return expression.KeyConditionBuilder{}, false
}

func toFilter(f driver.Filter) expression.ConditionBuilder {
	name := expression.Name(strings.Join(f.FieldPath, "."))
	val := expression.Value(f.Value)
	switch f.Op {
	case "<":
		return expression.LessThan(name, val)
	case "<=":
		return expression.LessThanEqual(name, val)
	case driver.EqualOp:
		return expression.Equal(name, val)
	case ">=":
		return expression.GreaterThanEqual(name, val)
	case ">":
		return expression.GreaterThan(name, val)
	default:
		panic(fmt.Sprint("invalid filter operation:", f.Op))
	}
}

type documentIterator struct {
	qr     *queryRunner
	items  []map[string]*dyn.AttributeValue
	curr   int
	limit  int
	count  int // number of items returned
	last   map[string]*dyn.AttributeValue
	asFunc func(i interface{}) bool
}

func (it *documentIterator) Next(ctx context.Context, doc driver.Document) error {
	if it.limit > 0 && it.count >= it.limit || it.curr >= len(it.items) && it.last == nil {
		return io.EOF
	}
	if it.curr >= len(it.items) {
		// Make a new query request at the end of this page.
		var err error
		it.items, it.last, it.asFunc, err = it.qr.run(ctx, it.last)
		if err != nil {
			return err
		}
		it.curr = 0
	}
	if err := decodeDoc(&dyn.AttributeValue{M: it.items[it.curr]}, doc); err != nil {
		return err
	}
	it.curr++
	it.count++
	return nil
}

func (it *documentIterator) Stop() {
	it.items = nil
	it.last = nil
}

func (it *documentIterator) As(i interface{}) bool {
	return it.asFunc(i)
}

func (c *collection) QueryPlan(q *driver.Query) (string, error) {
	qr, err := c.planQuery(q)
	if err != nil {
		return "", err
	}
	return qr.queryPlan(), nil
}

func (qr *queryRunner) queryPlan() string {
	if qr.scanIn != nil {
		return "Scan"
	}
	if qr.queryIn.IndexName != nil {
		return fmt.Sprintf("Index: %q", *qr.queryIn.IndexName)
	}
	return "Table"
}

func (c *collection) RunDeleteQuery(ctx context.Context, q *driver.Query) error {
	return c.runActionQuery(ctx, q, nil)
}

func (c *collection) RunUpdateQuery(ctx context.Context, q *driver.Query, mods []driver.Mod) error {
	return c.runActionQuery(ctx, q, mods)
}

func (c *collection) runActionQuery(ctx context.Context, q *driver.Query, mods []driver.Mod) error {
	q.FieldPaths = [][]string{{c.partitionKey}}
	if c.sortKey != "" {
		q.FieldPaths = append(q.FieldPaths, []string{c.sortKey})
	}
	qr, err := c.planQuery(q)
	if err != nil {
		return err
	}

	var actions []*driver.Action
	var startAfter map[string]*dyn.AttributeValue
	for {
		items, last, _, err := qr.run(ctx, startAfter)
		if err != nil {
			return err
		}
		for _, item := range items {
			doc, err := driver.NewDocument(map[string]interface{}{})
			if err != nil {
				return err
			}
			if err := decodeDoc(&dyn.AttributeValue{M: item}, doc); err != nil {
				return err
			}
			key, err := c.Key(doc)
			if err != nil {
				return err
			}
			a := &driver.Action{Doc: doc, Key: key, Index: len(actions), Mods: mods}
			if mods == nil {
				a.Kind = driver.Delete
			} else {
				a.Kind = driver.Update
			}
			actions = append(actions, a)
		}
		if last == nil {
			break
		}
		startAfter = last
	}
	alerr := c.RunActions(ctx, actions, &driver.RunActionsOptions{})
	if len(alerr) == 0 {
		return nil
	}
	return docstore.ActionListError(alerr)
}
