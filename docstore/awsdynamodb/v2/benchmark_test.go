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
	"context"
	"fmt"
	"strconv"
	"testing"

	awsv2cfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dyn2Types "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var benchmarkTableName = collectionName3

func BenchmarkPutVSTransact(b *testing.B) {
	// This benchmark compares two ways to replace N items and retrieve their previous values.
	// The first way makes N calls to PutItem with ReturnValues set to ALL_OLD.
	// The second way calls BatchGetItem followed by TransactWriteItem.
	//
	// The results show that separate PutItems are faster for up to two items.
	cfg, err := awsv2cfg.LoadDefaultConfig(context.Background())
	if err != nil {
		b.Fatal("Error initializing aws session for benchmark: ", err)
	}
	db := dynamodb.NewFromConfig(cfg)

	for nItems := 1; nItems <= 5; nItems++ {
		b.Run(fmt.Sprintf("%d-Items", nItems), func(b *testing.B) {
			var items []map[string]dyn2Types.AttributeValue
			for i := 0; i < nItems; i++ {
				items = append(items, map[string]dyn2Types.AttributeValue{
					"name": &dyn2Types.AttributeValueMemberS{Value: fmt.Sprintf("pt-vs-transact-%d", i)},
					"x":    &dyn2Types.AttributeValueMemberN{Value: strconv.Itoa(i)},
					"rev":  &dyn2Types.AttributeValueMemberN{Value: "1"},
				})
			}
			for _, item := range items {
				_, err := db.PutItem(context.Background(), &dynamodb.PutItemInput{
					TableName: &benchmarkTableName,
					Item:      item,
				})
				if err != nil {
					b.Fatal(err)
				}
			}
			b.Run("PutItem", func(b *testing.B) {
				for n := 0; n < b.N; n++ {
					putItems(b, db, items)
				}
			})
			b.Run("TransactWrite", func(b *testing.B) {
				for n := 0; n < b.N; n++ {
					batchGetTransactWrite(b, db, items)
				}
			})
		})
	}
}

func putItems(b *testing.B, db *dynamodb.Client, items []map[string]dyn2Types.AttributeValue) {
	b.Helper()

	for i, item := range items {
		item["x"] = &dyn2Types.AttributeValueMemberN{Value: strconv.Itoa(i + 1)}
		in := &dynamodb.PutItemInput{
			TableName:    &benchmarkTableName,
			Item:         item,
			ReturnValues: dyn2Types.ReturnValueAllOld,
		}
		ce, err := expression.NewBuilder().
			WithCondition(expression.Name("rev").Equal(expression.Value(1))).
			Build()
		if err != nil {
			b.Fatal(err)
		}
		in.ExpressionAttributeNames = ce.Names()
		in.ExpressionAttributeValues = ce.Values()
		in.ConditionExpression = ce.Condition()
		out, err := db.PutItem(context.Background(), in)
		if err != nil {
			b.Fatal(err)
		}
		if got, want := len(out.Attributes), 3; got != want {
			b.Fatalf("got %d attributes, want %d", got, want)
		}
	}
}

func batchGetTransactWrite(b *testing.B, db *dynamodb.Client, items []map[string]dyn2Types.AttributeValue) {
	b.Helper()

	keys := make([]map[string]dyn2Types.AttributeValue, len(items))
	tws := make([]dyn2Types.TransactWriteItem, len(items))
	for i, item := range items {
		keys[i] = map[string]dyn2Types.AttributeValue{"name": items[i]["name"]}
		item["x"] = &dyn2Types.AttributeValueMemberN{Value: strconv.Itoa(i + 2)}
		put := &dyn2Types.Put{TableName: &benchmarkTableName, Item: items[i]}
		ce, err := expression.NewBuilder().
			WithCondition(expression.Name("rev").Equal(expression.Value(1))).
			Build()
		if err != nil {
			b.Fatal(err)
		}
		put.ExpressionAttributeNames = ce.Names()
		put.ExpressionAttributeValues = ce.Values()
		put.ConditionExpression = ce.Condition()
		tws[i] = dyn2Types.TransactWriteItem{Put: put}
	}
	_, err := db.BatchGetItem(context.Background(), &dynamodb.BatchGetItemInput{
		RequestItems: map[string]dyn2Types.KeysAndAttributes{
			benchmarkTableName: {Keys: keys},
		},
	})
	if err != nil {
		b.Fatal(err)
	}
	_, err = db.TransactWriteItems(context.Background(), &dynamodb.TransactWriteItemsInput{TransactItems: tws})
	if err != nil {
		b.Fatal(err)
	}
}
