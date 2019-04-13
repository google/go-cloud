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
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/expression"
	"gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/driver"
)

func (c *collection) RunGetQuery(ctx context.Context, q *driver.Query) (driver.DocumentIterator, error) {
	out, err := c.runGetQuery(ctx, q, nil)
	if err != nil {
		return nil, err
	}
	return &documentIterator{
		c:     c,
		q:     q,
		items: out.Items,
		count: *out.Count,
		limit: q.Limit, // manually count limit since dynamodb uses "limit" as scan limit before filtering
		last:  out.LastEvaluatedKey,
	}, nil
}

func (c *collection) runGetQuery(ctx context.Context, q *driver.Query, startAfter map[string]*dynamodb.AttributeValue) (*dynamodb.QueryOutput, error) {
	pb := expression.NamesList(expression.Name(docstore.RevisionField))
	for _, fp := range q.FieldPaths {
		pb = pb.AddNames(expression.Name(strings.Join(fp, ".")))
	}
	cb := expression.NewBuilder().WithProjection(pb)
	var err error
	cb, err = processFilters(cb, q.Filters, c.partitionKey, c.sortKey)
	if err != nil {
		return nil, err
	}
	ce, err := cb.Build()
	if err != nil {
		return nil, err
	}
	return c.db.QueryWithContext(ctx, &dynamodb.QueryInput{
		TableName:                 &c.table,
		ExpressionAttributeNames:  ce.Names(),
		ExpressionAttributeValues: ce.Values(),
		KeyConditionExpression:    ce.KeyCondition(),
		FilterExpression:          ce.Filter(),
		ProjectionExpression:      ce.Projection(),
		ExclusiveStartKey:         startAfter,
	})
}

func processFilters(cb expression.Builder, fs []driver.Filter, pkey, skey string) (expression.Builder, error) {
	var kbs []expression.KeyConditionBuilder
	var cbs []expression.ConditionBuilder
	var hasP bool

	for _, f := range fs {
		switch strings.Join(f.FieldPath, ".") {
		case pkey:
			kbs = append(kbs, toKeyCondition(f, true))
			hasP = true
		case skey:
			kbs = append(kbs, toKeyCondition(f, false))
		default:
			cbs = append(cbs, toFilter(f))
		}
	}
	if !hasP {
		return expression.Builder{}, errors.New("no partition key found in query")
	}
	keyBuilder := kbs[0]
	for i := 1; i < len(kbs); i++ {
		keyBuilder.And(kbs[i])
	}
	cb = cb.WithKeyCondition(keyBuilder)
	var condBuilder expression.ConditionBuilder
	if len(cbs) > 0 {
		condBuilder = cbs[0]
		for i := 1; i < len(cbs); i++ {
			condBuilder.And(cbs[i])
		}
		cb = cb.WithFilter(condBuilder)
	}
	return cb, nil
}

func toKeyCondition(f driver.Filter, partition bool) expression.KeyConditionBuilder {
	kp := strings.Join(f.FieldPath, ".")
	key := expression.Key(kp)
	val := expression.Value(f.Value)
	if partition {
		if f.Op != driver.EqualOp {
			panic(fmt.Sprintf("invalid filter operation for the partition key: %v, only \"=\" is allowed", f.Op))
		}
		return expression.KeyEqual(key, val)
	}
	switch f.Op {
	case "<":
		return expression.KeyLessThan(key, val)
	case "<=":
		return expression.KeyLessThanEqual(key, val)
	case driver.EqualOp:
		return expression.KeyEqual(key, val)
	case ">=":
		return expression.KeyGreaterThanEqual(key, val)
	case ">":
		return expression.KeyGreaterThan(key, val)
	default:
		panic(fmt.Sprint("invalid filter operation:", f.Op))
	}
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
	count int64
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
		out, err := it.c.runGetQuery(ctx, it.q, it.last)
		if err != nil {
			return err
		}
		it.limit -= len(out.Items)
		it.items = out.Items
		it.curr = 0
		it.last = out.LastEvaluatedKey
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
