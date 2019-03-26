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
	"strings"
	"time"

	"gocloud.dev/gcerrors"

	"github.com/aws/aws-sdk-go/aws/awserr"
	dyn "github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/expression"
	"gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/driver"
	"gocloud.dev/internal/gcerr"
)

type collection struct {
	db           *dyn.DynamoDB
	table        string // DynamoDB table name
	partitionKey string
	sortKey      string
}

// OpenCollection creates a *docstore.Collection representing a DynamoDB collection.
func OpenCollection(db *dyn.DynamoDB, tableName, partitionKey, sortKey string) *docstore.Collection {
	return docstore.NewCollection(newCollection(db, tableName, partitionKey, sortKey))
}

func newCollection(db *dyn.DynamoDB, tableName, partitionKey, sortKey string) *collection {
	c := &collection{
		db:           db,
		table:        tableName,
		partitionKey: partitionKey,
		sortKey:      sortKey,
	}
	return c

}

func (c *collection) KeyFields() []string {
	if c.sortKey == "" {
		return []string{c.partitionKey}
	}
	return []string{c.partitionKey, c.sortKey}
}

func (c *collection) RunActions(ctx context.Context, actions []*driver.Action) (int, error) {
	for i, a := range actions {
		var pc *expression.ConditionBuilder
		var err error
		if a.Kind != driver.Create && a.Kind != driver.Get {
			pc, err = revisionPrecondition(a.Doc)
			if err != nil {
				return i, err
			}
		}
		switch a.Kind {
		case driver.Create:
			cb := expression.AttributeNotExists(expression.Name(c.partitionKey))
			err = c.put(ctx, a.Kind, a.Doc, &cb)
		case driver.Replace:
			if pc == nil {
				c := expression.AttributeExists(expression.Name(c.partitionKey))
				pc = &c
			}
			err = c.put(ctx, a.Kind, a.Doc, pc)
		case driver.Put:
			err = c.put(ctx, a.Kind, a.Doc, pc)
		case driver.Delete:
			err = c.delete(ctx, a.Doc, pc)
		case driver.Get:
			err = c.get(ctx, a.Doc, a.FieldPaths)
		case driver.Update:
			cb := expression.AttributeExists(expression.Name(c.partitionKey))
			if pc != nil {
				cb = cb.And(*pc)
			}
			err = c.update(ctx, a.Doc, a.Mods, &cb)
		default:
			panic("unimp")
		}
		if err != nil {
			return i, err
		}
	}
	return len(actions), nil
}

func (c *collection) missingKeyField(m map[string]*dyn.AttributeValue) string {
	if _, ok := m[c.partitionKey]; !ok {
		return c.partitionKey
	}
	if _, ok := m[c.sortKey]; !ok && c.sortKey != "" {
		return c.sortKey
	}
	return ""
}

func (c *collection) put(ctx context.Context, k driver.ActionKind, doc driver.Document, condition *expression.ConditionBuilder) error {
	av, err := encodeDoc(doc)
	if err != nil {
		return err
	}
	mf := c.missingKeyField(av.M)
	if k != driver.Create && mf != "" {
		return fmt.Errorf("missing key field %q", mf)
	}
	var newPartitionKey string
	if mf == c.partitionKey {
		newPartitionKey = driver.UniqueString()
		av.M[c.partitionKey] = new(dyn.AttributeValue).SetS(newPartitionKey)
	}
	if c.sortKey != "" && mf == c.sortKey {
		// It doesn't make sense to generate a random sort key.
		return fmt.Errorf("missing sort key %q", c.sortKey)
	}

	if av.M[docstore.RevisionField], err = encodeValue(time.Now().UTC()); err != nil {
		return err
	}
	in := &dyn.PutItemInput{
		TableName: &c.table,
		Item:      av.M,
	}
	if condition != nil {
		ce, err := expression.NewBuilder().WithCondition(*condition).Build()
		if err != nil {
			return err
		}
		in.ExpressionAttributeNames = ce.Names()
		in.ExpressionAttributeValues = ce.Values()
		in.ConditionExpression = ce.Condition()
	}
	_, err = c.db.PutItemWithContext(ctx, in)
	if err == nil && newPartitionKey != "" {
		doc.SetField(c.partitionKey, newPartitionKey)
	}
	return err
}

func (c *collection) get(ctx context.Context, doc driver.Document, fieldpaths [][]string) error {
	av, err := encodeDocKeyFields(doc, c.partitionKey, c.sortKey)
	if err != nil {
		return err
	}
	if len(fieldpaths) > 0 {
		return errors.New("Get with field paths unimplemented")
	}
	in := &dyn.GetItemInput{
		TableName: &c.table,
		Key:       av.M,
	}
	out, err := c.db.GetItemWithContext(ctx, in)
	if err != nil {
		return err
	}
	return decodeDoc(&dyn.AttributeValue{M: out.Item}, doc)
}

func (c *collection) delete(ctx context.Context, doc driver.Document, condition *expression.ConditionBuilder) error {
	av, err := encodeDocKeyFields(doc, c.partitionKey, c.sortKey)
	if err != nil {
		return err
	}

	in := &dyn.DeleteItemInput{
		TableName: &c.table,
		Key:       av.M,
	}
	if condition != nil {
		ce, err := expression.NewBuilder().WithCondition(*condition).Build()
		if err != nil {
			return err
		}
		in.ExpressionAttributeNames = ce.Names()
		in.ExpressionAttributeValues = ce.Values()
		in.ConditionExpression = ce.Condition()
	}
	_, err = c.db.DeleteItemWithContext(ctx, in)
	return err
}

func (c *collection) update(ctx context.Context, doc driver.Document, mods []driver.Mod, condition *expression.ConditionBuilder) error {
	if len(mods) == 0 {
		return nil
	}
	av, err := encodeDocKeyFields(doc, c.partitionKey, c.sortKey)
	if err != nil {
		return err
	}
	var ub expression.UpdateBuilder
	for _, m := range mods {
		// TODO(shantuo): check for invalid field paths
		fp := strings.Join(m.FieldPath, ".")
		if m.Value == nil {
			ub = ub.Remove(expression.Name(fp))
		} else {
			ub = ub.Set(expression.Name(fp), expression.Value(m.Value))
		}
	}
	ub = ub.Set(expression.Name(docstore.RevisionField), expression.Value(time.Now().UTC()))
	ce, err := expression.NewBuilder().WithCondition(*condition).WithUpdate(ub).Build()
	if err != nil {
		return err
	}
	_, err = c.db.UpdateItemWithContext(ctx, &dyn.UpdateItemInput{
		TableName:                 &c.table,
		Key:                       av.M,
		ConditionExpression:       ce.Condition(),
		UpdateExpression:          ce.Update(),
		ExpressionAttributeNames:  ce.Names(),
		ExpressionAttributeValues: ce.Values(),
	})
	return err
}

// revisionPrecondition returns a DynamoDB expression that asserts that the
// stored document's revision matches the revision of doc.
func revisionPrecondition(doc driver.Document) (*expression.ConditionBuilder, error) {
	v, err := doc.GetField(docstore.RevisionField)
	if err != nil {
		return nil, nil
	}
	rev, ok := v.(string)
	if !ok {
		return nil, gcerr.Newf(gcerr.InvalidArgument, nil,
			"%s field contains wrong type: got %T, want string",
			docstore.RevisionField, v)
	}
	if rev == "" {
		return nil, nil
	}
	// Value encodes rev to an attribute value.
	cb := expression.Name(docstore.RevisionField).Equal(expression.Value(rev))
	return &cb, nil
}

func (c *collection) ErrorCode(err error) gcerr.ErrorCode {
	ae, ok := err.(awserr.Error)
	if !ok {
		return gcerr.Unknown
	}
	ec, ok := errorCodeMap[ae.Code()]
	if !ok {
		return gcerr.Unknown
	}
	return ec
}

var errorCodeMap = map[string]gcerrors.ErrorCode{
	dyn.ErrCodeConditionalCheckFailedException:          gcerr.FailedPrecondition,
	dyn.ErrCodeProvisionedThroughputExceededException:   gcerr.ResourceExhausted,
	dyn.ErrCodeResourceNotFoundException:                gcerr.FailedPrecondition,
	dyn.ErrCodeItemCollectionSizeLimitExceededException: gcerr.ResourceExhausted,
	dyn.ErrCodeTransactionConflictException:             gcerr.Internal,
	dyn.ErrCodeRequestLimitExceeded:                     gcerr.ResourceExhausted,
	dyn.ErrCodeInternalServerError:                      gcerr.Internal,
}
