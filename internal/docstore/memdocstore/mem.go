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
//
// Every document in a memdocstore collection has a unique primary key. The primary
// key values need not be strings; they may be any comparable Go value.
//
//
// Unordered Action Lists
//
// Unordered action lists are executed concurrently. Each action in an unordered
// action list is executed in a separate goroutine.
//
//
// URLs
//
// For docstore.OpenCollection, memdocstore registers for the scheme
// "mem".
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
package memdocstore // import "gocloud.dev/internal/docstore/memdocstore"

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"

	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/driver"
	"gocloud.dev/internal/gcerr"
)

func init() {
	docstore.DefaultURLMux().RegisterCollection(Scheme, &URLOpener{})
}

// Scheme is the URL scheme memdocstore registers its URLOpener under on
// docstore.DefaultMux.
const Scheme = "mem"

// URLOpener opens URLs like "mem://_id".
//
// The URL's host is used as the keyField.
// The URL's path is ignored.
//
// No query parameters are supported.
type URLOpener struct{}

// OpenCollectionURL opens a docstore.Collection based on u.
func (*URLOpener) OpenCollectionURL(ctx context.Context, u *url.URL) (*docstore.Collection, error) {
	for param := range u.Query() {
		return nil, fmt.Errorf("open collection %v: invalid query parameter %q", u, param)
	}
	return OpenCollection(u.Host, nil)
}

// Options are optional arguments to the OpenCollection functions.
type Options struct{}

// TODO(jba): make this package thread-safe.

// OpenCollection creates a *docstore.Collection backed by memory. keyField is the
// document field holding the primary key of the collection.
func OpenCollection(keyField string, _ *Options) (*docstore.Collection, error) {
	c, err := newCollection(keyField, nil)
	if err != nil {
		return nil, err
	}
	return docstore.NewCollection(c), nil
}

// OpenCollectionWithKeyFunc creates a *docstore.Collection backed by memory. keyFunc takes
// a document and returns the document's primary key. It should return nil if the
// document is missing the information to construct a key. This will cause all
// actions, even Create, to fail.
func OpenCollectionWithKeyFunc(keyFunc func(docstore.Document) interface{}, _ *Options) (*docstore.Collection, error) {
	c, err := newCollection("", keyFunc)
	if err != nil {
		return nil, err
	}
	return docstore.NewCollection(c), nil
}

func newCollection(keyField string, keyFunc func(docstore.Document) interface{}) (driver.Collection, error) {
	if keyField == "" && keyFunc == nil {
		return nil, gcerr.Newf(gcerr.InvalidArgument, nil, "must provide either keyField or keyFunc")
	}
	return &collection{
		keyField:     keyField,
		keyFunc:      keyFunc,
		docs:         map[interface{}]map[string]interface{}{},
		nextRevision: 1,
	}, nil
}

type collection struct {
	keyField string
	keyFunc  func(docstore.Document) interface{}
	mu       sync.Mutex
	// map from keys to documents. Documents are represented as map[string]interface{},
	// regardless of what their original representation is. Even if the user is using
	// map[string]interface{}, we make our own copy.
	docs         map[interface{}]map[string]interface{}
	nextRevision int64 // incremented on each write
}

func (c *collection) Key(doc driver.Document) (interface{}, error) {
	if c.keyField != "" {
		key, _ := doc.GetField(c.keyField) // no error on missing key, and it will be nil
		return key, nil
	}
	key := c.keyFunc(doc.Origin)
	if key == nil {
		return nil, gcerr.Newf(gcerr.InvalidArgument, nil, "missing document key")
	}
	return key, nil
}

// ErrorCode implements driver.ErrorCode.
func (c *collection) ErrorCode(err error) gcerr.ErrorCode {
	return gcerrors.Code(err)
}

// RunActions implements driver.RunActions.
func (c *collection) RunActions(ctx context.Context, actions []*driver.Action, opts *driver.RunActionsOptions) driver.ActionListError {
	if opts.Unordered {
		errs := make([]error, len(actions))
		var wg sync.WaitGroup
		for i, a := range actions {
			i := i
			a := a
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs[i] = c.runAction(ctx, a)
			}()
		}
		wg.Wait()
		return driver.NewActionListError(errs)
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
	c.mu.Lock()
	defer c.mu.Unlock()
	// If there is a key, get the current document with that key.
	var (
		current map[string]interface{}
		exists  bool
	)
	if a.Key != nil {
		current, exists = c.docs[a.Key]
	}
	// Check for a NotFound error.
	if !exists && (a.Kind == driver.Replace || a.Kind == driver.Update || a.Kind == driver.Get) {
		return gcerr.Newf(gcerr.NotFound, nil, "document with key %v does not exist", a.Key)
	}
	switch a.Kind {
	case driver.Create:
		// It is an error to attempt to create an existing document.
		if exists {
			return gcerr.Newf(gcerr.AlreadyExists, nil, "Create: document with key %v exists", a.Key)
		}
		// If the user didn't supply a value for the key field, create a new one.
		if a.Key == nil {
			a.Key = driver.UniqueString()
			// Set the new key in the document.
			if err := a.Doc.SetField(c.keyField, a.Key); err != nil {
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
		c.docs[a.Key] = doc

	case driver.Delete:
		if err := checkRevision(a.Doc, current); err != nil {
			return err
		}
		delete(c.docs, a.Key)

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

// Must be called with the lock held.
func (c *collection) update(doc map[string]interface{}, mods []driver.Mod) error {
	// Sort mods by first field path element so tests are deterministic.
	sort.Slice(mods, func(i, j int) bool { return mods[i].FieldPath[0] < mods[j].FieldPath[0] })

	// To make update atomic, we first convert the actions into a form that can't
	// fail.
	type guaranteedMod struct {
		parentMap    map[string]interface{} // the map holding the key to be modified
		key          string
		encodedValue interface{} // the value after encoding
	}

	gmods := make([]guaranteedMod, len(mods))
	var err error
	for i, mod := range mods {
		gmod := &gmods[i]
		// Check that the field path is valid. That is, every component of the path
		// but the last refers to a map, and no component along the way is nil.
		if gmod.parentMap, err = getParentMap(doc, mod.FieldPath, false); err != nil {
			return err
		}
		gmod.key = mod.FieldPath[len(mod.FieldPath)-1]
		if inc, ok := mod.Value.(driver.IncOp); ok {
			amt, err := encodeValue(inc.Amount)
			if err != nil {
				return err
			}
			if gmod.encodedValue, err = add(gmod.parentMap[gmod.key], amt); err != nil {
				return err
			}
		} else if mod.Value != nil {
			// Make sure the value encodes successfully.
			if gmod.encodedValue, err = encodeValue(mod.Value); err != nil {
				return err
			}
		}
	}
	// Now execute the guaranteed mods.
	for _, m := range gmods {
		if m.encodedValue == nil {
			delete(m.parentMap, m.key)
		} else {
			m.parentMap[m.key] = m.encodedValue
		}
	}
	if len(mods) > 0 {
		c.changeRevision(doc)
	}
	return nil
}

// Add two encoded numbers.
// Since they're encoded, they are either int64 or float64.
// Allow adding a float to an int, producing a float.
// TODO(jba): see how other providers handle that.
func add(x, y interface{}) (interface{}, error) {
	if x == nil {
		return y, nil
	}
	switch x := x.(type) {
	case int64:
		switch y := y.(type) {
		case int64:
			return x + y, nil
		case float64:
			return float64(x) + y, nil
		default:
			// This shouldn't happen because it should be checked by docstore.
			panic("bad increment amount")
		}
	case float64:
		switch y := y.(type) {
		case int64:
			return x + float64(y), nil
		case float64:
			return x + y, nil
		default:
			panic("bad increment amount")
		}
	default:
		return nil, gcerr.Newf(gcerr.InvalidArgument, nil, "value %v being incremented not int64 or float64", x)
	}
}

// Must be called with the lock held.
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

// As implements driver.As.
func (c *collection) As(i interface{}) bool { return false }
