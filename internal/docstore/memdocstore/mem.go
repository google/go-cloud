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

// Package memdocstore provides an in-memory implementation of the docstore
// API. It is suitable for local development and testing.
package memdocstore // import "gocloud.dev/internal/docstore/memdocstore"

import (
	"context"
	"errors"
	"sort"
	"strings"

	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/driver"
	"gocloud.dev/internal/gcerr"
)

// TODO(jba): make this package thread-safe.

// Options sets options for constructing a *docstore.Collection backed by memory.
type Options struct{}

// OpenCollection creates a *docstore.Collection backed by memory.
// memdocstore requires that a single field be designated the primary
// key. Its values must be unique over all documents in the collection,
// and the primary key must be provided to retrieve a document.
// The values need not be strings; they may be any comparable Go value.
// keyField is the name of the primary key field.
func OpenCollection(keyField string, opts *Options) *docstore.Collection {
	return docstore.NewCollection(newCollection(keyField))
}

func newCollection(keyField string) driver.Collection {
	return &collection{
		keyField:     keyField,
		docs:         map[interface{}]map[string]interface{}{},
		nextRevision: 1,
	}
}

type collection struct {
	keyField string
	// map from keys to documents. Documents are represented as map[string]interface{},
	// regardless of what their original representation is. Even if the user is using
	// map[string]interface{}, we make our own copy.
	docs         map[interface{}]map[string]interface{}
	nextRevision int64 // incremented on each write
}

// ErrorCode implements driver.ErrorCode.
func (c *collection) ErrorCode(err error) gcerr.ErrorCode {
	return gcerrors.Code(err)
}

// RunActions implements driver.RunActions.
func (c *collection) RunActions(ctx context.Context, actions []*driver.Action, unordered bool) driver.ActionListError {
	if unordered {
		panic("unordered unimplemented")
	}
	// Run each action in order, stopping at the first error.
	for i, a := range actions {
		if err := c.runAction(ctx, a); err != nil {
			return driver.ActionListError{{i, err}}
		}
	}
	return nil
}

// runAction executes a single action.
func (c *collection) runAction(ctx context.Context, a *driver.Action) error {
	// Stop if the context is done.
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// Get the key from the doc so we can look it up in the map.
	key, err := a.Doc.GetField(c.keyField)
	// The only acceptable error case is NotFound during a Create.
	if err != nil && !(gcerrors.Code(err) == gcerr.NotFound && a.Kind == driver.Create) {
		return err
	}
	// If there is a key, get the current document.
	var (
		current map[string]interface{}
		exists  bool
	)
	if err == nil {
		current, exists = c.docs[key]
	}
	// Check for a NotFound error.
	if !exists && (a.Kind == driver.Replace || a.Kind == driver.Update || a.Kind == driver.Get) {
		return gcerr.Newf(gcerr.NotFound, nil, "document with key %v does not exist", key)
	}
	switch a.Kind {
	case driver.Create:
		// It is an error to attempt to create an existing document.
		if exists {
			return gcerr.Newf(gcerr.AlreadyExists, nil, "Create: document with key %v exists", key)
		}
		// If the user didn't supply a value for the key field, create a new one.
		if key == nil {
			key = driver.UniqueString()
			// Set the new key in the document.
			if err := a.Doc.SetField(c.keyField, key); err != nil {
				return gcerr.Newf(gcerr.InvalidArgument, nil, "cannot set key field %q", c.keyField)
			}
		}
		fallthrough

	case driver.Replace, driver.Put:
		if err := checkRevision(a.Doc, current); err != nil {
			return err
		}
		doc, err := encodeDoc(a.Doc)
		if err != nil {
			return err
		}
		c.changeRevision(doc)
		c.docs[key] = doc

	case driver.Delete:
		if err := checkRevision(a.Doc, current); err != nil {
			return err
		}
		delete(c.docs, key)

	case driver.Update:
		if err := checkRevision(a.Doc, current); err != nil {
			return err
		}
		if err := c.update(current, a.Mods); err != nil {
			return err
		}

	case driver.Get:
		// We've already retrieved the document into current, above.
		// Now we copy its fields into the user-provided document.
		if err := decodeDoc(current, a.Doc, a.FieldPaths); err != nil {
			return err
		}
	default:
		return gcerr.Newf(gcerr.Internal, nil, "unknown kind %v", a.Kind)
	}
	return nil
}

func (c *collection) update(doc map[string]interface{}, mods []driver.Mod) error {
	// Apply each modification. Fail if any mod would fail.
	// Sort mods by first field path element so tests are deterministic.
	sort.Slice(mods, func(i, j int) bool { return mods[i].FieldPath[0] < mods[j].FieldPath[0] })

	// Check first that every field path is valid. That is, whether every component
	// of the path but the last refers to a map, and no component along the way is
	// nil. If that check succeeds, all actions will succeed, making update atomic.
	for _, m := range mods {
		if _, err := getParentMap(doc, m.FieldPath, false); err != nil {
			return err
		}
	}
	for _, m := range mods {
		if m.Value == nil {
			deleteAtFieldPath(doc, m.FieldPath)
		} else {
			// This can't fail because we checked it above.
			_ = setAtFieldPath(doc, m.FieldPath, m.Value)
		}
	}
	if len(mods) > 0 {
		c.changeRevision(doc)
	}
	return nil
}

func (c *collection) changeRevision(doc map[string]interface{}) {
	c.nextRevision++
	doc[docstore.RevisionField] = c.nextRevision
}

func checkRevision(arg driver.Document, current map[string]interface{}) error {
	if current == nil {
		return nil // no existing document
	}
	curRev := current[docstore.RevisionField].(int64)
	r, err := arg.GetField(docstore.RevisionField)
	if err != nil || r == nil {
		return nil // no incoming revision information: nothing to check
	}
	wantRev, ok := r.(int64)
	if !ok {
		return gcerr.Newf(gcerr.InvalidArgument, nil, "revision field %s is not an int64", docstore.RevisionField)
	}
	if wantRev != curRev {
		return gcerr.Newf(gcerr.FailedPrecondition, nil, "mismatched revisions: want %d, current %d", wantRev, curRev)
	}
	return nil
}

// getAtFieldPath gets the value of m at fp. It returns an error if fp is invalid
// (see getParentMap).
func getAtFieldPath(m map[string]interface{}, fp []string) (interface{}, error) {
	m2, err := getParentMap(m, fp, false)
	if err != nil {
		return nil, err
	}
	return m2[fp[len(fp)-1]], nil
}

// setAtFieldPath sets m's value at fp to val. It creates intermediate maps as
// needed. It returns an error if a non-final component of fp does not denote a map.
func setAtFieldPath(m map[string]interface{}, fp []string, val interface{}) error {
	m2, err := getParentMap(m, fp, true)
	if err != nil {
		return err
	}
	m2[fp[len(fp)-1]] = val
	return nil
}

// Delete the value from m at the given field path, if it exists.
func deleteAtFieldPath(m map[string]interface{}, fp []string) {
	m2, _ := getParentMap(m, fp, false) // ignore error
	if m2 != nil {
		delete(m2, fp[len(fp)-1])
	}
}

// getParentMap returns the map that directly contains the given field path;
// that is, the value of m at the field path that excludes the last component
// of fp. If a non-map is encountered along the way, an InvalidArgument error is
// returned. If nil is encountered, nil is returned unless create is true, in
// which case a map is added at that point.
func getParentMap(m map[string]interface{}, fp []string, create bool) (map[string]interface{}, error) {
	var ok bool
	for _, k := range fp[:len(fp)-1] {
		if m[k] == nil {
			if !create {
				return nil, nil
			}
			m[k] = map[string]interface{}{}
		}
		m, ok = m[k].(map[string]interface{})
		if !ok {
			return nil, gcerr.Newf(gcerr.InvalidArgument, nil, "invalid field path %q at %q", strings.Join(fp, "."), k)
		}
	}
	return m, nil
}

func (*collection) RunGetQuery(context.Context, *driver.Query) (driver.DocumentIterator, error) {
	return errors.New("unimp")
}
