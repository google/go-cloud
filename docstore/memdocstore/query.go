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

package memdocstore

import (
	"context"
	"io"
	"reflect"
	"sort"
	"strings"
	"time"

	"gocloud.dev/docstore/driver"
)

func (c *collection) RunGetQuery(_ context.Context, q *driver.Query) (driver.DocumentIterator, error) {
	if q.BeforeQuery != nil {
		if err := q.BeforeQuery(func(interface{}) bool { return false }); err != nil {
			return nil, err
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	var resultDocs []storedDoc
	for _, doc := range c.docs {
		if filtersMatch(q.Filters, doc) {
			resultDocs = append(resultDocs, doc)
		}
	}
	if q.OrderByField != "" {
		sortDocs(resultDocs, q.OrderByField, q.OrderAscending)
	}

	if q.Limit > 0 && len(resultDocs) > q.Limit {
		resultDocs = resultDocs[:q.Limit]
	}

	// Include the key field in the field paths if there is one.
	var fps [][]string
	if len(q.FieldPaths) > 0 && c.keyField != "" {
		fps = append([][]string{{c.keyField}}, q.FieldPaths...)
	} else {
		fps = q.FieldPaths
	}

	return &docIterator{
		docs:       resultDocs,
		fieldPaths: fps,
		revField:   c.opts.RevisionField,
	}, nil
}

func filtersMatch(fs []driver.Filter, doc storedDoc) bool {
	for _, f := range fs {
		if !filterMatches(f, doc) {
			return false
		}
	}
	return true
}

func filterMatches(f driver.Filter, doc storedDoc) bool {
	docval, err := getAtFieldPath(doc, f.FieldPath)
	// missing or bad field path => no match
	if err != nil {
		return false
	}
	c, ok := compare(docval, f.Value)
	if !ok {
		return false
	}
	return applyComparison(f.Op, c)
}

// op is one of the five permitted docstore operators ("=", "<", etc.)
// c is the result of strings.Compare or the like.
// TODO(jba): dedup from gcpfirestore/query?
func applyComparison(op string, c int) bool {
	switch op {
	case driver.EqualOp:
		return c == 0
	case ">":
		return c > 0
	case "<":
		return c < 0
	case ">=":
		return c >= 0
	case "<=":
		return c <= 0
	default:
		panic("bad op")
	}
}

func compare(x1, x2 interface{}) (int, bool) {
	v1 := reflect.ValueOf(x1)
	v2 := reflect.ValueOf(x2)
	if v1.Kind() == reflect.String && v2.Kind() == reflect.String {
		return strings.Compare(v1.String(), v2.String()), true
	}
	if cmp, err := driver.CompareNumbers(v1, v2); err == nil {
		return cmp, true
	}
	if t1, ok := x1.(time.Time); ok {
		if t2, ok := x2.(time.Time); ok {
			return driver.CompareTimes(t1, t2), true
		}
	}
	return 0, false
}

func sortDocs(docs []storedDoc, field string, asc bool) {
	sort.Slice(docs, func(i, j int) bool {
		c, ok := compare(docs[i][field], docs[j][field])
		if !ok {
			return false
		}
		if asc {
			return c < 0
		}
		return c > 0
	})
}

type docIterator struct {
	docs       []storedDoc
	fieldPaths [][]string
	revField   string
	err        error
}

func (it *docIterator) Next(ctx context.Context, doc driver.Document) error {
	if it.err != nil {
		return it.err
	}
	if len(it.docs) == 0 {
		it.err = io.EOF
		return it.err
	}
	if err := decodeDoc(it.docs[0], doc, it.fieldPaths); err != nil {
		it.err = err
		return it.err
	}
	it.docs = it.docs[1:]
	return nil
}

func (it *docIterator) Stop() { it.err = io.EOF }

func (it *docIterator) As(i interface{}) bool { return false }

func (c *collection) QueryPlan(q *driver.Query) (string, error) {
	return "", nil
}
