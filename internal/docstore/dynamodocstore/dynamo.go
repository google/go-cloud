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
//  - ActionList.BeforeDo: *dynamodb.TransactGetItemsInput or *dynamodb.TransactWriteItemsInput
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
	if opts.Unordered {
		return c.runActionsUnordered(ctx, actions, opts)
	}
	return c.runActionsOrdered(ctx, actions, opts)
}

func (c *collection) runActionsOrdered(ctx context.Context, actions []*driver.Action, opts *driver.RunActionsOptions) driver.ActionListError {
	groups := c.splitActions(actions)
	nRun := 0 // number of actions successfully run
	var err error
	for _, g := range groups {
		if g[0].Kind == driver.Get {
			err = c.runGets(ctx, g, opts)[0]
		} else {
			err = c.runWrites(ctx, g, opts)
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
		groups [][]*driver.Action // the actions, split; the return value
		cur    []*driver.Action   // the group currently being constructed
	)
	collect := func() { // called when the current group is known to be finished
		if len(cur) > 0 {
			groups = append(groups, cur)
			cur = nil
		}
	}
	for _, a := range actions {
		if len(cur) > 0 && c.shouldSplit(cur[len(cur)-1], a) ||
			len(cur) >= 10 { // each transaction can run up to 10 operations.
			collect()
		}
		cur = append(cur, a)
	}
	collect()
	return groups
}

func (c *collection) shouldSplit(curr, next *driver.Action) bool {
	if (curr.Kind == driver.Get) != (next.Kind == driver.Get) { // different kind
		return true
	}
	return false
}

func (c *collection) runActionsUnordered(ctx context.Context, actions []*driver.Action, opts *driver.RunActionsOptions) driver.ActionListError {
	errs := make([]error, len(actions))
	groups, i, err := c.splitActionsUnordered(actions)
	if err != nil {
		errs[i] = err
		return driver.NewActionListError(errs)
	}
	var wg sync.WaitGroup
	groupBaseIndex := 0 // index in actions of first action in group
	for _, g := range groups {
		g := g
		base := groupBaseIndex
		wg.Add(1)
		go func() {
			defer wg.Done()
			if g[0].Kind == driver.Get {
				for i, err := range c.runGets(ctx, g, opts) {
					errs[base+i] = err
				}
			} else {
				err := c.runWrites(ctx, g, opts)
				for i := 0; i < len(g); i++ {
					errs[base+i] = err
				}
			}
		}()
		groupBaseIndex += len(g)
	}
	wg.Wait()
	return driver.NewActionListError(errs)
}

// splitActionsUnordered divides the actions slice into sub-slices, which
// maximizes the actions sent in each group and may change the order of the
// action list, each of which can be passed to run a dynamo transaction
// operation. If an error occurs, it returns the error and the index of the
// action that cause the error.
func (c *collection) splitActionsUnordered(actions []*driver.Action) ([][]*driver.Action, int, error) {
	var (
		groups [][]*driver.Action // the actions, split; the return value
		gets   []*driver.Action   // the gets group currently being constructed
		writes []*driver.Action   // the writes group currently being constructed
	)
	collect := func(cur *[]*driver.Action) { // called when the current group is known to be finished
		if len(*cur) > 0 {
			groups = append(groups, *cur)
			*cur = nil
		}
	}
	for _, a := range actions {
		cur := &writes
		if a.Kind == driver.Get {
			cur = &gets
		}
		*cur = append(*cur, a)
		if len(*cur) >= 10 {
			collect(cur)
		}
	}
	collect(&gets)
	collect(&writes)
	return groups, -1, nil
}

// runGets runs a list of gets in a transaction call. When running in ordered
// mode, it returns the first error encountered, if any; when running in
// unordered mode, it returns a list of errors with indices matched with these
// in the action list.
func (c *collection) runGets(ctx context.Context, actions []*driver.Action, opts *driver.RunActionsOptions) (errs []error) {
	if opts.Unordered {
		errs = make([]error, len(actions))
	} else {
		errs = make([]error, 1)
	}
	setErr := func(err error) {
		for i := range errs {
			errs[i] = err
		}
	}

	// Assume all actions Kinds are Gets.
	tgs := make([]*dyn.TransactGetItem, len(actions))
	for i, a := range actions {
		tg, err := c.toTransactGet(a.Doc, a.FieldPaths)
		if err != nil {
			setErr(err)
			return errs
		}
		tgs[i] = tg
	}

	in := &dyn.TransactGetItemsInput{TransactItems: tgs}
	if opts.BeforeDo != nil {
		asFunc := func(i interface{}) bool {
			p, ok := i.(**dyn.TransactGetItemsInput)
			if !ok {
				return false
			}
			*p = in
			return true
		}
		if err := opts.BeforeDo(asFunc); err != nil {
			setErr(err)
			return errs
		}
	}
	out, err := c.db.TransactGetItemsWithContext(ctx, in)
	if err != nil {
		setErr(err)
		return errs
	}

	for i, res := range out.Responses {
		item := res.Item
		if opts.Unordered {
			if item == nil {
				errs[i] = gcerr.Newf(gcerr.NotFound, nil, "item %v not found", actions[i].Doc)
			} else {
				errs[i] = decodeDoc(&dyn.AttributeValue{M: res.Item}, actions[i].Doc)
			}
		} else {
			if item == nil {
				setErr(gcerr.Newf(gcerr.NotFound, nil, "item %v not found", actions[i].Doc))
				return errs
			} else if err := decodeDoc(&dyn.AttributeValue{M: res.Item}, actions[i].Doc); err != nil {
				setErr(err)
				return errs
			}
		}
	}
	return errs
}

func (c *collection) runWrites(ctx context.Context, actions []*driver.Action, opts *driver.RunActionsOptions) error {
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
			return err
		}
	}
	if _, err := c.db.TransactWriteItemsWithContext(ctx, in); err != nil {
		return err
	}
	for i, a := range actions {
		if a.Kind == driver.Create {
			if _, err := a.Doc.GetField(c.partitionKey); err != nil && gcerrors.Code(err) == gcerrors.NotFound {
				actions[i].Doc.SetField(c.partitionKey, *tws[i].Put.Item[c.partitionKey].S)
			}
		}
	}
	return nil
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
