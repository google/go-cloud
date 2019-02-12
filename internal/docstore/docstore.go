// Copyright 2019 The Go Cloud Authors
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

// Package docstore provides a portable implementation of a document store.
package docstore // import "gocloud.dev/internal/docstore"

import (
	"context"
	"strings"
	"unicode/utf8"

	"gocloud.dev/internal/docstore/driver"
	"gocloud.dev/internal/gcerr"
)

// A Document is a set of field-value pairs.
// It can be represented as a map[string]int or a pointer to a struct.
// For structs, the exported fields are the document fields.
type Document = interface{}

// A Collection is a set of documents.
type Collection struct {
	driver driver.Collection
}

// NewCollection is intended for use by provider implementations.
var NewCollection = newCollection

// newCollection makes a Collection.
func newCollection(d driver.Collection) *Collection {
	return &Collection{driver: d}
}

// Actions returns an ActionList that can be used to perform
// actions on the collection's documents.
func (c *Collection) Actions() *ActionList {
	return &ActionList{coll: c}
}

// An ActionList is a sequence of actions that affect a single collection.
type ActionList struct {
	coll    *Collection
	actions []*Action
}

// An Action is a read or write on a single document.
// Use the methods of ActionList to create and execute Actions.
type Action struct {
	kind       driver.ActionKind
	doc        Document
	fieldpaths []string // paths to retrieve, for Get
	mods       Mods     // modifications to make, for Update
}

func (l *ActionList) add(a *Action) *ActionList {
	l.actions = append(l.actions, a)
	return l
}

// Create adds an action that creates a new document.
// The document must not already exist; an AlreadyExists error is returned if it
// does. If the document doesn't have key fields, it will be given key fields with
// unique values.
// TODO(jba): treat zero values for struct fields as not present?
func (l *ActionList) Create(doc Document) *ActionList {
	return l.add(&Action{kind: driver.Create, doc: doc})
}

// Replace adds an action that replaces a document.
// The document must already exist; a NotFound error is returned if it does not.
func (l *ActionList) Replace(doc Document) *ActionList {
	return l.add(&Action{kind: driver.Replace, doc: doc})
}

// Put adds an action that adds or replaces a document.
// The document may or may not already exist.
func (l *ActionList) Put(doc Document) *ActionList {
	return l.add(&Action{kind: driver.Put, doc: doc})
}

// Delete adds an action that deletes a document.
// Only the key fields and RevisionField of doc are used.
// If the document doesn't exist, nothing happens and no error is returned.
func (l *ActionList) Delete(doc Document) *ActionList {
	// Rationale for not returning an error if the document does not exist:
	// Returning an error might be informative and could be ignored, but if the
	// semantics of an action list are to stop at first error, then we might abort a
	// list of Deletes just because one of the docs was not present, and that seems
	// wrong, or at least something you'd want to turn off.
	return l.add(&Action{kind: driver.Delete, doc: doc})
}

// Get adds an action that retrieves a document.
// Only the key fields of doc are used to create the request.
// If fps is omitted, all the fields of doc are set to those of the
// retrieved document. If fps is present, only the given field paths are
// set. In both cases, other fields of doc are not touched.
//
// A field path is a dot-separated sequence of UTF-8 field names. A field path
// can select top level fields or elements of sub-documents. There is no way to
// select a single list element.
func (l *ActionList) Get(doc Document, fps ...string) *ActionList {
	return l.add(&Action{
		kind:       driver.Get,
		doc:        doc,
		fieldpaths: fps,
	})
}

// Update applies Mods to doc, which must exist.
// Only the key and revision fields of doc are used.
//
// A modification will create a field if it doesn't exist.
//
// No field path in mods can be a prefix of another. (It makes no sense
// to, say, set foo but increment foo.bar.)
//
// Update does not modify its doc argument. To obtain the new value of the document,
// call Get after calling Update.
func (l *ActionList) Update(doc Document, mods Mods) *ActionList {
	return l.add(&Action{
		kind: driver.Update,
		doc:  doc,
		mods: mods,
	})
}

// Mods is a map from field paths to modifications.
// A field path is a dot-separated sequence of UTF-8 field names.
// See ActionList.Update.
type Mods map[string]interface{}

// Do executes the action list. If all the actions executed successfully, Do returns
// (number of actions, nil). If any failed, Do returns the number of successful
// actions and an error. In general there is no way to know which actions succeeded,
// but the error will contain as much information as possible about the failures.
func (l *ActionList) Do(ctx context.Context) (int, error) {
	var das []*driver.Action
	for _, a := range l.actions {
		d, err := a.toDriverAction()
		if err != nil {
			return 0, wrapError(l.coll.driver, err)
		}
		das = append(das, d)
	}
	n, err := l.coll.driver.RunActions(ctx, das)
	return n, wrapError(l.coll.driver, err)
}

func (a *Action) toDriverAction() (*driver.Action, error) {
	ddoc, err := driver.NewDocument(a.doc)
	if err != nil {
		return nil, err
	}
	d := &driver.Action{Kind: a.kind, Doc: ddoc}
	if a.fieldpaths != nil {
		d.FieldPaths = make([][]string, len(a.fieldpaths))
		for i, s := range a.fieldpaths {
			fp, err := parseFieldPath(s)
			if err != nil {
				return nil, err
			}
			d.FieldPaths[i] = fp
		}
	}
	if a.mods != nil {
		// Convert mods from a map to a slice of (fieldPath, value) pairs.
		// The map is easier for users to write, but the slice is easier
		// to process.
		// TODO(jba): check for prefix
		for k, v := range a.mods {
			fp, err := parseFieldPath(k)
			if err != nil {
				return nil, err
			}
			d.Mods = append(d.Mods, driver.Mod{fp, v})
		}
	}
	return d, nil
}

func parseFieldPath(s string) ([]string, error) {
	if !utf8.ValidString(s) {
		return nil, gcerr.Newf(gcerr.InvalidArgument, nil, "invalid UTF-8 field path %q", s)
	}
	fp := strings.Split(s, ".")
	for _, c := range fp {
		if c == "" {
			return nil, gcerr.Newf(gcerr.InvalidArgument, nil, "empty component in field path %q", s)
		}
	}
	return fp, nil
}

func wrapError(c driver.Collection, err error) error {
	if err == nil {
		return nil
	}
	if gcerr.DoNotWrap(err) {
		return err
	}
	return gcerr.New(c.ErrorCode(err), err, 2, "docstore")
}

// TODO(jba): ErrorAs
