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
// See https://godoc.org/gocloud.dev#hdr-URLs for background information.
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
	return OpenCollection(o.Client.Database(dbName).Collection(collName), idField, &o.Options), nil
}

type collection struct {
	coll    *mongo.Collection
	idField string
	idFunc  func(docstore.Document) interface{}
}

type Options struct{}

// OpenCollection opens a MongoDB collection for use with Docstore.
// The idField argument is the name of the document field to use for the document ID
// (MongoDB's _id field). If it is empty, the field "_id" will be used.
func OpenCollection(mcoll *mongo.Collection, idField string, _ *Options) *docstore.Collection {
	return docstore.NewCollection(newCollection(mcoll, idField, nil))
}

// OpenCollectionWithIDFunc opens a MongoDB collection for use with Docstore.
// The idFunc argument is function that accepts a document and returns the value to
// be used for the document ID (MongoDB's _id field). IDFunc should return nil if the
// document is missing the information to construct an ID. This will cause all
// actions, even Create, to fail.
func OpenCollectionWithIDFunc(mcoll *mongo.Collection, idFunc func(docstore.Document) interface{}, _ *Options) *docstore.Collection {
	return docstore.NewCollection(newCollection(mcoll, "", idFunc))
}

func newCollection(mcoll *mongo.Collection, idField string, idFunc func(docstore.Document) interface{}) *collection {
	c := &collection{coll: mcoll, idField: idField, idFunc: idFunc}
	if c.idField == "" && c.idFunc == nil {
		c.idField = mongoIDField
	}
	return c
}

// From https://docs.mongodb.com/manual/core/document: "The field name _id is
// reserved for use as a primary key; its value must be unique in the collection, is
// immutable, and may be of any type other than an array."
const mongoIDField = "_id"

// TODO(jba): use bulk RPCs.
func (c *collection) RunActions(ctx context.Context, actions []*driver.Action, unordered bool) driver.ActionListError {
	if unordered {
		panic("unordered unimplemented")
	}
	for i, a := range actions {
		if err := c.runAction(ctx, a); err != nil {
			return driver.ActionListError{{i, err}}
		}
	}
	return nil
}

func (c *collection) runAction(ctx context.Context, action *driver.Action) error {
	var err error
	switch action.Kind {
	case driver.Get:
		err = c.get(ctx, action)

	case driver.Create:
		err = c.create(ctx, action)

	case driver.Replace, driver.Put:
		err = c.replace(ctx, action, action.Kind == driver.Put)

	case driver.Delete:
		err = c.delete(ctx, action)

	case driver.Update:
		err = c.update(ctx, action)

	default:
		err = gcerr.Newf(gcerr.Internal, nil, "bad action %+v", action)
	}
	return err
}

func (c *collection) get(ctx context.Context, a *driver.Action) error {
	id, err := c.docID(a.Doc)
	if err != nil {
		return err
	}
	opts := options.FindOne()
	if len(a.FieldPaths) > 0 {
		opts.Projection = projectionDoc(a.FieldPaths)
	}
	res := c.coll.FindOne(ctx, bson.D{{"_id", id}}, opts)
	if res.Err() != nil {
		return res.Err()
	}
	var m map[string]interface{}
	// TODO(jba): Benchmark the double decode to see if it's worth trying to avoid it.
	if err := res.Decode(&m); err != nil {
		return err
	}
	return decodeDoc(m, a.Doc)
}

// Construct a mongo "projection document" from field paths.
// Always include the revision field.
func projectionDoc(fps [][]string) bson.D {
	proj := bson.D{{Key: docstore.RevisionField, Value: 1}}
	for _, fp := range fps {
		proj = append(proj, bson.E{Key: strings.Join(fp, "."), Value: 1})
	}
	return proj
}

func (c *collection) create(ctx context.Context, a *driver.Action) error {
	// See https://docs.mongodb.com/manual/reference/method/db.collection.insertOne
	id, err := c.docID(a.Doc)
	// If the user provides a function to get the ID rather than a field, then Create
	// can't make a unique ID for a document that's missing one, because it doesn't
	// know how to create it or where to store it in the original document.
	if err != nil && c.idField == "" {
		return err
	}
	mdoc, err := encodeDoc(a.Doc)
	if err != nil {
		return err
	}
	if id != nil {
		mdoc[mongoIDField] = id
	}
	mdoc[docstore.RevisionField] = driver.UniqueString()
	result, err := c.coll.InsertOne(ctx, mdoc)
	if err != nil {
		return err
	}
	if result.InsertedID == nil {
		return nil
	}
	// Here, c.idField must be non-empty. If it were empty, then the ID returned by
	// c.idFunc would be set in mdoc, so result.InsertedID would be nil.
	return a.Doc.SetField(c.idField, result.InsertedID)
}

func (c *collection) replace(ctx context.Context, a *driver.Action, upsert bool) error {
	// See https://docs.mongodb.com/manual/reference/method/db.collection.replaceOne
	doc, err := encodeDoc(a.Doc)
	if err != nil {
		return err
	}
	id, err := c.docID(a.Doc)
	if err != nil {
		return err
	}
	doc[mongoIDField] = id
	doc[docstore.RevisionField] = driver.UniqueString()
	opts := options.Replace()
	if upsert {
		opts.SetUpsert(true) // Document will be created if it doesn't exist.
	}
	filter, id, _, err := c.makeFilter(a.Doc)
	if err != nil {
		return err
	}
	result, err := c.coll.ReplaceOne(ctx, filter, doc, opts)
	if err != nil {
		return err
	}
	if !upsert && result.MatchedCount == 0 {
		return gcerr.Newf(gcerr.NotFound, nil, "document with ID %v does not exist", id)
	}
	return nil
}

func (c *collection) delete(ctx context.Context, a *driver.Action) error {
	filter, id, rev, err := c.makeFilter(a.Doc)
	if err != nil {
		return err
	}
	result, err := c.coll.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		if rev == nil {
			return nil
		}
		// Not sure if the document doesn't exist, or the revision is wrong. Distinguish the two.
		res := c.coll.FindOne(ctx, bson.D{{"_id", id}})
		if res.Err() != nil { // TODO(jba): distinguish between not found and other errors.
			return gcerr.Newf(gcerr.NotFound, res.Err(), "document with ID %v does not exist", id)
		}
		return gcerr.Newf(gcerr.FailedPrecondition, nil, "document with ID %v does not have revision %v", id, rev)
	}
	return err
}

func (c *collection) update(ctx context.Context, a *driver.Action) error {
	var (
		sets   bson.D
		unsets bson.D
	)
	for _, m := range a.Mods {
		key := strings.Join(m.FieldPath, ".")
		if m.Value == nil {
			unsets = append(unsets, bson.E{Key: key, Value: ""})
		} else {
			val, err := encodeValue(m.Value)
			if err != nil {
				return err
			}
			sets = append(sets, bson.E{Key: key, Value: val})
		}
	}
	if len(sets) == 0 && len(unsets) == 0 {
		// MongoDB returns an error if there are no updates, but docstore treats it
		// as a no-op.
		return nil
	}
	updateDoc := map[string]bson.D{}
	updateDoc["$set"] = append(sets, bson.E{Key: docstore.RevisionField, Value: driver.UniqueString()})
	if len(unsets) > 0 {
		updateDoc["$unset"] = unsets
	}
	filter, id, _, err := c.makeFilter(a.Doc)
	if err != nil {
		return err
	}
	result, err := c.coll.UpdateOne(ctx, filter, updateDoc)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return gcerr.Newf(gcerr.NotFound, nil, "document with ID %v does not exist", id)
	}
	return nil
}

func (c *collection) makeFilter(doc driver.Document) (filter bson.D, id, rev interface{}, err error) {
	id, err = c.docID(doc)
	if err != nil {
		return nil, nil, nil, err
	}
	rev, err = doc.GetField(docstore.RevisionField)
	if err != nil && gcerrors.Code(err) != gcerrors.NotFound {
		return nil, nil, nil, err
	}
	// Only select the document with the given ID.
	filter = bson.D{{"_id", id}}
	// If the given document has a revision, it must match the stored document.
	if rev != nil {
		filter = append(filter, bson.E{Key: docstore.RevisionField, Value: rev})
	}
	return filter, id, rev, nil
}

func (c *collection) docID(doc driver.Document) (interface{}, error) {
	if c.idField != "" {
		id, err := doc.GetField(c.idField)
		if err != nil {
			return nil, gcerr.New(gcerr.InvalidArgument, err, 2, "document missing ID")
		}
		return id, nil
	}
	id := c.idFunc(doc.Origin)
	if id == nil {
		return nil, gcerr.New(gcerr.InvalidArgument, nil, 2, "document missing ID")
	}
	return id, nil
}

// Error code for a write error when no documents match a filter.
// (The Go mongo driver doesn't define an exported constant for this.)
const mongoNotFoundCode = 11000

func (c *collection) ErrorCode(err error) gcerrors.ErrorCode {
	if g, ok := err.(*gcerr.Error); ok {
		return g.Code
	}
	if wexc, ok := err.(mongo.WriteException); ok && len(wexc.WriteErrors) > 0 {
		if wexc.WriteErrors[0].Code == mongoNotFoundCode {
			return gcerrors.NotFound
		}
	}
	return gcerrors.Unknown
}
