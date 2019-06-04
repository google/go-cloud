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

// Package mongodocstore provides an implementation of the docstore API for MongoDB.
//
// URLs
//
// For docstore.OpenCollection, mongodocstore registers for the scheme "mongo".
// The default URL opener will dial a Mongo server using the environment
// variable "MONGO_SERVER_URL".
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// As
//
// mongodocstore exposes the following types for As:
// - Collection: *mongo.Collection
// - ActionList.BeforeDo: *options.FindOneOptions, *options.InsertOneOptions,
//   *options.ReplaceOptions, *options.UpdateOptions or *options.DeleteOptions
// - Query.BeforeQuery: *options.FindOptions
// - DocumentIterator: *mongo.Cursor
//
// Docstore types not supported by the Go mongo client, go.mongodb.org/mongo-driver/mongo:
// TODO(jba): write
//
// MongoDB types not supported by Docstore:
// TODO(jba): write
package mongodocstore // import "gocloud.dev/internal/docstore/mongodocstore"

// MongoDB reference manual: https://docs.mongodb.com/manual
// Client documentation: https://godoc.org/go.mongodb.org/mongo-driver/mongo
//
// The client methods accept a document of type interface{},
// which is marshaled by the go.mongodb.org/mongo-driver/bson package.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/driver"
	"gocloud.dev/internal/gcerr"
)

func init() {
	docstore.DefaultURLMux().RegisterCollection(Scheme, new(defaultDialer))
}

// defaultDialer dials a default Mongo server based on the environment variable
// MONGO_SERVER_URL.
type defaultDialer struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

func (o *defaultDialer) OpenCollectionURL(ctx context.Context, u *url.URL) (*docstore.Collection, error) {
	o.init.Do(func() {
		serverURL := os.Getenv("MONGO_SERVER_URL")
		if serverURL == "" {
			o.err = errors.New("MONGO_SERVER_URL environment variable is not set")
			return
		}
		client, err := Dial(ctx, serverURL)
		if err != nil {
			o.err = fmt.Errorf("failed to dial default Mongo server at %q: %v", serverURL, err)
			return
		}
		o.opener = &URLOpener{Client: client}
	})
	if o.err != nil {
		return nil, fmt.Errorf("open collection %s: %v", u, o.err)
	}
	return o.opener.OpenCollectionURL(ctx, u)
}

// Dial returns a new mongoDB client that is connected to the server URI.
func Dial(ctx context.Context, uri string) (*mongo.Client, error) {
	opts := options.Client().ApplyURI(uri)
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	client, err := mongo.NewClient(opts)
	if err != nil {
		return nil, err
	}
	if err := client.Connect(ctx); err != nil {
		return nil, err
	}
	return client, nil
}

// Scheme is the URL scheme mongodocstore registers its URLOpener under on
// docstore.DefaultMux.
const Scheme = "mongo"

// URLOpener opens URLs like "mongo://mydb/mycollection".
// See https://docs.mongodb.com/manual/reference/limits/#naming-restrictions for
// naming restrictions.
//
// The URL Host is used as the database name.
// The URL Path is used as the collection name.
//
// No query parameters are supported.
type URLOpener struct {
	// A Client is a MongoDB client that performs operations on the db, must be
	// non-nil.
	Client *mongo.Client

	// Options specifies the options to pass to OpenCollection.
	Options Options
}

// OpenCollectionURL opens the Collection URL.
func (o *URLOpener) OpenCollectionURL(ctx context.Context, u *url.URL) (*docstore.Collection, error) {
	q := u.Query()
	idField := q.Get("id_field")
	q.Del("id_field")
	for param := range q {
		return nil, fmt.Errorf("open collection %s: invalid query parameter %q", u, param)
	}

	dbName := u.Host
	if dbName == "" {
		return nil, fmt.Errorf("open collection %s: URL must have a non-empty Host (database name)", u)
	}
	collName := strings.TrimPrefix(u.Path, "/")
	if collName == "" {
		return nil, fmt.Errorf("open collection %s: URL must have a non-empty Path (collection name)", u)
	}
	return OpenCollection(o.Client.Database(dbName).Collection(collName), idField, &o.Options)
}

type collection struct {
	coll          *mongo.Collection
	idField       string
	idFunc        func(docstore.Document) interface{}
	revisionField string
	opts          *Options
}

type Options struct {
	// Lowercase all field names for document encoding, field selection, update modifications
	// and queries.
	LowercaseFields bool
}

// OpenCollection opens a MongoDB collection for use with Docstore.
// The idField argument is the name of the document field to use for the document ID
// (MongoDB's _id field). If it is empty, the field "_id" will be used.
func OpenCollection(mcoll *mongo.Collection, idField string, opts *Options) (*docstore.Collection, error) {
	dc, err := newCollection(mcoll, idField, nil, opts)
	if err != nil {
		return nil, err
	}
	return docstore.NewCollection(dc), nil
}

// OpenCollectionWithIDFunc opens a MongoDB collection for use with Docstore.
// The idFunc argument is function that accepts a document and returns the value to
// be used for the document ID (MongoDB's _id field). IDFunc should return nil if the
// document is missing the information to construct an ID. This will cause all
// actions, even Create, to fail.
func OpenCollectionWithIDFunc(mcoll *mongo.Collection, idFunc func(docstore.Document) interface{}, opts *Options) (*docstore.Collection, error) {
	dc, err := newCollection(mcoll, "", idFunc, opts)
	if err != nil {
		return nil, err
	}
	return docstore.NewCollection(dc), nil
}

func newCollection(mcoll *mongo.Collection, idField string, idFunc func(docstore.Document) interface{}, opts *Options) (*collection, error) {
	if opts == nil {
		opts = &Options{}
	}
	c := &collection{
		coll:          mcoll,
		idField:       idField,
		idFunc:        idFunc,
		revisionField: docstore.RevisionField,
		opts:          opts,
	}
	if c.idField == "" && c.idFunc == nil {
		c.idField = mongoIDField
	}

	if opts.LowercaseFields {
		c.idField = strings.ToLower(c.idField)
		c.revisionField = strings.ToLower(c.revisionField)
	}
	return c, nil
}

func (c *collection) Key(doc driver.Document) (interface{}, error) {
	if c.idField != "" {
		id, _ := doc.GetField(c.idField)
		return id, nil // missing field is not an error
	}
	id := c.idFunc(doc.Origin)
	if id == nil {
		return nil, gcerr.Newf(gcerr.InvalidArgument, nil, "missing document key")
	}
	return id, nil
}

// From https://docs.mongodb.com/manual/core/document: "The field name _id is
// reserved for use as a primary key; its value must be unique in the collection, is
// immutable, and may be of any type other than an array."
const mongoIDField = "_id"

func (c *collection) RunActions(ctx context.Context, actions []*driver.Action, opts *driver.RunActionsOptions) driver.ActionListError {
	errs := make([]error, len(actions))
	beforeGets, gets, writes, afterGets := driver.GroupActions(actions)
	c.runGets(ctx, beforeGets, errs, opts)
	ch := make(chan []error)
	go func() { ch <- c.bulkWrite(ctx, writes, errs) }()
	c.runGets(ctx, gets, errs, opts)
	writeErrs := <-ch
	c.runGets(ctx, afterGets, errs, opts)
	alerr := driver.NewActionListError(errs)
	for _, werr := range writeErrs {
		alerr = append(alerr, indexedError{-1, werr})
	}
	return alerr
}

type indexedError = struct {
	Index int
	Err   error
}

// TODO(jba): consider doing a bulk get by using Find with an "or" filter.
func (c *collection) runGets(ctx context.Context, gets []*driver.Action, errs []error, opts *driver.RunActionsOptions) {
	var wg sync.WaitGroup
	for _, g := range gets {
		g := g
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[g.Index] = c.get(ctx, g, opts)
		}()
	}
	wg.Wait()
}

func (c *collection) get(ctx context.Context, a *driver.Action, dopts *driver.RunActionsOptions) error {
	opts := options.FindOne()
	if len(a.FieldPaths) > 0 {
		opts.Projection = c.projectionDoc(a.FieldPaths)
	}
	if dopts.BeforeDo != nil {
		if err := dopts.BeforeDo(driver.AsFunc(opts)); err != nil {
			return err
		}
	}
	res := c.coll.FindOne(ctx, bson.D{{"_id", a.Key}}, opts)
	if res.Err() != nil {
		return res.Err()
	}
	var m map[string]interface{}
	if err := res.Decode(&m); err != nil {
		return err
	}
	return decodeDoc(m, a.Doc, c.idField)
}

// Construct a mongo "projection document" from field paths.
// Always include the revision field.
func (c *collection) projectionDoc(fps [][]string) bson.D {
	proj := bson.D{{Key: c.revisionField, Value: 1}}
	for _, fp := range fps {
		proj = append(proj, bson.E{Key: c.toMongoFieldPath(fp), Value: 1})
	}
	return proj
}

func (c *collection) toMongoFieldPath(fp []string) string {
	if c.opts.LowercaseFields {
		sliceToLower(fp)
	}
	return strings.Join(fp, ".")
}

func sliceToLower(s []string) {
	for i, e := range s {
		s[i] = strings.ToLower(e)
	}
}

func (c *collection) prepareCreate(a *driver.Action) (mdoc, createdID interface{}, err error) {
	id := a.Key
	if id == nil {
		// Create a unique ID here. (The MongoDB Go client does this for us when calling InsertOne,
		// but not for BulkWrite.)
		id = primitive.NewObjectID()
		createdID = id
	}
	mdoc, err = c.encodeDoc(a.Doc, id)
	if err != nil {
		return nil, nil, err
	}
	return mdoc, createdID, nil
}

func (c *collection) prepareReplace(a *driver.Action) (filter bson.D, mdoc map[string]interface{}, err error) {
	filter, _, err = c.makeFilter(a.Key, a.Doc)
	if err != nil {
		return nil, nil, err
	}
	mdoc, err = c.encodeDoc(a.Doc, a.Key)
	if err != nil {
		return nil, nil, err
	}
	return filter, mdoc, nil
}

func (c *collection) encodeDoc(doc driver.Document, id interface{}) (map[string]interface{}, error) {
	mdoc, err := encodeDoc(doc, c.opts.LowercaseFields)
	if err != nil {
		return nil, err
	}
	if id != nil {
		if c.idField != "" {
			delete(mdoc, c.idField)
		}
		mdoc[mongoIDField] = id
	}
	mdoc[c.revisionField] = driver.UniqueString()
	return mdoc, nil
}

func (c *collection) prepareUpdate(a *driver.Action) (filter bson.D, updateDoc map[string]bson.D, err error) {
	filter, _, err = c.makeFilter(a.Key, a.Doc)
	if err != nil {
		return nil, nil, err
	}
	updateDoc, err = c.newUpdateDoc(a.Mods)
	if err != nil {
		return nil, nil, err
	}
	return filter, updateDoc, nil
}

func (c *collection) newUpdateDoc(mods []driver.Mod) (map[string]bson.D, error) {
	var (
		sets   bson.D
		unsets bson.D
		incs   bson.D
	)
	for _, m := range mods {
		key := c.toMongoFieldPath(m.FieldPath)
		if m.Value == nil {
			unsets = append(unsets, bson.E{Key: key, Value: ""})
		} else if inc, ok := m.Value.(driver.IncOp); ok {
			val, err := encodeValue(inc.Amount)
			if err != nil {
				return nil, err
			}
			incs = append(incs, bson.E{Key: key, Value: val})
		} else {
			val, err := encodeValue(m.Value)
			if err != nil {
				return nil, err
			}
			sets = append(sets, bson.E{Key: key, Value: val})
		}
	}
	updateDoc := map[string]bson.D{}
	updateDoc["$set"] = append(sets, bson.E{Key: c.revisionField, Value: driver.UniqueString()})
	if len(unsets) > 0 {
		updateDoc["$unset"] = unsets
	}
	if len(incs) > 0 {
		updateDoc["$inc"] = incs
	}
	return updateDoc, nil
}

func (c *collection) makeFilter(id interface{}, doc driver.Document) (filter bson.D, rev interface{}, err error) {
	rev, err = doc.GetField(c.revisionField)
	if err != nil && gcerrors.Code(err) != gcerrors.NotFound {
		return nil, nil, err
	}
	// Only select the document with the given ID.
	filter = bson.D{{"_id", id}}
	// If the given document has a revision, it must match the stored document.
	if rev != nil {
		filter = append(filter, bson.E{Key: c.revisionField, Value: rev})
	}
	return filter, rev, nil
}

// bulkWrite calls the Mongo driver's BulkWrite RPC in unordered mode with the actions, which must
// be writes.
// errs is the slice of errors indexed by the position of the action in the original
// action list. bulkWrite populates this slice. In addition, bulkWrite's return value
// contains errors that cannot be attributed to any single action.
func (c *collection) bulkWrite(ctx context.Context, actions []*driver.Action, errs []error) []error {
	var (
		models          []mongo.WriteModel
		modelActions    []*driver.Action // corresponding action for each model
		newIDs          []interface{}    // new IDs for Create actions, corresponding to models slice
		nDeletes        int64
		nNonCreateWrite int64 // total operations expected from Put, Replace and Update
	)
	for _, a := range actions {
		var m mongo.WriteModel
		var err error
		var newID interface{}
		switch a.Kind {
		case driver.Create:
			m, newID, err = c.newCreateModel(a)
		case driver.Delete:
			m, err = c.newDeleteModel(a)
			if err == nil {
				nDeletes++
			}
		case driver.Replace, driver.Put:
			m, err = c.newReplaceModel(a, a.Kind == driver.Put)
			if err == nil {
				nNonCreateWrite++
			}
		case driver.Update:
			m, err = c.newUpdateModel(a)
			if err == nil && m != nil {
				nNonCreateWrite++
			}
		default:
			err = gcerr.Newf(gcerr.Internal, nil, "bad action %+v", a)
		}
		if err != nil {
			errs[a.Index] = err
		} else if m != nil { // m can be nil for a no-op update
			models = append(models, m)
			modelActions = append(modelActions, a)
			newIDs = append(newIDs, newID)
		}
	}
	if len(models) == 0 {
		return nil
	}
	var reterrs []error
	res, err := c.coll.BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false))
	if err != nil {
		bwe, ok := err.(mongo.BulkWriteException)
		if !ok { // assume everything failed with this error
			return []error{err}
		}
		// The returned indexes of the WriteErrors are wrong. See https://jira.mongodb.org/browse/GODRIVER-1028.
		// Until it's fixed, use negative values for the indexes in the errors we return.
		for _, w := range bwe.WriteErrors {
			reterrs = append(reterrs, gcerr.Newf(translateMongoCode(w.Code), nil, "%s", w.Message))
		}
		return reterrs
	}
	for i, newID := range newIDs {
		if newID == nil {
			continue
		}
		a := modelActions[i]
		if err := a.Doc.SetField(c.idField, newID); err != nil {
			errs[a.Index] = err
		}
	}
	if res.DeletedCount != nDeletes {
		// Some Delete actions failed. It's not an error if a Delete failed because
		// the document didn't exist, but it is an error if it failed because of a
		// precondition mismatch. Find all the documents with revisions we tried to delete; if
		// any are still present, that's an error.
		c.determineDeleteErrors(ctx, models, modelActions, errs)
	}
	if res.MatchedCount+res.UpsertedCount != nNonCreateWrite {
		reterrs = append(reterrs, gcerr.Newf(gcerr.NotFound, nil, "some writes failed (replaced %d, upserted %d, out of total %d)", res.MatchedCount, res.UpsertedCount, nNonCreateWrite))
	}
	return reterrs
}

func (c *collection) determineDeleteErrors(ctx context.Context, models []mongo.WriteModel, actions []*driver.Action, errs []error) {
	// TODO(jba): do this concurrently.
	for i, m := range models {
		if dm, ok := m.(*mongo.DeleteOneModel); ok {
			filter := dm.Filter.(bson.D)
			if len(filter) > 1 {
				// Delete with both ID and revision. See if the document is still there.
				idOnlyFilter := filter[:1]
				res := c.coll.FindOne(ctx, idOnlyFilter)
				// Assume an error means the document wasn't found.
				// That means either that it was deleted successfully, or that it never
				// existed. Either way, it's not an error.
				// TODO(jba): distinguish between not found and other errors.
				if res.Err() == nil {
					// The document exists, but we didn't delete it: assume we had the wrong
					// revision.
					errs[actions[i].Index] = gcerr.Newf(gcerr.FailedPrecondition, nil,
						"wrong revision for document with ID %v", actions[i].Key)
				}
			}
		}
	}
}

func (c *collection) newCreateModel(a *driver.Action) (*mongo.InsertOneModel, interface{}, error) {
	mdoc, createdID, err := c.prepareCreate(a)
	if err != nil {
		return nil, nil, err
	}
	return &mongo.InsertOneModel{Document: mdoc}, createdID, nil
}

func (c *collection) newDeleteModel(a *driver.Action) (*mongo.DeleteOneModel, error) {
	filter, _, err := c.makeFilter(a.Key, a.Doc)
	if err != nil {
		return nil, err
	}
	return &mongo.DeleteOneModel{Filter: filter}, nil
}

func (c *collection) newReplaceModel(a *driver.Action, upsert bool) (*mongo.ReplaceOneModel, error) {
	filter, mdoc, err := c.prepareReplace(a)
	if err != nil {
		return nil, err
	}
	return &mongo.ReplaceOneModel{
		Filter:      filter,
		Replacement: mdoc,
		Upsert:      &upsert,
	}, nil
}

func (c *collection) newUpdateModel(a *driver.Action) (*mongo.UpdateOneModel, error) {
	filter, updateDoc, err := c.prepareUpdate(a)
	if err != nil {
		return nil, err
	}
	if filter == nil { // no-op
		return nil, nil
	}
	return &mongo.UpdateOneModel{Filter: filter, Update: updateDoc}, nil
}

// As implements driver.As.
func (c *collection) As(i interface{}) bool {
	p, ok := i.(**mongo.Collection)
	if !ok {
		return false
	}
	*p = c.coll
	return true
}

func (c *collection) ErrorCode(err error) gcerrors.ErrorCode {
	if g, ok := err.(*gcerr.Error); ok {
		return g.Code
	}
	if err == mongo.ErrNoDocuments {
		return gcerrors.NotFound
	}
	if wexc, ok := err.(mongo.WriteException); ok && len(wexc.WriteErrors) > 0 {
		return translateMongoCode(wexc.WriteErrors[0].Code)
	}
	return gcerrors.Unknown
}

// Error code for a write error when no documents match a filter.
// (The Go mongo driver doesn't define an exported constant for this.)
const mongoDupKeyCode = 11000

func translateMongoCode(code int) gcerrors.ErrorCode {
	switch code {
	case mongoDupKeyCode:
		return gcerrors.FailedPrecondition
	default:
		return gcerrors.Unknown
	}
}
