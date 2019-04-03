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

package docstore

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"gocloud.dev/internal/docstore/driver"
	"gocloud.dev/internal/gcerr"
)

// A Document is a set of field-value pairs. One or more fields, called the key
// fields, must uniquely identify the document in the collection. You specify the key
// fields when you open a provider collection.
// A field name must be a valid UTF-8 string that does not contain a '.'.
//
// A Document can be represented as a map[string]int or a pointer to a struct. For
// structs, the exported fields are the document fields.
type Document = interface{}

// A Collection is a set of documents.
// TODO(jba): make the docstring look more like blob.Bucket.
type Collection struct {
	driver driver.Collection
}

// NewCollection is intended for use by provider implementations.
var NewCollection = newCollection

// newCollection makes a Collection.
func newCollection(d driver.Collection) *Collection {
	return &Collection{driver: d}
}

// RevisionField is the name of the document field used for document revision
// information, to implement optimistic locking.
// See the Revisions section of the package documentation.
const RevisionField = "DocstoreRevision"

// A FieldPath is a dot-separated sequence of UTF-8 field names. Examples:
//   room
//   room.size
//   room.size.width
//
// A FieldPath can be used select top-level fields or elements of sub-documents.
// There is no way to select a single list element.
type FieldPath string

// Actions returns an ActionList that can be used to perform
// actions on the collection's documents.
func (c *Collection) Actions() *ActionList {
	return &ActionList{coll: c}
}

// An ActionList is a group of actions that affect a single collection.
//
// By default, the actions in the list are performed in order from the point of view
// of the client. However, the actions may not be performed atomically, and there is
// no guarantee that a Get following a write will see the value just written (for
// example, if the provider is eventually consistent). Execution stops with the first
// failed action.
//
// If the Unordered method is called on an ActionList, then the actions may be
// executed in any order, perhaps concurrently. All actions will be executed, even if
// some fail.
type ActionList struct {
	coll      *Collection
	actions   []*Action
	unordered bool
}

// An Action is a read or write on a single document.
// Use the methods of ActionList to create and execute Actions.
type Action struct {
	kind       driver.ActionKind
	doc        Document
	fieldpaths []FieldPath // paths to retrieve, for Get
	mods       Mods        // modifications to make, for Update
}

func (l *ActionList) add(a *Action) *ActionList {
	l.actions = append(l.actions, a)
	return l
}

// Create adds an action that creates a new document.
// The document must not already exist; an error for which gcerrors.Code returns
// AlreadyExists is returned if it does. If the document doesn't have key fields, it
// will be given key fields with unique values.
// TODO(jba): treat zero values for struct fields as not present?
func (l *ActionList) Create(doc Document) *ActionList {
	return l.add(&Action{kind: driver.Create, doc: doc})
}

// Replace adds an action that replaces a document.
// The key fields must be set.
// The document must already exist; an error for which gcerrors.Code returns NotFound
// is returned if it does not.
// See the Revisions section of the package documentation for how revisions are
// handled.
func (l *ActionList) Replace(doc Document) *ActionList {
	return l.add(&Action{kind: driver.Replace, doc: doc})
}

// Put adds an action that adds or replaces a document.
// The key fields must be set.
// The document may or may not already exist.
// See the Revisions section of the package documentation for how revisions are
// handled.
func (l *ActionList) Put(doc Document) *ActionList {
	return l.add(&Action{kind: driver.Put, doc: doc})
}

// Delete adds an action that deletes a document.
// Only the key fields and RevisionField of doc are used.
// See the Revisions section of the package documentation for how revisions are
// handled.
// If doc has no revision and the document doesn't exist, nothing happens and no
// error is returned.
func (l *ActionList) Delete(doc Document) *ActionList {
	// Rationale for not returning an error if the document does not exist:
	// Returning an error might be informative and could be ignored, but if the
	// semantics of an action list are to stop at first error, then we might abort a
	// list of Deletes just because one of the docs was not present, and that seems
	// wrong, or at least something you'd want to turn off.
	return l.add(&Action{kind: driver.Delete, doc: doc})
}

// Get adds an action that retrieves a document. Only the key fields of doc are used.
// If fps is omitted, doc will contain all the fields of the retrieved document. If
// fps is present, only the given field paths are retrieved, in addition to the
// revision field. It is undefined whether other fields of doc at the time of the
// call are removed, unchanged, or zeroed, so for portable behavior doc should
// contain only the key fields.
func (l *ActionList) Get(doc Document, fps ...FieldPath) *ActionList {
	return l.add(&Action{
		kind:       driver.Get,
		doc:        doc,
		fieldpaths: fps,
	})
}

// Update atomically applies Mods to doc, which must exist.
// Only the key and revision fields of doc are used.
//
// A modification will create a field if it doesn't exist.
//
// No field path in mods can be a prefix of another. (It makes no sense
// to, say, set foo but increment foo.bar.)
//
// See the Revisions section of the package documentation for how revisions are
// handled.
//
// It is undefined whether updating a sub-field of a non-map field will succeed.
// For instance, if the current document is {a: 1} and Update is called with the
// mod "a.b": 2, then either Update will fail, or it will succeed with the result
// {a: {b: 2}}.
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

// After Unordered is called, Do may execute the actions in any order.
// All actions will be executed, even if some fail.
func (l *ActionList) Unordered() *ActionList {
	l.unordered = true
	return l
}

// Mods is a map from field paths to modifications.
// At present, a modification is one of:
// - nil, to delete the field
// - any other value, to set the field to that value
// TODO(jba): add other kinds of modification
// See ActionList.Update.
type Mods map[FieldPath]interface{}

// An ActionListError is returned by ActionList.Do. It contains all the errors
// encountered while executing the ActionList, and the positions of the corresponding
// actions.
type ActionListError []struct {
	Index int
	Err   error
}

// TODO(jba): use xerrors formatting.

func (e ActionListError) Error() string {
	var s []string
	for _, x := range e {
		s = append(s, fmt.Sprintf("at %d: %v", x.Index, x.Err))
	}
	return strings.Join(s, "; ")
}

// Unwrap returns the error in e, if there is exactly one. If there is more than one
// error, Unwrap returns nil, since there is no way to determine which should be
// returned.
func (e ActionListError) Unwrap() error {
	if len(e) == 1 {
		return e[0].Err
	}
	// Return nil when e is nil, or has more than one error.
	// When there are multiple errors, it doesn't make sense to return any of them.
	return nil
}

// Do executes the action list.
//
// If Do returns a non-nil error, it will be of type ActionListError. If any action
// fails, the returned error will contain the position in the ActionList of each
// failed action. As a special case, none of the actions will be executed if any is
// invalid (for example, a Put whose document is missing its key field).
//
// In ordered mode (when the Unordered method was not called on the list), execution
// will stop after the first action that fails.
//
// In unordered mode, all the actions will be executed.
func (l *ActionList) Do(ctx context.Context) error {
	var das []*driver.Action
	var alerr ActionListError
	for i, a := range l.actions {
		d, err := a.toDriverAction()
		if err != nil {
			alerr = append(alerr, struct {
				Index int
				Err   error
			}{i, wrapError(l.coll.driver, err)})
		} else {
			das = append(das, d)
		}
	}
	if len(alerr) > 0 {
		return alerr
	}
	alerr = ActionListError(l.coll.driver.RunActions(ctx, das, l.unordered))
	if len(alerr) == 0 {
		return nil // Explicitly return nil, because alerr is not of type error.
	}
	for i := range alerr {
		alerr[i].Err = wrapError(l.coll.driver, alerr[i].Err)
	}
	return alerr
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
		// Sort keys so tests are deterministic.
		var keys []string
		for k := range a.mods {
			keys = append(keys, string(k))
		}
		sort.Strings(keys)
		for _, k := range keys {
			k := FieldPath(k)
			v := a.mods[k]
			fp, err := parseFieldPath(k)
			if err != nil {
				return nil, err
			}
			d.Mods = append(d.Mods, driver.Mod{FieldPath: fp, Value: v})
		}
	}
	return d, nil
}

// Create is a convenience for building and running a single-element action list.
// See ActionList.Create.
func (c *Collection) Create(ctx context.Context, doc Document) error {
	if err := c.Actions().Create(doc).Do(ctx); err != nil {
		return err.(ActionListError).Unwrap()
	}
	return nil
}

// Replace is a convenience for building and running a single-element action list.
// See ActionList.Replace.
func (c *Collection) Replace(ctx context.Context, doc Document) error {
	if err := c.Actions().Replace(doc).Do(ctx); err != nil {
		return err.(ActionListError).Unwrap()
	}
	return nil
}

// Put is a convenience for building and running a single-element action list.
// See ActionList.Put.
func (c *Collection) Put(ctx context.Context, doc Document) error {
	if err := c.Actions().Put(doc).Do(ctx); err != nil {
		return err.(ActionListError).Unwrap()
	}
	return nil
}

// Delete is a convenience for building and running a single-element action list.
// See ActionList.Delete.
func (c *Collection) Delete(ctx context.Context, doc Document) error {
	if err := c.Actions().Delete(doc).Do(ctx); err != nil {
		return err.(ActionListError).Unwrap()
	}
	return nil
}

// Get is a convenience for building and running a single-element action list.
// See ActionList.Get.
func (c *Collection) Get(ctx context.Context, doc Document, fps ...FieldPath) error {
	if err := c.Actions().Get(doc, fps...).Do(ctx); err != nil {
		return err.(ActionListError).Unwrap()
	}
	return nil
}

// Update is a convenience for building and running a single-element action list.
// See ActionList.Update.
func (c *Collection) Update(ctx context.Context, doc Document, mods Mods) error {
	if err := c.Actions().Update(doc, mods).Do(ctx); err != nil {
		return err.(ActionListError).Unwrap()
	}
	return nil
}

func parseFieldPath(fp FieldPath) ([]string, error) {
	if len(fp) == 0 {
		return nil, gcerr.Newf(gcerr.InvalidArgument, nil, "empty field path")
	}
	if !utf8.ValidString(string(fp)) {
		return nil, gcerr.Newf(gcerr.InvalidArgument, nil, "invalid UTF-8 field path %q", fp)
	}
	parts := strings.Split(string(fp), ".")
	for _, p := range parts {
		if p == "" {
			return nil, gcerr.Newf(gcerr.InvalidArgument, nil, "empty component in field path %q", fp)
		}
	}
	return parts, nil
}

func wrapError(c driver.Collection, err error) error {
	if err == nil {
		return nil
	}
	if gcerr.DoNotWrap(err) {
		return err
	}
	if _, ok := err.(*gcerr.Error); ok {
		return err
	}
	return gcerr.New(c.ErrorCode(err), err, 2, "docstore")
}

// TODO(jba): ErrorAs
