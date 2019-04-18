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
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	dyn "github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/expression"
	"gocloud.dev/gcerrors"
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

func (c *collection) RunActions(ctx context.Context, actions []*driver.Action, unordered bool) driver.ActionListError {
	if unordered {
		panic("unordered unimplemented")
	}
	groups := c.splitActions(actions)
	nRun := 0 // number of actions successfully run
	var err error
	for _, g := range groups {
		if g[0].Kind == driver.Get {
			err = c.runGets(ctx, g)
		} else {
			err = c.runWrites(ctx, g)
		}
		if err != nil {
			return driver.ActionListError{{nRun, err}}
		}
		nRun += len(g)
	}
	return nil
}

// splitActions divides the actions slice into sub-slices, each of which can be
// passed to run a dynamo transaction operation.
// splitActions doesn't change the order of the input slice.
func (c *collection) splitActions(actions []*driver.Action) [][]*driver.Action {
	var (
		groups [][]*driver.Action      // the actions, split; the return value
		cur    []*driver.Action        // the group currently being constructed
		wm     = make(map[string]bool) // writes group cannot contain duplicate items
	)
	collect := func() { // called when the current group is known to be finished
		if len(cur) > 0 {
			groups = append(groups, cur)
			cur = nil
			wm = make(map[string]bool)
		}
	}
	for _, a := range actions {
		if len(cur) > 0 && c.shouldSplit(cur[len(cur)-1], a, wm) ||
			len(cur) >= 10 { // each transaction can run up to 10 operations.
			collect()
		}
		cur = append(cur, a)
		if a.Kind != driver.Get {
			wm[c.primaryKey(a)] = true
		}
	}
	collect()
	return groups
}

func (c *collection) shouldSplit(curr, next *driver.Action, wm map[string]bool) bool {
	if (curr.Kind == driver.Get) != (next.Kind == driver.Get) { // different kind
		return true
	}
	if curr.Kind == driver.Get { // both are Get's
		return false
	}
	_, ok := wm[c.primaryKey(next)]
	return ok // different Write's in one transaction cannot target the same item
}

// primaryKey tries to get the primary key from the doc, which is the partition
// key if there is no sort key, or the combination of both keys. If there is not
// a key for Create action, it generates a partition key of the doc. It returns
// the composite key to be guaranteed unique.
func (c *collection) primaryKey(a *driver.Action) string {
	pkey, err := a.Doc.GetField(c.partitionKey)
	if err != nil {
		if c.ErrorCode(err) == gcerr.NotFound && a.Kind == driver.Create {
			newPartitionKey := driver.UniqueString()
			a.Doc.SetField(c.partitionKey, newPartitionKey)
			return newPartitionKey
		}
		panic(err) // shouldn't happen
	}
	if c.sortKey != "" {
		skey, err := a.Doc.GetField(c.sortKey)
		if err != nil {
			panic(err) // shouldn't happen
		}
		return fmt.Sprintf("%s, %s", pkey, skey)
	}
	return pkey.(string)
}

func (c *collection) runGets(ctx context.Context, actions []*driver.Action) error {
	// Assume all actions Kinds are Get's.
	tgs := make([]*dyn.TransactGetItem, len(actions))
	for i, a := range actions {
		tg, err := c.toTransactGet(a.Doc, a.FieldPaths)
		if err != nil {
			return err
		}
		tgs[i] = tg
	}

	out, err := c.db.TransactGetItemsWithContext(ctx, &dyn.TransactGetItemsInput{TransactItems: tgs})
	if err != nil {
		return err
	}

	for i, res := range out.Responses {
		if err := decodeDoc(&dyn.AttributeValue{M: res.Item}, actions[i].Doc); err != nil {
			return err
		}
	}
	return nil
}

func (c *collection) runWrites(ctx context.Context, actions []*driver.Action) error {
	tws := make([]*dyn.TransactWriteItem, len(actions))
	for i, a := range actions {
		var pc *expression.ConditionBuilder
		var err error
		if a.Kind != driver.Create {
			pc, err = revisionPrecondition(a.Doc)
			if err != nil {
				return err
			}
		}

		var tw *dyn.TransactWriteItem
		switch a.Kind {
		case driver.Create:
			cb := expression.AttributeNotExists(expression.Name(c.partitionKey))
			tw, err = c.toTransactPut(ctx, a.Kind, a.Doc, &cb)
		case driver.Replace:
			if pc == nil {
				c := expression.AttributeExists(expression.Name(c.partitionKey))
				pc = &c
			}
			tw, err = c.toTransactPut(ctx, a.Kind, a.Doc, pc)
		case driver.Put:
			tw, err = c.toTransactPut(ctx, a.Kind, a.Doc, pc)
		case driver.Delete:
			tw, err = c.toTransactDelete(ctx, a.Doc, pc)
		case driver.Update:
			cb := expression.AttributeExists(expression.Name(c.partitionKey))
			if pc != nil {
				cb = cb.And(*pc)
			}
			tw, err = c.toTransactUpdate(ctx, a.Doc, a.Mods, &cb)
		default:
			panic("wrong action passed in; writes should be of kind Create, Replace, Put, Delete or Update")
		}
		if err != nil {
			return err
		}
		tws[i] = tw
	}

	_, err := c.db.TransactWriteItemsWithContext(ctx, &dyn.TransactWriteItemsInput{
		ClientRequestToken: aws.String(driver.UniqueString()),
		TransactItems:      tws,
	})
	return err
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

func (c *collection) toTransactPut(ctx context.Context, k driver.ActionKind, doc driver.Document, condition *expression.ConditionBuilder) (*dyn.TransactWriteItem, error) {
	av, err := encodeDoc(doc)
	if err != nil {
		return nil, err
	}

	if av.M[docstore.RevisionField], err = encodeValue(driver.UniqueString()); err != nil {
		return nil, err
	}
	put := &dyn.Put{
		TableName: &c.table,
		Item:      av.M,
	}
	if condition != nil {
		ce, err := expression.NewBuilder().WithCondition(*condition).Build()
		if err != nil {
			return nil, err
		}
		put.ExpressionAttributeNames = ce.Names()
		put.ExpressionAttributeValues = ce.Values()
		put.ConditionExpression = ce.Condition()
	}
	return &dyn.TransactWriteItem{Put: put}, err
}

func (c *collection) toTransactGet(doc driver.Document, fieldpaths [][]string) (*dyn.TransactGetItem, error) {
	av, err := encodeDocKeyFields(doc, c.partitionKey, c.sortKey)
	if err != nil {
		return nil, err
	}

	get := &dyn.Get{
		TableName: &c.table,
		Key:       av.M,
	}
	if len(fieldpaths) > 0 {
		// Construct a projection expression for the field paths.
		// See https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/Expressions.ProjectionExpressions.html.
		nbs := []expression.NameBuilder{expression.Name(docstore.RevisionField)}
		for _, fp := range fieldpaths {
			nbs = append(nbs, expression.Name(strings.Join(fp, ".")))
		}
		expr, err := expression.NewBuilder().
			WithProjection(expression.AddNames(expression.ProjectionBuilder{}, nbs...)).
			Build()
		if err != nil {
			return nil, err
		}
		get.ProjectionExpression = expr.Projection()
		get.ExpressionAttributeNames = expr.Names()
	}
	return &dyn.TransactGetItem{Get: get}, nil
}

func (c *collection) toTransactDelete(ctx context.Context, doc driver.Document, condition *expression.ConditionBuilder) (*dyn.TransactWriteItem, error) {
	av, err := encodeDocKeyFields(doc, c.partitionKey, c.sortKey)
	if err != nil {
		return nil, err
	}

	del := &dyn.Delete{
		TableName: &c.table,
		Key:       av.M,
	}
	if condition != nil {
		ce, err := expression.NewBuilder().WithCondition(*condition).Build()
		if err != nil {
			return nil, err
		}
		del.ExpressionAttributeNames = ce.Names()
		del.ExpressionAttributeValues = ce.Values()
		del.ConditionExpression = ce.Condition()
	}
	return &dyn.TransactWriteItem{Delete: del}, nil
}

func (c *collection) toTransactUpdate(ctx context.Context, doc driver.Document, mods []driver.Mod, condition *expression.ConditionBuilder) (*dyn.TransactWriteItem, error) {
	if len(mods) == 0 {
		return nil, nil
	}
	av, err := encodeDocKeyFields(doc, c.partitionKey, c.sortKey)
	if err != nil {
		return nil, err
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
	ub = ub.Set(expression.Name(docstore.RevisionField), expression.Value(driver.UniqueString()))
	ce, err := expression.NewBuilder().WithCondition(*condition).WithUpdate(ub).Build()
	if err != nil {
		return nil, err
	}
	return &dyn.TransactWriteItem{
		Update: &dyn.Update{
			TableName:                 &c.table,
			Key:                       av.M,
			ConditionExpression:       ce.Condition(),
			UpdateExpression:          ce.Update(),
			ExpressionAttributeNames:  ce.Names(),
			ExpressionAttributeValues: ce.Values(),
		},
	}, nil
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
	dyn.ErrCodeResourceNotFoundException:                gcerr.NotFound,
	dyn.ErrCodeItemCollectionSizeLimitExceededException: gcerr.ResourceExhausted,
	dyn.ErrCodeTransactionConflictException:             gcerr.Internal,
	dyn.ErrCodeRequestLimitExceeded:                     gcerr.ResourceExhausted,
	dyn.ErrCodeInternalServerError:                      gcerr.Internal,
	dyn.ErrCodeTransactionCanceledException:             gcerr.FailedPrecondition,
	dyn.ErrCodeTransactionInProgressException:           gcerr.InvalidArgument,
	dyn.ErrCodeIdempotentParameterMismatchException:     gcerr.InvalidArgument,
}
