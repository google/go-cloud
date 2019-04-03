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
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/driver"
	"gocloud.dev/internal/gcerr"
)

type collection struct {
	coll *mongo.Collection
}

type Options struct {
}

// OpenCollection opens a MongoDB collection for use with Docstore.
func OpenCollection(mcoll *mongo.Collection, _ *Options) *docstore.Collection {
	return docstore.NewCollection(newCollection(mcoll))
}

func newCollection(mcoll *mongo.Collection) *collection {
	return &collection{coll: mcoll}
}

// From https://docs.mongodb.com/manual/core/document: "The field name _id is
// reserved for use as a primary key; its value must be unique in the collection, is
// immutable, and may be of any type other than an array."
const idField = "_id"

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
	id, err := a.Doc.GetField(idField)
	if err != nil {
		return err
	}
	opts := options.FindOne()
	if len(a.FieldPaths) > 0 {
		// Create a "projection document" that specifies the fields to retrieve.
		// Always get the revision field.
		proj := bson.D{{Key: docstore.RevisionField, Value: 1}}
		for _, fp := range a.FieldPaths {
			proj = append(proj, bson.E{Key: strings.Join(fp, "."), Value: 1})
		}
		opts.Projection = proj
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

func (c *collection) create(ctx context.Context, a *driver.Action) error {
	// See https://docs.mongodb.com/manual/reference/method/db.collection.insertOne
	mdoc, err := encodeDoc(a.Doc)
	if err != nil {
		return err
	}
	mdoc[docstore.RevisionField] = driver.UniqueString()
	result, err := c.coll.InsertOne(ctx, mdoc)
	if err != nil {
		return err
	}
	if result.InsertedID == nil {
		return nil
	}
	return a.Doc.SetField(idField, result.InsertedID)
}

func (c *collection) replace(ctx context.Context, a *driver.Action, upsert bool) error {
	// See https://docs.mongodb.com/manual/reference/method/db.collection.replaceOne
	doc, err := encodeDoc(a.Doc)
	if err != nil {
		return err
	}
	doc[docstore.RevisionField] = driver.UniqueString()
	opts := options.Replace()
	if upsert {
		opts.SetUpsert(true) // Document will be created if it doesn't exist.
	}
	filter, id, _, err := makeFilter(a.Doc)
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
	filter, id, rev, err := makeFilter(a.Doc)
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
	filter, id, _, err := makeFilter(a.Doc)
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

func makeFilter(doc driver.Document) (filter bson.D, id, rev interface{}, err error) {
	id, err = doc.GetField(idField)
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

func (c *collection) RunQuery(context.Context, *driver.Query) error {
	return errors.New("unimp")
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
