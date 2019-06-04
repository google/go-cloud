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

// Package dynamodocstore provides a docstore implementation backed by AWS
// DynamoDB.
// Use OpenCollection to construct a *docstore.Collection.
//
// URLs
//
// For docstore.OpenCollection, dynamodocstore registers for the scheme
// "dynamodb". The default URL opener will use an AWS session with the default
// credentials and configuration; see
// https://docs.aws.amazon.com/sdk-for-go/api/aws/session/ for more details.
// To customize the URL opener, or for more details on the URL format, see
// URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// As
//
// dynamodocstore exposes the following types for As:
//  - Collection.As: *dynamodb.DynamoDB
//  - ActionList.BeforeDo: *dynamodb.BatchGetItemsInput or *dynamodb.TransactWriteItemsInput
//  - Query.BeforeQuery: *dynamodb.QueryInput or *dynamodb.ScanInput
//  - DocumentIterator: *dynamodb.QueryOutput or *dynamodb.ScanOutput
package dynamodocstore

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/client"
	dyn "github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/expression"
	gcaws "gocloud.dev/aws"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/driver"
	"gocloud.dev/internal/gcerr"
)

func init() {
	docstore.DefaultURLMux().RegisterCollection(Scheme, new(lazySessionOpener))
}

type lazySessionOpener struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

func (o *lazySessionOpener) OpenCollectionURL(ctx context.Context, u *url.URL) (*docstore.Collection, error) {
	o.init.Do(func() {
		sess, err := gcaws.NewDefaultSession()
		if err != nil {
			o.err = err
			return
		}
		o.opener = &URLOpener{
			ConfigProvider: sess,
		}
	})
	if o.err != nil {
		return nil, fmt.Errorf("open collection %s: %v", u, o.err)
	}
	return o.opener.OpenCollectionURL(ctx, u)
}

// Scheme is the URL scheme dynamodb registers its URLOpener under on
// docstore.DefaultMux.
const Scheme = "dynamodb"

// URLOpener opens dynamodb URLs like
// "dynamodb://mytable?partition_key=partkey&sort_key=sortkey".
//
// The URL Host is used as the table name. See
// https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/HowItWorks.NamingRulesDataTypes.html
// for more details.
//
// The following query parameters are supported:
//
//   - partition_key (required): the path to the partition key of a table or an index.
//   - sort_key: the path to the sort key of a table or an index.
//   - allow_scans: if "true", allow table scans to be used for queries
//
// See https://godoc.org/gocloud.dev/aws#ConfigFromURLParams for supported query
// parameters for overriding the aws.Session from the URL.
type URLOpener struct {
	// ConfigProvider must be set to a non-nil value.
	ConfigProvider client.ConfigProvider
}

// OpenCollectionURL opens the collection at the URL's path. See the package doc for more details.
func (o *URLOpener) OpenCollectionURL(_ context.Context, u *url.URL) (*docstore.Collection, error) {
	db, tableName, partitionKey, sortKey, opts, err := o.processURL(u)
	if err != nil {
		return nil, err
	}
	return OpenCollection(db, tableName, partitionKey, sortKey, opts)
}

func (o *URLOpener) processURL(u *url.URL) (db *dyn.DynamoDB, tableName, partitionKey, sortKey string, opts *Options, err error) {
	q := u.Query()

	partitionKey = q.Get("partition_key")
	if partitionKey == "" {
		return nil, "", "", "", nil, fmt.Errorf("open collection %s: partition_key is required to open a table", u)
	}
	q.Del("partition_key")
	sortKey = q.Get("sort_key")
	q.Del("sort_key")
	allowScans := q.Get("allow_scans")
	q.Del("allow_scans")
	opts = &Options{AllowScans: allowScans == "true"}

	tableName = u.Host
	if tableName == "" {
		return nil, "", "", "", nil, fmt.Errorf("open collection %s: URL's host cannot be empty (the table name)", u)
	}
	if u.Path != "" {
		return nil, "", "", "", nil, fmt.Errorf("open collection %s: URL path must be empty, only the host is needed", u)
	}

	configProvider := &gcaws.ConfigOverrider{
		Base: o.ConfigProvider,
	}
	overrideCfg, err := gcaws.ConfigFromURLParams(q)
	if err != nil {
		return nil, "", "", "", nil, fmt.Errorf("open collection %s: %v", u, err)
	}
	configProvider.Configs = append(configProvider.Configs, overrideCfg)
	db, err = Dial(configProvider)
	if err != nil {
		return nil, "", "", "", nil, fmt.Errorf("open collection %s: %v", u, err)
	}
	return db, tableName, partitionKey, sortKey, opts, nil
}

// Dial gets an AWS DynamoDB service client.
func Dial(p client.ConfigProvider) (*dyn.DynamoDB, error) {
	if p == nil {
		return nil, errors.New("getting Dynamo service: no AWS session provided")
	}
	return dyn.New(p), nil
}

type collection struct {
	db           *dyn.DynamoDB
	table        string // DynamoDB table name
	partitionKey string
	sortKey      string
	description  *dyn.TableDescription
	opts         *Options
}

type Options struct {
	// If false, queries that can only be executed by scanning the entire table
	// return an error instead (with the exception of a query with no filters).
	AllowScans bool
}

// OpenCollection creates a *docstore.Collection representing a DynamoDB collection.
func OpenCollection(db *dyn.DynamoDB, tableName, partitionKey, sortKey string, opts *Options) (*docstore.Collection, error) {
	c, err := newCollection(db, tableName, partitionKey, sortKey, opts)
	if err != nil {
		return nil, err
	}
	return docstore.NewCollection(c), nil
}

func newCollection(db *dyn.DynamoDB, tableName, partitionKey, sortKey string, opts *Options) (*collection, error) {
	out, err := db.DescribeTable(&dyn.DescribeTableInput{TableName: &tableName})
	if err != nil {
		return nil, err
	}
	if opts == nil {
		opts = &Options{}
	}
	return &collection{
		db:           db,
		table:        tableName,
		partitionKey: partitionKey,
		sortKey:      sortKey,
		description:  out.Table,
		opts:         opts,
	}, nil
}

// Key returns a two-element array with the partition key and sort key, if any.
func (c *collection) Key(doc driver.Document) (interface{}, error) {
	var keys [2]interface{}
	var err error
	keys[0], err = doc.GetField(c.partitionKey)
	if err != nil {
		return nil, nil // missing key is not an error
	}
	if c.sortKey != "" {
		keys[1], _ = doc.GetField(c.sortKey) // ignore error since keys[1] is nil in that case
	}
	return keys, nil
}

func (c *collection) RunActions(ctx context.Context, actions []*driver.Action, opts *driver.RunActionsOptions) driver.ActionListError {
	errs := make([]error, len(actions))
	beforeGets, gets, writes, afterGets := driver.GroupActions(actions)
	c.runGets(ctx, beforeGets, errs, opts)
	ch := make(chan struct{})
	go func() { defer close(ch); c.runWrites(ctx, writes, errs, opts) }()
	c.runGets(ctx, gets, errs, opts)
	<-ch
	c.runGets(ctx, afterGets, errs, opts)
	return driver.NewActionListError(errs)
}

func (c *collection) runGets(ctx context.Context, actions []*driver.Action, errs []error, opts *driver.RunActionsOptions) {
	const batchSize = 100
	var wg sync.WaitGroup
	for _, group := range driver.GroupByFieldPath(actions) {
		n := len(group) / batchSize
		for i := 0; i < n; i++ {
			i := i
			wg.Add(1)
			go func() {
				defer wg.Done()
				c.batchGet(ctx, group, errs, opts, batchSize*i, batchSize*(i+1)-1)
			}()
		}
		if n*batchSize < len(group) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				c.batchGet(ctx, group, errs, opts, batchSize*n, len(group)-1)
			}()
		}
	}
	wg.Wait()
}

func (c *collection) batchGet(ctx context.Context, gets []*driver.Action, errs []error, opts *driver.RunActionsOptions, start, end int) {
	// errors need to be mapped to the actions' indices.
	setErr := func(err error) {
		for i := start; i <= end; i++ {
			errs[gets[i].Index] = err
		}
	}

	keys := make([]map[string]*dyn.AttributeValue, 0, end-start+1)
	for i := start; i <= end; i++ {
		av, err := encodeDocKeyFields(gets[i].Doc, c.partitionKey, c.sortKey)
		if err != nil {
			errs[gets[i].Index] = err
		}

		keys = append(keys, av.M)
	}
	ka := &dyn.KeysAndAttributes{
		Keys:           keys,
		ConsistentRead: aws.Bool(true),
	}
	if len(gets[start].FieldPaths) != 0 {
		// We need to add the key fields if the user doesn't include them. The
		// BatchGet API doesn't return them otherwise.
		var hasP, hasS bool
		nbs := []expression.NameBuilder{expression.Name(docstore.RevisionField)}
		for _, fp := range gets[start].FieldPaths {
			p := strings.Join(fp, ".")
			nbs = append(nbs, expression.Name(p))
			if p == c.partitionKey {
				hasP = true
			} else if p == c.sortKey {
				hasS = true
			}
		}
		if !hasP {
			nbs = append(nbs, expression.Name(c.partitionKey))
		}
		if c.sortKey != "" && !hasS {
			nbs = append(nbs, expression.Name(c.sortKey))
		}
		expr, err := expression.NewBuilder().
			WithProjection(expression.AddNames(expression.ProjectionBuilder{}, nbs...)).
			Build()
		if err != nil {
			setErr(err)
			return
		}
		ka.ProjectionExpression = expr.Projection()
		ka.ExpressionAttributeNames = expr.Names()
	}
	in := &dyn.BatchGetItemInput{RequestItems: map[string]*dyn.KeysAndAttributes{c.table: ka}}
	if opts.BeforeDo != nil {
		asFunc := func(i interface{}) bool {
			p, ok := i.(**dyn.BatchGetItemInput)
			if !ok {
				return false
			}
			*p = in
			return true
		}
		if err := opts.BeforeDo(asFunc); err != nil {
			setErr(err)
			return
		}
	}
	out, err := c.db.BatchGetItemWithContext(ctx, in)
	if err != nil {
		setErr(err)
		return
	}
	found := make([]bool, end-start+1)
	am := mapActionIndices(gets, start, end)
	for _, item := range out.Responses[c.table] {
		if item != nil {
			key := map[string]interface{}{c.partitionKey: nil}
			if c.sortKey != "" {
				key[c.sortKey] = nil
			}
			keysOnly, err := driver.NewDocument(key)
			if err != nil {
				panic(err)
			}
			err = decodeDoc(&dyn.AttributeValue{M: item}, keysOnly)
			if err != nil {
				continue
			}
			decKey, err := c.Key(keysOnly)
			if err != nil {
				continue
			}
			i := am[decKey]
			errs[gets[i].Index] = decodeDoc(&dyn.AttributeValue{M: item}, gets[i].Doc)
			found[i-start] = true
		}
	}
	for delta, f := range found {
		if !f {
			errs[gets[start+delta].Index] = gcerr.Newf(gcerr.NotFound, nil, "item %v not found", gets[start+delta].Doc)
		}
	}
}

func mapActionIndices(actions []*driver.Action, start, end int) map[interface{}]int {
	m := make(map[interface{}]int)
	for i := start; i <= end; i++ {
		m[actions[i].Key] = i
	}
	return m
}

func (c *collection) runWrites(ctx context.Context, writes []*driver.Action, errs []error, opts *driver.RunActionsOptions) {
	// TODO(jba): Do these concurrently.
	for _, a := range writes {
		var pc *expression.ConditionBuilder
		var err error
		if a.Kind != driver.Create {
			pc, err = revisionPrecondition(a.Doc)
			if err != nil {
				errs[a.Index] = err
				continue
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
			errs[a.Index] = err
		}
	}
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

	if av.M[docstore.RevisionField], err = encodeValue(driver.UniqueString()); err != nil {
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

// TODO(jba): use this if/when we support atomic writes.
func (c *collection) transactWrite(ctx context.Context, actions []*driver.Action, errs []error, opts *driver.RunActionsOptions, start, end int) {
	setErr := func(err error) {
		for i := start; i <= end; i++ {
			errs[actions[i].Index] = err
		}
	}

	tws := make([]*dyn.TransactWriteItem, 0, end-start+1)
	for i := start; i <= end; i++ {
		var pc *expression.ConditionBuilder
		var err error
		a := actions[i]
		if a.Kind != driver.Create {
			pc, err = revisionPrecondition(a.Doc)
			if err != nil {
				setErr(err)
				return
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
			setErr(err)
			return
		}
		tws = append(tws, tw)
	}

	in := &dyn.TransactWriteItemsInput{
		ClientRequestToken: aws.String(driver.UniqueString()),
		TransactItems:      tws,
	}

	if opts.BeforeDo != nil {
		asFunc := func(i interface{}) bool {
			p, ok := i.(**dyn.TransactWriteItemsInput)
			if !ok {
				return false
			}
			*p = in
			return true
		}
		if err := opts.BeforeDo(asFunc); err != nil {
			setErr(err)
			return
		}
	}
	if _, err := c.db.TransactWriteItemsWithContext(ctx, in); err != nil {
		setErr(err)
		return
	}
	for i, a := range actions {
		if a.Kind == driver.Create {
			if _, err := a.Doc.GetField(c.partitionKey); err != nil && gcerrors.Code(err) == gcerrors.NotFound {
				actions[i].Doc.SetField(c.partitionKey, *tws[i].Put.Item[c.partitionKey].S)
			}
		}
	}
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
	mf := c.missingKeyField(av.M)
	if k != driver.Create && mf != "" {
		return nil, fmt.Errorf("missing key field %q", mf)
	}
	if mf == c.partitionKey {
		av.M[c.partitionKey] = new(dyn.AttributeValue).SetS(driver.UniqueString())
	}
	if c.sortKey != "" && mf == c.sortKey {
		// It doesn't make sense to generate a random sort key.
		return nil, fmt.Errorf("missing sort key %q", c.sortKey)
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
	return &dyn.TransactWriteItem{Put: put}, nil
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
		fp := expression.Name(strings.Join(m.FieldPath, "."))
		if inc, ok := m.Value.(driver.IncOp); ok {
			ub.Add(fp, expression.Value(inc.Amount))
		} else if m.Value == nil {
			ub = ub.Remove(fp)
		} else {
			ub = ub.Set(fp, expression.Value(m.Value))
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
	if err != nil { // field not present
		return nil, nil
	}
	if v == nil { // field is present, but nil
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

func (c *collection) As(i interface{}) bool {
	p, ok := i.(**dyn.DynamoDB)
	if !ok {
		return false
	}
	*p = c.db
	return true
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
	"ValidationException":                               gcerr.InvalidArgument,
}
