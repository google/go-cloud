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
	"net/http"
	"strconv"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	awscreds "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	dyn "github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/expression"
)

var benchmarkTableName = collectionName3

func BenchmarkPutVSTransact(b *testing.B) {
	// This benchmark compares two ways to replace N items and retrieve their previous values.
	// The first way makes N calls to PutItem with ReturnValues set to ALL_OLD.
	// The second way calls BatchGetItem followed by TransactWriteItem.
	//
	// The results show that separate PutItems are faster for up to two items.
	sess, err := awsSession(region, http.DefaultClient)
	if err != nil {
		b.Fatal(err)
	}
	db := dynamodb.New(sess)

	for nItems := 1; nItems <= 5; nItems++ {
		b.Run(fmt.Sprintf("%d-Items", nItems), func(b *testing.B) {
			var items []map[string]*dynamodb.AttributeValue
			for i := 0; i < nItems; i++ {
				items = append(items, map[string]*dynamodb.AttributeValue{
					"name": new(dyn.AttributeValue).SetS(fmt.Sprintf("pt-vs-transact-%d", i)),
					"x":    new(dyn.AttributeValue).SetN(strconv.Itoa(i)),
					"rev":  new(dyn.AttributeValue).SetN("1"),
				})
			}
			for _, item := range items {
				_, err := db.PutItem(&dynamodb.PutItemInput{
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

func putItems(b *testing.B, db *dynamodb.DynamoDB, items []map[string]*dynamodb.AttributeValue) {
	for i, item := range items {
		item["x"].SetN(strconv.Itoa(i + 1))
		in := &dynamodb.PutItemInput{
			TableName:    &benchmarkTableName,
			Item:         item,
			ReturnValues: aws.String("ALL_OLD"),
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
		out, err := db.PutItem(in)
		if err != nil {
			b.Fatal(err)
		}
		if got, want := len(out.Attributes), 3; got != want {
			b.Fatalf("got %d attributes, want %d", got, want)
		}
	}
}

func batchGetTransactWrite(b *testing.B, db *dynamodb.DynamoDB, items []map[string]*dynamodb.AttributeValue) {
	keys := make([]map[string]*dynamodb.AttributeValue, len(items))
	tws := make([]*dyn.TransactWriteItem, len(items))
	for i, item := range items {
		keys[i] = map[string]*dynamodb.AttributeValue{"name": items[i]["name"]}
		item["x"].SetN(strconv.Itoa(i + 2))
		put := &dynamodb.Put{TableName: &benchmarkTableName, Item: items[i]}
		ce, err := expression.NewBuilder().
			WithCondition(expression.Name("rev").Equal(expression.Value(1))).
			Build()
		if err != nil {
			b.Fatal(err)
		}
		put.ExpressionAttributeNames = ce.Names()
		put.ExpressionAttributeValues = ce.Values()
		put.ConditionExpression = ce.Condition()
		tws[i] = &dynamodb.TransactWriteItem{Put: put}
	}
	_, err := db.BatchGetItem(&dynamodb.BatchGetItemInput{
		RequestItems: map[string]*dynamodb.KeysAndAttributes{
			benchmarkTableName: {Keys: keys},
		},
	})
	if err != nil {
		b.Fatal(err)
	}
	_, err = db.TransactWriteItems(&dynamodb.TransactWriteItemsInput{TransactItems: tws})
	if err != nil {
		b.Fatal(err)
	}
}

func awsSession(region string, client *http.Client) (*session.Session, error) {
	// Provide fake creds if running in replay mode.
	var creds *awscreds.Credentials
	return session.NewSession(&aws.Config{
		HTTPClient:  client,
		Region:      aws.String(region),
		Credentials: creds,
		MaxRetries:  aws.Int(0),
	})
}
