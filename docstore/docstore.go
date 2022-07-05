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
	"encoding/base64"
	"fmt"
	"log"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"gocloud.dev/docstore/driver"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/internal/oc"
)

// A Document is a set of field-value pairs. One or more fields, called the key
// fields, must uniquely identify the document in the collection. You specify the key
// fields when you open a collection.
// A field name must be a valid UTF-8 string that does not contain a '.'.
//
// A Document can be represented as a map[string]int or a pointer to a struct. For
// structs, the exported fields are the document fields.
type Document = interface{}

// A Collection represents a set of documents. It provides an easy and portable
// way to interact with document stores.
// To create a Collection, use constructors found in driver subpackages.
type Collection struct {
	driver driver.Collection
	tracer *oc.Tracer
	mu     sync.Mutex
	closed bool
}

const pkgName = "gocloud.dev/docstore"

var (
	latencyMeasure = oc.LatencyMeasure(pkgName)

	// OpenCensusViews are predefined views for OpenCensus metrics.
	// The views include counts and latency distributions for API method calls.
	// See the example at https://godoc.org/go.opencensus.io/stats/view for usage.
	OpenCensusViews = oc.Views(pkgName, latencyMeasure)
)

// NewCollection is intended for use by drivers only. Do not use in application code.
var NewCollection = newCollection

// newCollection makes a Collection.
func newCollection(d driver.Collection) *Collection {
	c := &Collection{
		driver: d,
		tracer: &oc.Tracer{
			Package:        pkgName,
			Provider:       oc.ProviderName(d),
			LatencyMeasure: latencyMeasure,
		},
	}
	_, file, lineno, ok := runtime.Caller(1)
	runtime.SetFinalizer(c, func(c *Collection) {
		c.mu.Lock()
		closed := c.closed
		c.mu.Unlock()
		if !closed {
			var caller string
			if ok {
				caller = fmt.Sprintf(" (%s:%d)", file, lineno)
			}
			log.Printf("A docstore.Collection was never closed%s", caller)
		}
	})
	return c
}

// DefaultRevisionField is the default name of the document field used for document revision
// information, to implement optimistic locking.
// See the Revisions section of the package documentation.
const DefaultRevisionField = "DocstoreRevision"

func (c *Collection) revisionField() string {
	if r := c.driver.RevisionField(); r != "" {
		return r
	}
	return DefaultRevisionField
}

// A FieldPath is a dot-separated sequence of UTF-8 field names. Examples:
//
//	room
//	room.size
//	room.size.width
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
// The writes in an action list (Put, Create, Replace, Update and Delete actions)
// must refer to distinct documents and are unordered with respect to each other.
// Each write happens independently of the others: all actions will be executed, even
// if some fail.
//
// The Gets in an action list must also refer to distinct documents and are unordered
// and independent of each other.
//
// A Get and a write may refer to the same document. Each write may be paired with
// only one Get in this way. The Get and write will be executed in the order
// specified in the list: a Get before a write will see the old value of the
// document; a Get after the write will see the new value if the service is strongly
// consistent, but may see the old value if the service is eventually consistent.
type ActionList struct {
	coll     *Collection
	actions  []*Action
	beforeDo func(asFunc func(interface{}) bool) error
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

// Create adds an action that creates a new document to the given ActionList, and
// returns the ActionList. The document must not already exist; an error with code
// AlreadyExists is returned if it does. (See gocloud.dev/gcerrors for more on error
// codes.)
//
// If the document doesn't have key fields, or the key fields are empty, meaning
// 0, a nil interface value, or any empty array or string, key fields with
// unique values will be created and doc will be populated with them if there is
// a way to assign those keys, see each driver for details on the requirement of
// generating keys.
//
// The revision field of the document must be absent or nil.
//
// Except for setting the revision field and possibly setting the key fields, the doc
// argument is not modified.
func (l *ActionList) Create(doc Document) *ActionList {
	return l.add(&Action{kind: driver.Create, doc: doc})
}

// Replace adds an action that replaces a document to the given ActionList, and
// returns the ActionList. The key fields of the doc argument must be set. The
// document must already exist; an error with code NotFound is returned if it does
// not (or possibly FailedPrecondition, if the doc argument has a non-nil revision).
// (See gocloud.dev/gcerrors for more on error codes.)
//
// See the Revisions section of the package documentation for how revisions are
// handled.
func (l *ActionList) Replace(doc Document) *ActionList {
	return l.add(&Action{kind: driver.Replace, doc: doc})
}

// Put adds an action that adds or replaces a document to the given ActionList, and returns the ActionList.
// The key fields must be set.
//
// If the revision field is non-nil, then Put behaves exactly like Replace, returning
// an error if the document does not exist. Otherwise, Put will create the document
// if it does not exist.
//
// See the Revisions section of the package documentation for how revisions are
// handled.
func (l *ActionList) Put(doc Document) *ActionList {
	return l.add(&Action{kind: driver.Put, doc: doc})
}

// Delete adds an action that deletes a document to the given ActionList, and returns
// the ActionList. Only the key and revision fields of doc are used.
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

// Get adds an action that retrieves a document to the given ActionList, and
// returns the ActionList.
// Only the key fields of doc are used.
// If fps is omitted, doc will contain all the fields of the retrieved document.
// If fps is present, only the given field paths are retrieved. It is undefined
// whether other fields of doc at the time of the call are removed, unchanged,
// or zeroed, so for portable behavior doc should contain only the key fields.
// If you plan to write the document back and let Docstore to perform optimistic
// locking, include the revision field in fps. See more about revision at
// https://godoc.org/gocloud.dev/docstore#hdr-Revisions.
func (l *ActionList) Get(doc Document, fps ...FieldPath) *ActionList {
	return l.add(&Action{
		kind:       driver.Get,
		doc:        doc,
		fieldpaths: fps,
	})
}

// Update atomically applies Mods to doc, which must exist.
// Only the key and revision fields of doc are used.
// It is an error to pass an empty Mods to Update.
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
// Update does not modify its doc argument, except to set the new revision. To obtain
// the updated document, call Get after calling Update.
func (l *ActionList) Update(doc Document, mods Mods) *ActionList {
	return l.add(&Action{
		kind: driver.Update,
		doc:  doc,
		mods: mods,
	})
}

// Mods is a map from field paths to modifications.
// At present, a modification is one of:
//   - nil, to delete the field
//   - an Increment value, to add a number to the field
//   - any other value, to set the field to that value
//
// See ActionList.Update.
type Mods map[FieldPath]interface{}

// Increment returns a modification that results in a field being incremented. It
// should only be used as a value in a Mods map, like so:
//
//	docstore.Mods{"count": docstore.Increment(1)}
//
// The amount must be an integer or floating-point value.
func Increment(amount interface{}) interface{} {
	return driver.IncOp{amount}
}

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

// BeforeDo takes a callback function that will be called before the ActionList is
// executed by the underlying service. It may be invoked multiple times for a single
// call to ActionList.Do, because the driver may split the action list into several
// service calls. If any callback invocation returns an error, ActionList.Do returns
// an error.
//
// The callback takes a parameter, asFunc, that converts its argument to
// driver-specific types. See https://gocloud.dev/concepts/as for background
// information.
func (l *ActionList) BeforeDo(f func(asFunc func(interface{}) bool) error) *ActionList {
	l.beforeDo = f
	return l
}

// Do executes the action list.
//
// If Do returns a non-nil error, it will be of type ActionListError. If any action
// fails, the returned error will contain the position in the ActionList of each
// failed action.
//
// All the actions will be executed. Docstore tries to execute the actions as
// efficiently as possible. Sometimes this makes it impossible to attribute failures
// to specific actions; in such cases, the returned ActionListError will have entries
// whose Index field is negative.
func (l *ActionList) Do(ctx context.Context) error {
	return l.do(ctx, true)
}

// do implements Do with optional OpenCensus tracing, so it can be used internally.
func (l *ActionList) do(ctx context.Context, oc bool) (err error) {
	if err := l.coll.checkClosed(); err != nil {
		return ActionListError{{-1, errClosed}}
	}

	if oc {
		ctx = l.coll.tracer.Start(ctx, "ActionList.Do")
		defer func() { l.coll.tracer.End(ctx, err) }()
	}

	das, err := l.toDriverActions()
	if err != nil {
		return err
	}
	dopts := &driver.RunActionsOptions{BeforeDo: l.beforeDo}
	alerr := ActionListError(l.coll.driver.RunActions(ctx, das, dopts))
	if len(alerr) == 0 {
		return nil // Explicitly return nil, because alerr is not of type error.
	}
	for i := range alerr {
		alerr[i].Err = wrapError(l.coll.driver, alerr[i].Err)
	}
	return alerr
}

func (l *ActionList) toDriverActions() ([]*driver.Action, error) {
	var das []*driver.Action
	var alerr ActionListError
	// Create a set of (document key, is Get action) pairs for detecting duplicates:
	// an action list can have at most one get and at most one write for each key.
	type keyAndKind struct {
		key   interface{}
		isGet bool
	}
	seen := map[keyAndKind]bool{}
	for i, a := range l.actions {
		d, err := l.coll.toDriverAction(a)
		// Check for duplicate key.
		if err == nil && d.Key != nil {
			kk := keyAndKind{d.Key, d.Kind == driver.Get}
			if seen[kk] {
				err = gcerr.Newf(gcerr.InvalidArgument, nil, "duplicate key in action list: %v", d.Key)
			} else {
				seen[kk] = true
			}
		}
		if err != nil {
			alerr = append(alerr, struct {
				Index int
				Err   error
			}{i, wrapError(l.coll.driver, err)})
		} else {
			d.Index = i
			das = append(das, d)
		}
	}
	if len(alerr) > 0 {
		return nil, alerr
	}
	return das, nil
}

func (c *Collection) toDriverAction(a *Action) (*driver.Action, error) {
	ddoc, err := driver.NewDocument(a.doc)
	if err != nil {
		return nil, err
	}
	key, err := c.driver.Key(ddoc)
	if err != nil {
		if gcerrors.Code(err) != gcerr.InvalidArgument {
			err = gcerr.Newf(gcerr.InvalidArgument, err, "bad document key")
		}
		return nil, err
	}
	if key == nil || driver.IsEmptyValue(reflect.ValueOf(key)) {
		if a.kind != driver.Create {
			return nil, gcerr.Newf(gcerr.InvalidArgument, nil, "missing document key")
		}
		// set the key to nil so that the following code does not need to check for
		// empty.
		key = nil
	}
	if reflect.ValueOf(key).Kind() == reflect.Ptr {
		return nil, gcerr.Newf(gcerr.InvalidArgument, nil, "keys cannot be pointers")
	}
	rev, _ := ddoc.GetField(c.revisionField())
	if a.kind == driver.Create && rev != nil {
		return nil, gcerr.Newf(gcerr.InvalidArgument, nil, "cannot create a document with a revision field")
	}
	kind := a.kind
	if kind == driver.Put && rev != nil {
		// A Put with a revision field is equivalent to a Replace.
		kind = driver.Replace
	}
	d := &driver.Action{Kind: kind, Doc: ddoc, Key: key}
	if a.fieldpaths != nil {
		d.FieldPaths, err = parseFieldPaths(a.fieldpaths)
		if err != nil {
			return nil, err
		}
	}
	if a.kind == driver.Update {
		d.Mods, err = toDriverMods(a.mods)
		if err != nil {
			return nil, err
		}
	}
	return d, nil
}

func parseFieldPaths(fps []FieldPath) ([][]string, error) {
	res := make([][]string, len(fps))
	for i, s := range fps {
		fp, err := parseFieldPath(s)
		if err != nil {
			return nil, err
		}
		res[i] = fp
	}
	return res, nil
}

func toDriverMods(mods Mods) ([]driver.Mod, error) {
	// Convert mods from a map to a slice of (fieldPath, value) pairs.
	// The map is easier for users to write, but the slice is easier
	// to process.
	if len(mods) == 0 {
		return nil, gcerr.Newf(gcerr.InvalidArgument, nil, "no mods passed to Update")
	}

	// Sort keys so tests are deterministic.
	// After sorting, a key might not immediately follow its prefix. Consider the
	// sorted list of keys "a", "a+b", "a.b". "a" is prefix of "a.b", but since '+'
	// sorts before '.', it is not adjacent to it. All we can assume is that the
	// prefix is before the key.
	var keys []string
	for k := range mods {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)

	var dmods []driver.Mod
	for _, k := range keys {
		k := FieldPath(k)
		v := mods[k]
		fp, err := parseFieldPath(k)
		if err != nil {
			return nil, err
		}
		for _, d := range dmods {
			if fpHasPrefix(fp, d.FieldPath) {
				return nil, gcerr.Newf(gcerr.InvalidArgument, nil,
					"field path %q is a prefix of %q", strings.Join(d.FieldPath, "."), k)
			}
		}
		if inc, ok := v.(driver.IncOp); ok && !isIncNumber(inc.Amount) {
			return nil, gcerr.Newf(gcerr.InvalidArgument, nil,
				"Increment amount %v of type %[1]T must be an integer or floating-point number", inc.Amount)
		}
		dmods = append(dmods, driver.Mod{FieldPath: fp, Value: v})
	}
	return dmods, nil
}

// fpHasPrefix reports whether the field path fp begins with prefix.
func fpHasPrefix(fp, prefix []string) bool {
	if len(fp) < len(prefix) {
		return false
	}
	for i, p := range prefix {
		if fp[i] != p {
			return false
		}
	}
	return true
}

func isIncNumber(x interface{}) bool {
	switch reflect.TypeOf(x).Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	case reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

func (l *ActionList) String() string {
	var as []string
	for _, a := range l.actions {
		as = append(as, a.String())
	}
	return "[" + strings.Join(as, ", ") + "]"
}

func (a *Action) String() string {
	buf := &strings.Builder{}
	fmt.Fprintf(buf, "%s(%v", a.kind, a.doc)
	for _, fp := range a.fieldpaths {
		fmt.Fprintf(buf, ", %s", fp)
	}
	for _, m := range a.mods {
		fmt.Fprintf(buf, ", %v", m)
	}
	fmt.Fprint(buf, ")")
	return buf.String()
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

// RevisionToString converts a document revision to a string. The returned
// string should be treated as opaque; its only use is to provide a serialized
// form that can be passed around (e.g., as a hidden field on a web form)
// and then turned back into a revision using StringToRevision. The string is safe
// for use in URLs and HTTP forms.
func (c *Collection) RevisionToString(rev interface{}) (string, error) {
	if rev == nil {
		return "", gcerr.Newf(gcerr.InvalidArgument, nil, "RevisionToString: nil revision")
	}
	bytes, err := c.driver.RevisionToBytes(rev)
	if err != nil {
		return "", wrapError(c.driver, err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

// StringToRevision converts a string obtained with RevisionToString
// to a revision.
func (c *Collection) StringToRevision(s string) (interface{}, error) {
	if s == "" {
		return "", gcerr.Newf(gcerr.InvalidArgument, nil, "StringToRevision: empty string")
	}
	bytes, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	rev, err := c.driver.BytesToRevision(bytes)
	if err != nil {
		return "", wrapError(c.driver, err)
	}
	return rev, nil
}

// As converts i to driver-specific types.
// See https://gocloud.dev/concepts/as/ for background information, the "As"
// examples in this package for examples, and the driver package
// documentation for the specific types supported for that driver.
func (c *Collection) As(i interface{}) bool {
	if i == nil {
		return false
	}
	return c.driver.As(i)
}

var errClosed = gcerr.Newf(gcerr.FailedPrecondition, nil, "docstore: Collection has been closed")

// Close releases any resources used for the collection.
func (c *Collection) Close() error {
	c.mu.Lock()
	prev := c.closed
	c.closed = true
	c.mu.Unlock()
	if prev {
		return errClosed
	}
	return wrapError(c.driver, c.driver.Close())
}

func (c *Collection) checkClosed() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errClosed
	}
	return nil
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

// ErrorAs converts i to driver-specific types. See
// https://gocloud.dev/concepts/as/ for background information and the
// driver package documentation for the specific types supported for
// that driver.
//
// When the error is an ActionListError, ErrorAs works on individual errors in
// the slice, not the slice itself.
//
// ErrorAs panics if i is nil or not a pointer.
// ErrorAs returns false if err == nil.
func (c *Collection) ErrorAs(err error, i interface{}) bool {
	return gcerr.ErrorAs(err, i, c.driver.ErrorAs)
}
