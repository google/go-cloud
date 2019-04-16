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

type avmap = map[string]*dynamodb.AttributeValue

func (c *collection) RunGetQuery(ctx context.Context, q *driver.Query) (driver.DocumentIterator, error) {
	items, last, err := c.runQueryOrScan(ctx, q, nil)
	if err != nil {
		return nil, err
	}
	return &documentIterator{
		c:     c,
		q:     q,
		items: items,
		limit: q.Limit, // manually count limit since dynamodb uses "limit" as scan limit before filtering
		last:  last,
	}, nil
}

func (c *collection) runQueryOrScan(ctx context.Context, q *driver.Query, startAfter avmap) (items []avmap, last avmap, err error) {
	var cb expression.Builder
	cbUsed := false // It's an error to build an empty Builder.
	if len(q.FieldPaths) > 0 {
		pb := expression.NamesList(expression.Name(docstore.RevisionField))
		for _, fp := range q.FieldPaths {
			pb = pb.AddNames(expression.Name(strings.Join(fp, ".")))
		}
		cb = cb.WithProjection(pb)
		cbUsed = true
	}
	// If there is an equality filter on the partition key, do a query.
	// Otherwise, do a scan.
	doQuery := false
	for _, f := range q.Filters {
		if len(f.FieldPath) == 1 && f.FieldPath[0] == c.partitionKey && f.Op == driver.EqualOp {
			doQuery = true
			break
		}
	}
	if doQuery {
		cb = processFilters(cb, q.Filters, c.partitionKey, c.sortKey)
		ce, err := cb.Build()
		if err != nil {
			return nil, nil, err
		}
		out, err := c.db.QueryWithContext(ctx, &dynamodb.QueryInput{
			TableName:                 &c.table,
			ExpressionAttributeNames:  ce.Names(),
			ExpressionAttributeValues: ce.Values(),
			KeyConditionExpression:    ce.KeyCondition(),
			FilterExpression:          ce.Filter(),
			ProjectionExpression:      ce.Projection(),
			ExclusiveStartKey:         startAfter,
		})
		if err != nil {
			return nil, nil, err
		}
		return out.Items, out.LastEvaluatedKey, nil
	} else {
		if len(q.Filters) > 0 {
			cb = cb.WithFilter(filtersToConditionBuilder(q.Filters))
			cbUsed = true
		}
		in := &dynamodb.ScanInput{
			TableName:         &c.table,
			ExclusiveStartKey: startAfter,
		}
		if cbUsed {
			ce, err := cb.Build()
			if err != nil {
				return nil, nil, err
			}
			in.ExpressionAttributeNames = ce.Names()
			in.ExpressionAttributeValues = ce.Values()
			in.FilterExpression = ce.Filter()
			in.ProjectionExpression = ce.Projection()
		}
		out, err := c.db.ScanWithContext(ctx, in)
		if err != nil {
			return nil, nil, err
		}
		return out.Items, out.LastEvaluatedKey, nil
	}
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
		keyBuilder.And(kbs[i])
	}
	cb = cb.WithKeyCondition(keyBuilder)
	if len(cfs) > 0 {
		cb = cb.WithFilter(filtersToConditionBuilder(cfs))
	}
	return cb
}

func filtersToConditionBuilder(fs []driver.Filter) expression.ConditionBuilder {
	var cb expression.ConditionBuilder
	if len(fs) == 0 {
		return cb
	}
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
	c     *collection
	q     *driver.Query
	items []map[string]*dynamodb.AttributeValue
	curr  int
	limit int
	last  map[string]*dynamodb.AttributeValue
}

func (it *documentIterator) Next(ctx context.Context, doc driver.Document) error {
	if it.limit > 0 && it.curr >= it.limit ||
		it.curr >= len(it.items) && it.last == nil {
		return io.EOF
	}
	if it.curr >= len(it.items) && it.last != nil {
		// Make a new query request at the end of this page.
		items, last, err := it.c.runQueryOrScan(ctx, it.q, it.last)
		if err != nil {
			return err
		}
		it.limit -= len(items)
		it.items = items
		it.curr = 0
		it.last = last
		return it.Next(ctx, doc)
	}
	if err := decodeDoc(&dynamodb.AttributeValue{M: it.items[it.curr]}, doc); err != nil {
		return err
	}
	it.curr++
	return nil
}

func (it *documentIterator) Stop() {
	it.curr = len(it.items)
	it.last = nil
}
