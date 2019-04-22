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

	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/expression"
	"gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/driver"
)

// TODO: support parallel scans (http://docs.aws.amazon.com/amazondynamodb/latest/developerguide/Scan.html#Scan.ParallelScan)

// TODO: support an empty item slice returned from an RPC: "A Query operation can
// return an empty result set and a LastEvaluatedKey if all the items read for the
// page of results are filtered out."

type avmap = map[string]*dynamodb.AttributeValue

func (c *collection) RunGetQuery(ctx context.Context, q *driver.Query) (driver.DocumentIterator, error) {
	if c.description == nil {
		out, err := c.db.DescribeTable(&dynamodb.DescribeTableInput{TableName: &c.table})
		if err != nil {
			return nil, err
		}
		c.description = out.Table
	}
	qr, err := c.planQuery(q)
	if err != nil {
		return nil, err
	}
	it := &documentIterator{
		qr:    qr,
		limit: q.Limit,
		count: 0, // manually count limit since dynamodb uses "limit" as scan limit before filtering
	}
	it.items, it.last, err = it.qr.run(ctx, nil)
	if err != nil {
		return nil, err
	}
	return it, nil
}

func (c *collection) planQuery(q *driver.Query) (*queryRunner, error) {
	var cb expression.Builder
	cbUsed := false // It's an error to build an empty Builder.
	// Set up the projection expression.
	if len(q.FieldPaths) > 0 {
		var pb expression.ProjectionBuilder
		hasRevisionField := false
		for _, fp := range q.FieldPaths {
			if fpEqual(fp, docstore.RevisionField) {
				hasRevisionField = true
			}
			pb = pb.AddNames(expression.Name(strings.Join(fp, ".")))
		}
		if !hasRevisionField {
			pb = pb.AddNames(expression.Name(docstore.RevisionField))
			q.FieldPaths = append(q.FieldPaths, []string{docstore.RevisionField})
		}
		cb = cb.WithProjection(pb)
		cbUsed = true
	}

	// Find the best thing to query (table or index).
	indexName, pkey, skey := c.bestQueryable(q)
	if indexName == nil && pkey == "" {
		// No query can be done: fall back to scanning.
		if len(q.Filters) > 0 {
			cb = cb.WithFilter(filtersToConditionBuilder(q.Filters))
			cbUsed = true
		}
		in := &dynamodb.ScanInput{TableName: &c.table}
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
		return &queryRunner{c: c, scanIn: in}, nil
	}

	// Do a query.
	cb = processFilters(cb, q.Filters, pkey, skey)
	ce, err := cb.Build()
	if err != nil {
		return nil, err
	}
	return &queryRunner{
		c: c,
		queryIn: &dynamodb.QueryInput{
			TableName:                 &c.table,
			IndexName:                 indexName,
			ExpressionAttributeNames:  ce.Names(),
			ExpressionAttributeValues: ce.Values(),
			KeyConditionExpression:    ce.KeyCondition(),
			FilterExpression:          ce.Filter(),
			ProjectionExpression:      ce.Projection(),
		},
	}, nil
}

// Return the best choice of queryable (table or index) for this query.
// If indexName is nil but pkey is not empty, then use the table.
// If all return values are zero, no query will work: do a scan.
func (c *collection) bestQueryable(q *driver.Query) (indexName *string, pkey, skey string) {
	// If the query has an "=" filter on the table's partition key, look at the table
	// and local indexes.
	if hasEqualityFilter(q, c.partitionKey) {
		// If the table has a sort key that's in the query, use the table.
		if hasFilter(q, c.sortKey) {
			return nil, c.partitionKey, c.sortKey
		}
		// Look at local indexes. They all have the same partition key as the base table.
		// If one has a sort key in the query, use it.
		for _, li := range c.description.LocalSecondaryIndexes {
			pkey, skey := keyAttributes(li.KeySchema)
			if hasFilter(q, skey) {
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
		if hasEqualityFilter(q, pkey) && hasFilter(q, skey) && c.fieldsIncluded(q, gi) {
			return gi.IndexName, pkey, skey
		}
	}
	// There are no matches for both partition and sort key. Now consider matches on partition key only.
	// That will still be better than a scan.
	// First, check the table itself.
	if hasEqualityFilter(q, c.partitionKey) {
		return nil, c.partitionKey, c.sortKey
	}
	// No point checking local indexes: they have the same partition key as the table.
	// Check the global indexes.
	for _, gi := range c.description.GlobalSecondaryIndexes {
		pkey, skey := keyAttributes(gi.KeySchema)
		if hasEqualityFilter(q, pkey) && c.fieldsIncluded(q, gi) {
			return gi.IndexName, pkey, skey
		}
	}
	// We cannot do a query.
	// TODO: return the reason why we couldn't. At a minimum, distinguish failure due to keys
	// from failure due to projection (i.e. a global index had the right partition and sort key,
	// but didn't project the necessary fields).
	return nil, "", ""
}

// Reports whether the fields selected by the query are projected into (that is,
// contained directly in) the global index. We need this check before using the
// index, because if a global index doesn't have all the desired fields, then a
// separate RPC for each returned item would be necessary to retrieve those fields,
// and we'd rather scan than do that.
func (c *collection) fieldsIncluded(q *driver.Query, gi *dynamodb.GlobalSecondaryIndexDescription) bool {
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
func keyAttributes(ks []*dynamodb.KeySchemaElement) (pkey, skey string) {
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
		if fpEqual(f.FieldPath, field) {
			return true
		}
	}
	return false
}

// Reports whether q has a filter that checks if the top-level field is equal to something.
func hasEqualityFilter(q *driver.Query, field string) bool {
	for _, f := range q.Filters {
		if f.Op == driver.EqualOp && fpEqual(f.FieldPath, field) {
			return true
		}
	}
	return false
}

func fpEqual(fp []string, s string) bool {
	return len(fp) == 1 && fp[0] == s
}

type queryRunner struct {
	c       *collection
	scanIn  *dynamodb.ScanInput
	queryIn *dynamodb.QueryInput
}

func (qr *queryRunner) run(ctx context.Context, startAfter avmap) (items []avmap, last avmap, err error) {
	if qr.scanIn != nil {
		qr.scanIn.ExclusiveStartKey = startAfter
		out, err := qr.c.db.ScanWithContext(ctx, qr.scanIn)
		if err != nil {
			return nil, nil, err
		}
		return out.Items, out.LastEvaluatedKey, nil
	}
	qr.queryIn.ExclusiveStartKey = startAfter
	out, err := qr.c.db.QueryWithContext(ctx, qr.queryIn)
	if err != nil {
		return nil, nil, err
	}
	return out.Items, out.LastEvaluatedKey, nil
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
	qr    *queryRunner
	items []map[string]*dynamodb.AttributeValue
	curr  int
	limit int
	count int // number of items returned
	last  map[string]*dynamodb.AttributeValue
}

func (it *documentIterator) Next(ctx context.Context, doc driver.Document) error {
	if it.limit > 0 && it.count >= it.limit || it.curr >= len(it.items) && it.last == nil {
		return io.EOF
	}
	if it.curr >= len(it.items) {
		// Make a new query request at the end of this page.
		var err error
		it.items, it.last, err = it.qr.run(ctx, it.last)
		if err != nil {
			return err
		}
		it.curr = 0
	}
	if err := decodeDoc(&dynamodb.AttributeValue{M: it.items[it.curr]}, doc); err != nil {
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

func (c *collection) QueryPlan(q *driver.Query) (*driver.QueryPlan, error) {
	if c.description == nil {
		out, err := c.db.DescribeTable(&dynamodb.DescribeTableInput{TableName: &c.table})
		if err != nil {
			return nil, err
		}
		c.description = out.Table
	}
	qr, err := c.planQuery(q)
	if err != nil {
		return nil, err
	}
	return qr.queryPlan(), nil
}

func (qr *queryRunner) queryPlan() *driver.QueryPlan {
	qp := &driver.QueryPlan{Scan: qr.scanIn != nil}
	if !qp.Scan && qr.queryIn.IndexName != nil {
		qp.Index = *qr.queryIn.IndexName
	}
	return qp
}
