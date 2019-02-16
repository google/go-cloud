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

	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/driver"
	"gocloud.dev/internal/gcerr"
)

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
		keyField: keyField,
		docs:     map[interface{}]map[string]interface{}{},
	}
}

type collection struct {
	keyField string
	// map from keys to documents. Documents are represented as map[string]interface{},
	// regardless of what their original representation is. Even if the user is using
	// map[string]interface{}, we make our own copy.
	docs map[interface{}]map[string]interface{}
}

// RunActions implements driver.RunActions.
func (c *collection) RunActions(ctx context.Context, actions []*driver.Action) (int, error) {
	// Stop immediately if the context is done.
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	// Run each action in order, stopping at the first error.
	for i, a := range actions {
		if err := c.runAction(a); err != nil {
			return i, err
		}
	}
	return len(actions), nil
}

// runAction executes a single action.
func (c *collection) runAction(a *driver.Action) error {
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
		doc, err := encodeDoc(a.Doc)
		if err != nil {
			return err
		}
		c.docs[key] = doc

	case driver.Delete:
		delete(c.docs, key)

	case driver.Update:
		if err := c.update(current, a.Mods); err != nil {
			return err
		}

	case driver.Get:
		// We've already retrieved the document into current, above.
		// Now we copy its fields into the user-provided document.
		// TODO(jba): support field paths.
		if err := decodeDoc(a.Doc, current); err != nil {
			return err
		}
	default:
		return gcerr.Newf(gcerr.Internal, nil, "unknown kind %v", a.Kind)
	}
	return nil
}

func (c *collection) update(doc map[string]interface{}, mods []driver.Mod) error {
	// Apply each modification in order. Fail on the first error.
	// TODO(jba): this should be atomic.
	ddoc, err := driver.NewDocument(doc)
	if err != nil {
		return err
	}
	for _, m := range mods {
		if m.Value == nil {
			deleteAtFieldPath(doc, m.FieldPath)
		} else if err := ddoc.Set(m.FieldPath, m.Value); err != nil {
			return err
		}
	}
	return nil
}

// Delete the value from m at the given field path.
func deleteAtFieldPath(m map[string]interface{}, fp []string) {
	if len(fp) == 1 {
		delete(m, fp[0])
	} else if m2, ok := m[fp[0]].(map[string]interface{}); ok {
		deleteAtFieldPath(m2, fp[1:])
	}
	// Otherwise do nothing.
}

// ErrorCode implements driver.ErrorCOde.
func (c *collection) ErrorCode(err error) gcerr.ErrorCode {
	return gcerrors.Code(err)
}
