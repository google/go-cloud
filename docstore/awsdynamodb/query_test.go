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

package awsdynamodb

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gocloud.dev/docstore/driver"
	"gocloud.dev/docstore/drivertest"
)

func TestPlanQuery(t *testing.T) {
	c := &collection{
		table:        "T",
		partitionKey: "tableP",
		description:  &dynamodb.TableDescription{},
		opts:         &Options{AllowScans: true, RevisionField: "rev"},
	}

	// Build an ExpressionAttributeNames map with the given names.
	eans := func(names ...string) map[string]*string {
		m := map[string]*string{}
		for i, n := range names {
			m[fmt.Sprintf("#%d", i)] = aws.String(n)
		}
		return m
	}

	// Build an ExpressionAttributeValues map. Filter values are always the number 1
	// and the keys are always :0, :1, ..., so we only need to know how many entries.
	eavs := func(n int) map[string]*dynamodb.AttributeValue {
		if n == 0 {
			return nil
		}
		one := new(dynamodb.AttributeValue).SetN("1")
		m := map[string]*dynamodb.AttributeValue{}
		for i := 0; i < n; i++ {
			m[fmt.Sprintf(":%d", i)] = one
		}
		return m
	}

	// Ignores the ConsistentRead field from both QueryInput and ScanInput.
	opts := []cmp.Option{
		cmpopts.IgnoreFields(dynamodb.ScanInput{}, "ConsistentRead"),
		cmpopts.IgnoreFields(dynamodb.QueryInput{}, "ConsistentRead"),
	}

	for _, test := range []struct {
		desc string
		// In all cases, the table has a partition key called "tableP".
		tableSortKey            string   // if non-empty, the table sort key
		localIndexSortKey       string   // if non-empty, there is a local index with this sort key
		localIndexFields        []string // the fields projected into the local index
		globalIndexPartitionKey string   // if non-empty, there is a global index with this partition key
		globalIndexSortKey      string   // if non-empty, the global index  has this sort key
		globalIndexFields       []string // the fields projected into the global index
		query                   *driver.Query
		want                    interface{} // either a ScanInput or a QueryInput
		wantPlan                string
	}{
		{
			desc: "empty query",
			// A query with no filters requires a scan.
			query:    &driver.Query{},
			want:     &dynamodb.ScanInput{TableName: &c.table},
			wantPlan: "Scan",
		},
		{
			desc: "equality filter on table partition field",
			// A filter that compares the table's partition key for equality is the minimum
			// requirement for querying the table.
			query: &driver.Query{Filters: []driver.Filter{{[]string{"tableP"}, "=", 1}}},
			want: &dynamodb.QueryInput{
				KeyConditionExpression:    aws.String("#0 = :0"),
				ExpressionAttributeNames:  eans("tableP"),
				ExpressionAttributeValues: eavs(1),
			},
			wantPlan: "Table",
		},
		{
			desc: "equality filter on table partition field (sort key)",
			// Same as above, but the table has a sort key; shouldn't make a difference.
			tableSortKey: "tableS",
			query:        &driver.Query{Filters: []driver.Filter{{[]string{"tableP"}, "=", 1}}},
			want: &dynamodb.QueryInput{
				KeyConditionExpression:    aws.String("#0 = :0"),
				ExpressionAttributeNames:  eans("tableP"),
				ExpressionAttributeValues: eavs(1),
			},
			wantPlan: "Table",
		},
		{
			desc: "equality filter on other field",
			// This query has an equality filter, but not on the table's partition key.
			// Since there are no matching indexes, we must scan.
			query: &driver.Query{Filters: []driver.Filter{{[]string{"other"}, "=", 1}}},
			want: &dynamodb.ScanInput{
				FilterExpression:          aws.String("#0 = :0"),
				ExpressionAttributeNames:  eans("other"),
				ExpressionAttributeValues: eavs(1),
			},
			wantPlan: "Scan",
		},
		{
			desc: "non-equality filter on table partition field",
			// If the query doesn't have an equality filter on the partition key, and there
			// are no indexes, we must scan. The filter becomes a FilterExpression, evaluated
			// on the backend.
			query: &driver.Query{Filters: []driver.Filter{{[]string{"tableP"}, ">", 1}}},
			want: &dynamodb.ScanInput{
				FilterExpression:          aws.String("#0 > :0"),
				ExpressionAttributeNames:  eans("tableP"),
				ExpressionAttributeValues: eavs(1),
			},
			wantPlan: "Scan",
		},
		{
			desc: "equality filter on partition, filter on other",
			// The equality filter on the table's partition key lets us query the table.
			// The other filter is used in the filter expression.
			query: &driver.Query{Filters: []driver.Filter{
				{[]string{"tableP"}, "=", 1},
				{[]string{"other"}, "<=", 1},
			}},
			want: &dynamodb.QueryInput{
				KeyConditionExpression:    aws.String("#1 = :1"),
				FilterExpression:          aws.String("#0 <= :0"),
				ExpressionAttributeNames:  eans("other", "tableP"),
				ExpressionAttributeValues: eavs(2),
			},
			wantPlan: "Table",
		},
		{
			desc: "equality filter on partition, filter on sort",
			// If the table has a sort key and the query has a filter on it as well
			// as an equality filter on the table's partition key, we can query the
			// table.
			tableSortKey: "tableS",
			query: &driver.Query{Filters: []driver.Filter{
				{[]string{"tableP"}, "=", 1},
				{[]string{"tableS"}, "<=", 1},
			}},
			want: &dynamodb.QueryInput{
				KeyConditionExpression:    aws.String("(#0 = :0) AND (#1 <= :1)"),
				ExpressionAttributeNames:  eans("tableP", "tableS"),
				ExpressionAttributeValues: eavs(2),
			},
			wantPlan: "Table",
		},
		{
			desc: "equality filter on table partition, filter on local index sort",
			// The equality filter on the table's partition key allows us to query
			// the table, but there is a better choice: a local index with a sort key
			// that is mentioned in the query.
			localIndexSortKey: "localS",
			query: &driver.Query{Filters: []driver.Filter{
				{[]string{"tableP"}, "=", 1},
				{[]string{"localS"}, "<=", 1},
			}},
			want: &dynamodb.QueryInput{
				IndexName:                aws.String("local"),
				KeyConditionExpression:   aws.String("(#0 = :0) AND (#1 <= :1)"),
				ExpressionAttributeNames: eans("tableP", "localS"),
			},
			wantPlan: `Index: "local"`,
		},
		{
			desc: "equality filter on table partition, filter on local index sort, bad projection",
			// The equality filter on the table's partition key allows us to query
			// the table. There seems to be a better choice: a local index with a sort key
			// that is mentioned in the query. But the query wants the entire document,
			// and the local index only has some fields.
			localIndexSortKey: "localS",
			localIndexFields:  []string{}, // keys only
			query: &driver.Query{Filters: []driver.Filter{
				{[]string{"tableP"}, "=", 1},
				{[]string{"localS"}, "<=", 1},
			}},
			want: &dynamodb.QueryInput{
				KeyConditionExpression:   aws.String("#1 = :1"),
				FilterExpression:         aws.String("#0 <= :0"),
				ExpressionAttributeNames: eans("localS", "tableP"),
			},
			wantPlan: "Table",
		},
		{
			desc: "equality filter on table partition, filter on local index sort, good projection",
			// Same as above, but now the query no longer asks for all fields, so
			// we will only read the requested fields from the table.
			localIndexSortKey: "localS",
			localIndexFields:  []string{}, // keys only
			query: &driver.Query{
				FieldPaths: [][]string{{"tableP"}, {"localS"}},
				Filters: []driver.Filter{
					{[]string{"tableP"}, "=", 1},
					{[]string{"localS"}, "<=", 1},
				}},
			want: &dynamodb.QueryInput{
				IndexName:                 aws.String("local"),
				KeyConditionExpression:    aws.String("(#0 = :0) AND (#1 <= :1)"),
				ExpressionAttributeNames:  eans("tableP", "localS"),
				ExpressionAttributeValues: eavs(2),
				ProjectionExpression:      aws.String("#0, #1"),
			},
			wantPlan: `Index: "local"`,
		},
		{
			desc: "equality filter on table partition, filters on local index and table sort",
			// Given the choice of querying the table or a local index, prefer the table.
			tableSortKey:      "tableS",
			localIndexSortKey: "localS",
			query: &driver.Query{Filters: []driver.Filter{
				{[]string{"tableP"}, "=", 1},
				{[]string{"localS"}, "<=", 1},
				{[]string{"tableS"}, ">", 1},
			}},
			want: &dynamodb.QueryInput{
				IndexName:                nil,
				KeyConditionExpression:   aws.String("(#1 = :1) AND (#2 > :2)"),
				FilterExpression:         aws.String("#0 <= :0"),
				ExpressionAttributeNames: eans("localS", "tableP", "tableS"),
			},
			wantPlan: "Table",
		},
		{
			desc: "equality filter on other field with index",
			// The query is the same as in "equality filter on other field," but now there
			// is a global index with that field as partition key, so we can query it.
			globalIndexPartitionKey: "other",
			query:                   &driver.Query{Filters: []driver.Filter{{[]string{"other"}, "=", 1}}},
			want: &dynamodb.QueryInput{
				IndexName:                aws.String("global"),
				KeyConditionExpression:   aws.String("#0 = :0"),
				ExpressionAttributeNames: eans("other"),
			},
			wantPlan: `Index: "global"`,
		},
		{
			desc: "equality filter on table partition, filter on global index sort",
			// The equality filter on the table's partition key allows us to query
			// the table, but there is a better choice: a global index with the same
			// partition key and a sort key that is mentioned in the query.
			// (In these tests, the global index has all the fields of the table by default.)
			globalIndexPartitionKey: "tableP",
			globalIndexSortKey:      "globalS",
			query: &driver.Query{Filters: []driver.Filter{
				{[]string{"tableP"}, "=", 1},
				{[]string{"globalS"}, "<=", 1},
			}},
			want: &dynamodb.QueryInput{
				IndexName:                aws.String("global"),
				KeyConditionExpression:   aws.String("(#0 = :0) AND (#1 <= :1)"),
				ExpressionAttributeNames: eans("tableP", "globalS"),
			},
			wantPlan: `Index: "global"`,
		},
		{
			desc: "equality filter on table partition, filter on global index sort, bad projection",
			// Although there is a global index that matches the filters best, it doesn't
			// have the necessary fields. So we query against the table.
			// The query does not specify FilterPaths, so it retrieves the entire document.
			// globalIndexFields explicitly lists the fields that the global index has.
			// Since the global index does not have all the document fields, it can't be used.
			globalIndexPartitionKey: "tableP",
			globalIndexSortKey:      "globalS",
			globalIndexFields:       []string{"other"},
			query: &driver.Query{Filters: []driver.Filter{
				{[]string{"tableP"}, "=", 1},
				{[]string{"globalS"}, "<=", 1},
			}},
			want: &dynamodb.QueryInput{
				IndexName:                nil,
				KeyConditionExpression:   aws.String("#1 = :1"),
				FilterExpression:         aws.String("#0 <= :0"),
				ExpressionAttributeNames: eans("globalS", "tableP"),
			},
			wantPlan: "Table",
		},
		{
			desc: "equality filter on table partition, filter on global index sort, good projection",
			// The global index matches the filters best and has the necessary
			// fields. So we query against it.
			globalIndexPartitionKey: "tableP",
			globalIndexSortKey:      "globalS",
			globalIndexFields:       []string{"other", "rev"},
			query: &driver.Query{
				FieldPaths: [][]string{{"other"}},
				Filters: []driver.Filter{
					{[]string{"tableP"}, "=", 1},
					{[]string{"globalS"}, "<=", 1},
				}},
			want: &dynamodb.QueryInput{
				IndexName:                 aws.String("global"),
				KeyConditionExpression:    aws.String("(#0 = :0) AND (#1 <= :1)"),
				ProjectionExpression:      aws.String("#2, #0"),
				ExpressionAttributeNames:  eans("tableP", "globalS", "other"),
				ExpressionAttributeValues: eavs(2),
			},
			wantPlan: `Index: "global"`,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			c.sortKey = test.tableSortKey
			if test.localIndexSortKey == "" {
				c.description.LocalSecondaryIndexes = nil
			} else {
				c.description.LocalSecondaryIndexes = []*dynamodb.LocalSecondaryIndexDescription{
					{
						IndexName:  aws.String("local"),
						KeySchema:  keySchema("tableP", test.localIndexSortKey),
						Projection: indexProjection(test.localIndexFields),
					},
				}
			}
			if test.globalIndexPartitionKey == "" {
				c.description.GlobalSecondaryIndexes = nil
			} else {
				c.description.GlobalSecondaryIndexes = []*dynamodb.GlobalSecondaryIndexDescription{
					{
						IndexName:  aws.String("global"),
						KeySchema:  keySchema(test.globalIndexPartitionKey, test.globalIndexSortKey),
						Projection: indexProjection(test.globalIndexFields),
					},
				}
			}
			gotRunner, err := c.planQuery(test.query)
			if err != nil {
				t.Fatal(err)
			}
			var got interface{}
			switch tw := test.want.(type) {
			case *dynamodb.ScanInput:
				got = gotRunner.scanIn
				tw.TableName = &c.table
				if tw.ExpressionAttributeValues == nil {
					tw.ExpressionAttributeValues = eavs(len(tw.ExpressionAttributeNames))
				}
			case *dynamodb.QueryInput:
				got = gotRunner.queryIn
				tw.TableName = &c.table
				if tw.ExpressionAttributeValues == nil {
					tw.ExpressionAttributeValues = eavs(len(tw.ExpressionAttributeNames))
				}
			default:
				t.Fatalf("bad type for test.want: %T", test.want)
			}
			if diff := cmp.Diff(got, test.want, opts...); diff != "" {
				t.Error("input:\n", diff)
			}
			gotPlan := gotRunner.queryPlan()
			if diff := cmp.Diff(gotPlan, test.wantPlan); diff != "" {
				t.Error("plan:\n", diff)
			}
		})
	}
}

func TestQueryNoScans(t *testing.T) {
	c := &collection{
		table:        "T",
		partitionKey: "tableP",
		description:  &dynamodb.TableDescription{},
		opts:         &Options{AllowScans: false},
	}

	for _, test := range []struct {
		q       *driver.Query
		wantErr bool
	}{
		{&driver.Query{}, false},
		{&driver.Query{Filters: []driver.Filter{{[]string{"other"}, "=", 1}}}, true},
	} {
		qr, err := c.planQuery(test.q)
		if err != nil {
			t.Fatalf("%v: %v", test.q, err)
		}
		err = c.checkPlan(qr)
		if test.wantErr {
			if err == nil || !strings.Contains(err.Error(), "AllowScans") {
				t.Errorf("%v: got %v, want an error that mentions the AllowScans option", test.q, err)
			}
		} else if err != nil {
			t.Errorf("%v: got %v, want nil", test.q, err)
		}
	}
}

// Make a key schema from the names of the partition and sort keys.
func keySchema(pkey, skey string) []*dynamodb.KeySchemaElement {
	return []*dynamodb.KeySchemaElement{
		{AttributeName: &pkey, KeyType: aws.String("HASH")},
		{AttributeName: &skey, KeyType: aws.String("RANGE")},
	}
}

func indexProjection(fields []string) *dynamodb.Projection {
	var ptype string
	switch {
	case fields == nil:
		ptype = "ALL"
	case len(fields) == 0:
		ptype = "KEYS_ONLY"
	default:
		ptype = "INCLUDE"
	}
	proj := &dynamodb.Projection{ProjectionType: &ptype}
	for _, f := range fields {
		f := f
		proj.NonKeyAttributes = append(proj.NonKeyAttributes, &f)
	}
	return proj
}

func TestGlobalFieldsIncluded(t *testing.T) {
	c := &collection{partitionKey: "tableP", sortKey: "tableS"}
	gi := &dynamodb.GlobalSecondaryIndexDescription{
		KeySchema: keySchema("globalP", "globalS"),
	}
	for _, test := range []struct {
		desc         string
		queryFields  []string
		wantKeysOnly bool // when the projection includes only table and index keys
		wantInclude  bool // when the projection includes fields "f" and "g".
	}{
		{
			desc:         "all",
			queryFields:  nil,
			wantKeysOnly: false,
			wantInclude:  false,
		},
		{
			desc:         "key fields",
			queryFields:  []string{"tableS", "globalP"},
			wantKeysOnly: true,
			wantInclude:  true,
		},
		{
			desc:         "included fields",
			queryFields:  []string{"f", "g"},
			wantKeysOnly: false,
			wantInclude:  true,
		},
		{
			desc:         "included and key fields",
			queryFields:  []string{"f", "g", "tableP", "globalS"},
			wantKeysOnly: false,
			wantInclude:  true,
		},
		{
			desc:         "not included field",
			queryFields:  []string{"f", "g", "h"},
			wantKeysOnly: false,
			wantInclude:  false,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			var fps [][]string
			for _, qf := range test.queryFields {
				fps = append(fps, strings.Split(qf, "."))
			}
			q := &driver.Query{FieldPaths: fps}
			for _, p := range []struct {
				name string
				proj *dynamodb.Projection
				want bool
			}{
				{"ALL", indexProjection(nil), true},
				{"KEYS_ONLY", indexProjection([]string{}), test.wantKeysOnly},
				{"INCLUDE", indexProjection([]string{"f", "g"}), test.wantInclude},
			} {
				t.Run(p.name, func(t *testing.T) {
					gi.Projection = p.proj
					got := c.globalFieldsIncluded(q, gi)
					if got != p.want {
						t.Errorf("got %t, want %t", got, p.want)
					}
				})
			}
		})
	}
}

func TestCompare(t *testing.T) {
	tm := time.Now()
	for _, test := range []struct {
		a, b interface{}
		want int
	}{
		{1, 1, 0},
		{1, 2, -1},
		{2, 1, 1},
		{1.5, 2, -1},
		{2.5, 2.1, 1},
		{3.8, 3.8, 0},
		{"x", "x", 0},
		{"x", "xx", -1},
		{"x", "a", 1},
		{tm, tm, 0},
		{tm, tm.Add(1), -1},
		{tm, tm.Add(-1), 1},
		{[]byte("x"), []byte("x"), 0},
		{[]byte("x"), []byte("xx"), -1},
		{[]byte("x"), []byte("a"), 1},
	} {
		got := compare(test.a, test.b)
		if got != test.want {
			t.Errorf("compare(%v, %v) = %d, want %d", test.a, test.b, got, test.want)
		}
	}
}

func TestCopyTopLevel(t *testing.T) {
	type E struct{ C int }
	type S struct {
		A int
		B int
		E
	}

	s := &S{A: 1, B: 2, E: E{C: 3}}
	m := map[string]interface{}{"A": 1, "B": 2, "C": 3}
	for _, test := range []struct {
		dest, src interface{}
		want      interface{}
	}{
		{
			dest: map[string]interface{}{},
			src:  m,
			want: m,
		},
		{
			dest: &S{},
			src:  s,
			want: s,
		},
		{
			dest: map[string]interface{}{},
			src:  s,
			want: m,
		},
		{
			dest: &S{},
			src:  m,
			want: s,
		},
	} {
		dest := drivertest.MustDocument(test.dest)
		src := drivertest.MustDocument(test.src)
		if err := copyTopLevel(dest, src); err != nil {
			t.Fatalf("src=%+v: %v", test.src, err)
		}
		if !cmp.Equal(test.dest, test.want) {
			t.Errorf("src=%+v: got %v, want %v", test.src, test.dest, test.want)
		}
	}
}
