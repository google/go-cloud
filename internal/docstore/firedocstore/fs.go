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

// Package firedocstore provides a docstore implementation backed by GCP
// Firestore.
// Use OpenCollection to construct a *docstore.Collection.
//
// URLs
//
// For docstore.OpenCollection, firedocstore registers for the scheme
// "firestore".
// The default URL opener will create a connection using default credentials
// from the environment, as described in
// https://cloud.google.com/docs/authentication/production.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// As
//
// firedocstore exposes the following types for As:
// - Collection.As: *firestore.Client
// - ActionList.BeforeDo: *pb.BatchGetDocumentRequest or *pb.CommitRequest.
// - Query.BeforeQuery: *firestore.RunQueryRequest
// - DocumentIterator: firestore.Firestore_RunQueryClient
package firedocstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"sync"

	vkit "cloud.google.com/go/firestore/apiv1"
	tspb "github.com/golang/protobuf/ptypes/timestamp"
	"gocloud.dev/gcp"
	"gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/driver"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/internal/useragent"
	"google.golang.org/api/option"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
	"google.golang.org/grpc/metadata"
)

func init() {
	docstore.DefaultURLMux().RegisterCollection(Scheme, &lazyCredsOpener{})
}

type lazyCredsOpener struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

func (o *lazyCredsOpener) OpenCollectionURL(ctx context.Context, u *url.URL) (*docstore.Collection, error) {
	o.init.Do(func() {
		creds, err := gcp.DefaultCredentials(ctx)
		if err != nil {
			o.err = err
			return
		}
		client, _, err := Dial(ctx, creds.TokenSource)
		if err != nil {
			o.err = err
			return
		}
		o.opener = &URLOpener{Client: client}
	})
	if o.err != nil {
		return nil, fmt.Errorf("open collection %s: %v", u, o.err)
	}
	return o.opener.OpenCollectionURL(ctx, u)
}

// Dial returns a client to use with Firestore and a clean-up function to close
// the client after used.
func Dial(ctx context.Context, ts gcp.TokenSource) (*vkit.Client, func(), error) {
	c, err := vkit.NewClient(ctx, option.WithTokenSource(ts), useragent.ClientOption("docstore"))
	return c, func() { c.Close() }, err
}

// Scheme is the URL scheme firestore registers its URLOpener under on
// docstore.DefaultMux.
const Scheme = "firestore"

// URLOpener opens firestore URLs like
// "firestore://myproject/mycollection?name_field=myID".
//
//   - The URL's host holds the GCP projectID.
//   - The only element of the URL's path holds the path to a Firestore collection.
// See https://firebase.google.com/docs/firestore/data-model for more details.
//
// The following query parameters are supported:
//
//   - name_field (required): firedocstore requires that a single string field,
// name_field, be designated the primary key. Its values must be unique over all
// documents in the collection, and the primary key must be provided to retrieve
// a document.
type URLOpener struct {
	// Client must be set to a non-nil client authenticated with Cloud Firestore
	// scope or equivalent.
	Client *vkit.Client
}

// OpenCollectionURL opens a docstore.Collection based on u.
func (o *URLOpener) OpenCollectionURL(ctx context.Context, u *url.URL) (*docstore.Collection, error) {
	q := u.Query()

	nameField := q.Get("name_field")
	if nameField == "" {
		return nil, errors.New("open collection %s: name_field is required to open a collection")
	}
	q.Del("name_field")
	for param := range q {
		return nil, fmt.Errorf("open collection %s: invalid query parameter %q", u, param)
	}
	project, collPath, err := collectionNameFromURL(u)
	if err != nil {
		return nil, fmt.Errorf("open collection %s: %v", u, err)
	}
	return OpenCollection(o.Client, project, collPath, nameField, nil)
}

func collectionNameFromURL(u *url.URL) (string, string, error) {
	var project, collPath string
	if project = u.Host; project == "" {
		return "", "", errors.New("URL must have a non-empty Host (the project ID)")
	}
	if collPath = strings.TrimPrefix(u.Path, "/"); collPath == "" {
		return "", "", errors.New("URL must have a non-empty Path (the collection path)")
	}
	return project, collPath, nil
}

type collection struct {
	nameField string
	nameFunc  func(docstore.Document) string
	client    *vkit.Client
	dbPath    string // e.g. "projects/P/databases/(default)"
	collPath  string // e.g. "projects/P/databases/(default)/documents/States/Wisconsin/cities"
	opts      *Options
}

// Options contains optional arguments to the OpenCollection functions.
type Options struct {
	// If true, allow queries that require client-side evaluation of filters (Where clauses)
	// to run.
	AllowLocalFilters bool
}

// OpenCollection creates a *docstore.Collection representing a Firestore collection.
//
// collPath is the path to the collection, starting from a root collection. It may
// refer to a top-level collection, like "States", or it may be a path to a nested
// collection, like "States/Wisconsin/Cities".
//
// firedocstore requires that a single string field, nameField, be designated the
// primary key. Its values must be unique over all documents in the collection, and
// the primary key must be provided to retrieve a document.
func OpenCollection(client *vkit.Client, projectID, collPath, nameField string, opts *Options) (*docstore.Collection, error) {
	c, err := newCollection(client, projectID, collPath, nameField, nil, opts)
	if err != nil {
		return nil, err
	}
	return docstore.NewCollection(c), nil
}

// OpenCollectionWithNameFunc creates a *docstore.Collection representing a Firestore collection.
//
// collPath is the path to the collection, starting from a root collection. It may
// refer to a top-level collection, like "States", or it may be a path to a nested
// collection, like "States/Wisconsin/Cities".
//
// The nameFunc argument is function that accepts a document and returns the value to
// be used for the document's primary key. It should return the empty string if the
// document is missing the information to construct a name. This will cause all
// actions, even Create, to fail.
func OpenCollectionWithNameFunc(client *vkit.Client, projectID, collPath string, nameFunc func(docstore.Document) string, opts *Options) (*docstore.Collection, error) {
	c, err := newCollection(client, projectID, collPath, "", nameFunc, opts)
	if err != nil {
		return nil, err
	}
	return docstore.NewCollection(c), nil
}

func newCollection(client *vkit.Client, projectID, collPath, nameField string, nameFunc func(docstore.Document) string, opts *Options) (*collection, error) {
	if nameField == "" && nameFunc == nil {
		return nil, gcerr.Newf(gcerr.InvalidArgument, nil, "one of nameField or nameFunc must be provided")
	}
	dbPath := fmt.Sprintf("projects/%s/databases/(default)", projectID)
	if opts == nil {
		opts = &Options{}
	}
	return &collection{
		client:    client,
		nameField: nameField,
		nameFunc:  nameFunc,
		dbPath:    dbPath,
		collPath:  fmt.Sprintf("%s/documents/%s", dbPath, collPath),
		opts:      opts,
	}, nil
}

// Key returns the document key, if present. This is either the value of the field
// called c.nameField, or the result of calling c.nameFunc.
func (c *collection) Key(doc driver.Document) (interface{}, error) {
	if c.nameField != "" {
		name, err := doc.GetField(c.nameField)
		if err != nil {
			// missing field is not an error
			return nil, nil
		}
		// Check that the reflect kind is String so we can support any type whose underlying type
		// is string. E.g. "type DocName string".
		vn := reflect.ValueOf(name)
		if vn.Kind() != reflect.String {
			return nil, gcerr.Newf(gcerr.InvalidArgument, nil, "key field %q with value %v is not a string",
				c.nameField, name)
		}
		sname := vn.String()
		if sname == "" { // empty string is the same as missing
			return nil, nil
		}
		return sname, nil
	}
	sname := c.nameFunc(doc.Origin)
	if sname == "" {
		return nil, gcerr.Newf(gcerr.InvalidArgument, nil, "missing document key")
	}
	return sname, nil
}

// RunActions implements driver.RunActions.
func (c *collection) RunActions(ctx context.Context, actions []*driver.Action, opts *driver.RunActionsOptions) driver.ActionListError {
	if opts.Unordered {
		return c.runActionsUnordered(ctx, actions, opts)
	}
	return c.runActionsOrdered(ctx, actions, opts)
}

func (c *collection) runActionsOrdered(ctx context.Context, actions []*driver.Action, opts *driver.RunActionsOptions) driver.ActionListError {
	// Split the actions into groups, each of which can be done with a single RPC.
	// - Consecutive writes are grouped together.
	// - Consecutive gets with the same field paths are grouped together.
	// TODO(jba): when we have transforms, apply the constraint that at most one transform per document
	// is allowed in a given request (see write.proto).
	groups := driver.SplitActions(actions, shouldSplit)
	nRun := 0 // number of actions successfully run
	var n int
	var err error
	for _, g := range groups {
		if g[0].Kind == driver.Get {
			n, err = c.runGetsOrdered(ctx, g, opts)
			nRun += n
		} else {
			err = c.runWrites(ctx, g, opts)
			// Writes happen atomically: all or none.
			if err != nil {
				nRun += len(g)
			}
		}
		if err != nil {
			return driver.ActionListError{{nRun, err}}
		}
	}
	return nil
}

// Reports whether two consecutive actions in a list should be split into different groups.
func shouldSplit(cur, new *driver.Action) bool {
	// If the new action isn't a Get but the current group consists of Gets, split.
	if new.Kind != driver.Get && cur.Kind == driver.Get {
		return true
	}
	// If the new  action is a Get and either (1) the current group consists of writes, or (2) the current group
	// of gets has a different list of field paths to retrieve, then split.
	// (The BatchGetDocuments RPC we use for Gets supports only a single set of field paths.)
	return new.Kind == driver.Get && (cur.Kind != driver.Get || !fpsEqual(cur.FieldPaths, new.FieldPaths))
}

// Run a sequence of Get actions by calling the BatchGetDocuments RPC.
// It returns the number of initial successful gets, as well as an error.
func (c *collection) runGetsOrdered(ctx context.Context, gets []*driver.Action, opts *driver.RunActionsOptions) (int, error) {
	errs := c.runGets(ctx, gets, opts)
	for i, err := range errs {
		if err != nil {
			return i, err
		}
	}
	return len(gets), nil
}

// runGets executes a group of Get actions by calling the BatchGetDocuments RPC.
// It returns the error for each Get action in order.
func (c *collection) runGets(ctx context.Context, gets []*driver.Action, opts *driver.RunActionsOptions) []error {
	errs := make([]error, len(gets))
	setErr := func(err error) {
		for i := range errs {
			errs[i] = err
		}
	}

	req, err := c.newGetRequest(gets)
	if err != nil {
		setErr(err)
		return errs
	}

	indexByPath := map[string]int{} // from document path to index in gets slice
	for i, path := range req.Documents {
		indexByPath[path] = i
	}
	if opts.BeforeDo != nil {
		asFunc := func(i interface{}) bool {
			p, ok := i.(**pb.BatchGetDocumentsRequest)
			if !ok {
				return false
			}
			*p = req
			return true
		}
		if err := opts.BeforeDo(asFunc); err != nil {
			setErr(err)
			return errs
		}
	}
	streamClient, err := c.client.BatchGetDocuments(withResourceHeader(ctx, req.Database), req)
	if err != nil {
		setErr(err)
		return errs
	}
	for {
		resp, err := streamClient.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			setErr(err)
			return errs
		}
		switch r := resp.Result.(type) {
		case *pb.BatchGetDocumentsResponse_Found:
			pdoc := r.Found
			i, ok := indexByPath[pdoc.Name]
			if !ok {
				errs[i] = gcerr.Newf(gcerr.Internal, nil, "no index for path %s", pdoc.Name)
			} else {
				errs[i] = decodeDoc(pdoc, gets[i].Doc, c.nameField)
			}
		case *pb.BatchGetDocumentsResponse_Missing:
			i := indexByPath[r.Missing]
			errs[i] = gcerr.Newf(gcerr.NotFound, nil, "document at path %q is missing", r.Missing)
		default:
			setErr(gcerr.Newf(gcerr.Internal, nil, "unknown BatchGetDocumentsResponse result type"))
			return errs
		}
	}
	return errs
}

func (c *collection) newGetRequest(gets []*driver.Action) (*pb.BatchGetDocumentsRequest, error) {
	req := &pb.BatchGetDocumentsRequest{Database: c.dbPath}
	for _, a := range gets {
		req.Documents = append(req.Documents, c.collPath+"/"+a.Key.(string))
	}
	// groupActions has already made sure that all the actions have the same field paths,
	// so just use the first one.
	var fps []string // field paths that will go in the mask
	for _, fp := range gets[0].FieldPaths {
		fps = append(fps, toServiceFieldPath(fp))
	}
	if fps != nil {
		req.Mask = &pb.DocumentMask{FieldPaths: fps}
	}
	return req, nil
}

// runWrites executes all the actions in a single RPC. The actions are done atomically,
// so either they all succeed or they all fail.
func (c *collection) runWrites(ctx context.Context, actions []*driver.Action, opts *driver.RunActionsOptions) error {
	// Convert each action to one or more writes, collecting names for newly created
	// documents along the way.
	var pws []*pb.Write
	newNames := make([]string, len(actions)) // from Creates without a name
	for i, a := range actions {
		ws, nn, err := c.actionToWrites(a)
		if err != nil {
			return err
		}
		newNames[i] = nn
		pws = append(pws, ws...)
	}
	// Call the Commit RPC with the list of writes.
	wrs, err := c.commit(ctx, pws, opts)
	if err != nil {
		return err
	}
	// Now that we've successfully done the action, set the names for newly created docs
	// that weren't given a name by the caller.
	for i, nn := range newNames {
		if nn != "" {
			_ = actions[i].Doc.SetField(c.nameField, nn)
		}
	}
	// TODO(jba): should we set the revision fields of all docs to the returned update times?
	// We should only do this if we can for all providers.
	_ = wrs
	// for i, wr := range wrs {
	// 	// Ignore errors. It's fine if the doc doesn't have a revision field.
	// 	// (We also could get an error if that field is unsettable for some reason, but
	// 	// we just decide to ignore those as well.)
	// 	_ = actions[i].Doc.SetField(docstore.RevisionField, wr.UpdateTime)
	// }
	return nil
}

// Convert an action to one or more Firestore Write protos.
func (c *collection) actionToWrites(a *driver.Action) ([]*pb.Write, string, error) {
	var (
		w       *pb.Write
		ws      []*pb.Write
		err     error
		docName string
		newName string // for Create with no name
	)
	if a.Key != nil {
		docName = a.Key.(string)
	}
	switch a.Kind {
	case driver.Create:
		// Make a name for this document if it doesn't have one.
		if a.Key == nil {
			docName = driver.UniqueString()
			newName = docName
		}
		w, err = c.putWrite(a.Doc, docName, &pb.Precondition{ConditionType: &pb.Precondition_Exists{Exists: false}})

	case driver.Replace:
		// If the given document has a revision, use it as the precondition (it implies existence).
		pc, perr := revisionPrecondition(a.Doc)
		if perr != nil {
			return nil, "", perr
		}
		// Otherwise, just require that the document exists.
		if pc == nil {
			pc = &pb.Precondition{ConditionType: &pb.Precondition_Exists{Exists: true}}
		}
		w, err = c.putWrite(a.Doc, docName, pc)

	case driver.Put:
		pc, perr := revisionPrecondition(a.Doc)
		if perr != nil {
			return nil, "", perr
		}
		w, err = c.putWrite(a.Doc, docName, pc)

	case driver.Update:
		ws, err = c.updateWrites(a.Doc, docName, a.Mods)

	case driver.Delete:
		w, err = c.deleteWrite(a.Doc, docName)

	default:
		err = gcerr.Newf(gcerr.Internal, nil, "bad action %+v", a)
	}
	if err != nil {
		return nil, "", err
	}
	if ws == nil {
		ws = []*pb.Write{w}
	}
	return ws, newName, nil
}

func (c *collection) putWrite(doc driver.Document, docName string, pc *pb.Precondition) (*pb.Write, error) {
	pdoc, err := encodeDoc(doc, c.nameField)
	if err != nil {
		return nil, err
	}
	pdoc.Name = c.collPath + "/" + docName
	return &pb.Write{
		Operation:       &pb.Write_Update{Update: pdoc},
		CurrentDocument: pc,
	}, nil
}

func (c *collection) deleteWrite(doc driver.Document, docName string) (*pb.Write, error) {
	pc, err := revisionPrecondition(doc)
	if err != nil {
		return nil, err
	}
	return &pb.Write{
		Operation:       &pb.Write_Delete{Delete: c.collPath + "/" + docName},
		CurrentDocument: pc,
	}, nil
}

// updateWrites returns a slice of writes because we may need two: one for setting
// and deleting values, the other for transforms.
func (c *collection) updateWrites(doc driver.Document, docName string, mods []driver.Mod) ([]*pb.Write, error) {
	ts, err := revisionTimestamp(doc)
	if err != nil {
		return nil, err
	}
	fields, paths, transforms, err := processMods(mods)
	if err != nil {
		return nil, err
	}
	return newUpdateWrites(c.collPath+"/"+docName, ts, fields, paths, transforms)
}

func newUpdateWrites(docPath string, ts *tspb.Timestamp, fields map[string]*pb.Value, paths []string, transforms []*pb.DocumentTransform_FieldTransform) ([]*pb.Write, error) {
	pc := preconditionFromTimestamp(ts)
	// If there is no revision in the document, add a precondition that the document exists.
	if pc == nil {
		pc = &pb.Precondition{ConditionType: &pb.Precondition_Exists{Exists: true}}
	}
	var ws []*pb.Write
	if len(fields) > 0 || len(paths) > 0 {
		ws = []*pb.Write{{
			Operation: &pb.Write_Update{Update: &pb.Document{
				Name:   docPath,
				Fields: fields,
			}},
			UpdateMask:      &pb.DocumentMask{FieldPaths: paths},
			CurrentDocument: pc,
		}}
		pc = nil // If the precondition is in the write, we don't need it in the transform.
	}
	// TODO(jba): test an increment-only update.
	if len(transforms) > 0 {
		ws = append(ws, &pb.Write{
			Operation: &pb.Write_Transform{
				Transform: &pb.DocumentTransform{
					Document:        docPath,
					FieldTransforms: transforms,
				},
			},
			CurrentDocument: pc,
		})
	}
	return ws, nil
}

// To update a document, we need to send:
// - A document with all the fields we want to add or change.
// - A mask with the field paths of all the fields we want to add, change or delete.
// processMods converts the mods into the fields for the document, and a list of
// valid Firestore field paths for the mask.
func processMods(mods []driver.Mod) (fields map[string]*pb.Value, maskPaths []string, transforms []*pb.DocumentTransform_FieldTransform, err error) {
	fields = map[string]*pb.Value{}
	for _, m := range mods {
		sfp := toServiceFieldPath(m.FieldPath)
		// If m.Value is nil, we want to delete it. In that case, we put the field in
		// the mask but not in the doc.
		if inc, ok := m.Value.(driver.IncOp); ok {
			pv, err := encodeValue(inc.Amount)
			if err != nil {
				return nil, nil, nil, err
			}
			transforms = append(transforms, &pb.DocumentTransform_FieldTransform{
				FieldPath: sfp,
				TransformType: &pb.DocumentTransform_FieldTransform_Increment{
					Increment: pv,
				},
			})
		} else {
			// The field path of every other mod belongs in the mask.
			maskPaths = append(maskPaths, sfp)
			if m.Value != nil {
				pv, err := encodeValue(m.Value)
				if err != nil {
					return nil, nil, nil, err
				}
				if err := setAtFieldPath(fields, m.FieldPath, pv); err != nil {
					return nil, nil, nil, err
				}
			}
		}
	}
	return fields, maskPaths, transforms, nil
}

func (c *collection) runActionsUnordered(ctx context.Context, actions []*driver.Action, opts *driver.RunActionsOptions) driver.ActionListError {
	// Split into groups the same way, but run them concurrently.
	// TODO(jba): group without considering order.
	groups := driver.SplitActions(actions, shouldSplit)
	errs := make([]error, len(actions))
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
				// Writes run in a transaction, so there is a single error for the group.
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

////////////////
// From memdocstore/mem.go.

// setAtFieldPath sets m's value at fp to val. It creates intermediate maps as
// needed. It returns an error if a non-final component of fp does not denote a map.
func setAtFieldPath(m map[string]*pb.Value, fp []string, val *pb.Value) error {
	m2, err := getParentMap(m, fp, true)
	if err != nil {
		return err
	}
	m2[fp[len(fp)-1]] = val
	return nil
}

// getParentMap returns the map that directly contains the given field path;
// that is, the value of m at the field path that excludes the last component
// of fp. If a non-map is encountered along the way, an InvalidArgument error is
// returned. If nil is encountered, nil is returned unless create is true, in
// which case a map is added at that point.
func getParentMap(m map[string]*pb.Value, fp []string, create bool) (map[string]*pb.Value, error) {
	for _, k := range fp[:len(fp)-1] {
		if m[k] == nil {
			if !create {
				return nil, nil
			}
			m[k] = &pb.Value{ValueType: &pb.Value_MapValue{&pb.MapValue{Fields: map[string]*pb.Value{}}}}
		}
		mv := m[k].GetMapValue()
		if mv == nil {
			return nil, gcerr.Newf(gcerr.InvalidArgument, nil, "invalid field path %q at %q", strings.Join(fp, "."), k)
		}
		m = mv.Fields
	}
	return m, nil
}

////////////////
// From fieldpath.go in cloud.google.com/go/firestore.

// Convert a docstore field path, which is a []string, into the kind of field path
// that the Firestore service expects: a string of dot-separated components, some of
// which may be quoted.
func toServiceFieldPath(fp []string) string {
	cs := make([]string, len(fp))
	for i, c := range fp {
		cs[i] = toServiceFieldPathComponent(c)
	}
	return strings.Join(cs, ".")
}

// Google SQL syntax for an unquoted field.
var unquotedFieldRegexp = regexp.MustCompile("^[A-Za-z_][A-Za-z_0-9]*$")

// toServiceFieldPathComponent returns a string that represents key and is a valid
// Firestore field path component. Components must be quoted with backticks if
// they don't match the above regexp.
func toServiceFieldPathComponent(key string) string {
	if unquotedFieldRegexp.MatchString(key) {
		return key
	}
	var buf bytes.Buffer
	buf.WriteRune('`')
	for _, r := range key {
		if r == '`' || r == '\\' {
			buf.WriteRune('\\')
		}
		buf.WriteRune(r)
	}
	buf.WriteRune('`')
	return buf.String()
}

// revisionPrecondition returns a Firestore precondition that asserts that the stored document's
// revision matches the revision of doc.
func revisionPrecondition(doc driver.Document) (*pb.Precondition, error) {
	rev, err := revisionTimestamp(doc)
	if err != nil {
		return nil, err
	}
	return preconditionFromTimestamp(rev), nil
}

// revisionTimestamp extracts the timestamp from the revision field of doc, if there is one.
// It only returns an error if the revision field is present and does not contain the right type.
func revisionTimestamp(doc driver.Document) (*tspb.Timestamp, error) {
	v, err := doc.GetField(docstore.RevisionField)
	if err != nil { // revision field not present
		return nil, nil
	}
	if v == nil { // revision field is present, but nil
		return nil, nil
	}
	rev, ok := v.(*tspb.Timestamp)
	if !ok {
		return nil, gcerr.Newf(gcerr.InvalidArgument, nil,
			"%s field contains wrong type: got %T, want proto Timestamp",
			docstore.RevisionField, v)
	}
	return rev, nil
}

func preconditionFromTimestamp(ts *tspb.Timestamp) *pb.Precondition {
	if ts == nil || (ts.Seconds == 0 && ts.Nanos == 0) { // ignore a missing or zero revision
		return nil
	}
	return &pb.Precondition{ConditionType: &pb.Precondition_UpdateTime{ts}}
}

// TODO(jba): make sure we enforce these Firestore commit constraints:
// - At most one `transform` per document is allowed in a given request.
// - An `update` cannot follow a `transform` on the same document in a given request.
// These should actually happen in groupActions.
func (c *collection) commit(ctx context.Context, ws []*pb.Write, opts *driver.RunActionsOptions) ([]*pb.WriteResult, error) {
	req := &pb.CommitRequest{
		Database: c.dbPath,
		Writes:   ws,
	}
	if opts.BeforeDo != nil {
		asFunc := func(i interface{}) bool {
			p, ok := i.(**pb.CommitRequest)
			if !ok {
				return false
			}
			*p = req
			return true
		}
		if err := opts.BeforeDo(asFunc); err != nil {
			return nil, err
		}
	}
	res, err := c.client.Commit(withResourceHeader(ctx, req.Database), req)
	if err != nil {
		return nil, err
	}
	if len(res.WriteResults) != len(ws) {
		return nil, gcerr.Newf(gcerr.Internal, nil, "wrong number of WriteResults from firestore commit")
	}
	return res.WriteResults, nil
}

// Report whether two lists of field paths are equal.
func fpsEqual(fps1, fps2 [][]string) bool {
	// TODO?: We really care about sets of field paths, but that's too tedious to determine.
	if len(fps1) != len(fps2) {
		return false
	}
	for i, fp1 := range fps1 {
		if !fpEqual(fp1, fps2[i]) {
			return false
		}
	}
	return true
}

// Report whether two field paths are equal.
func fpEqual(fp1, fp2 []string) bool {
	if len(fp1) != len(fp2) {
		return false
	}
	for i, s1 := range fp1 {
		if s1 != fp2[i] {
			return false
		}
	}
	return true
}

func (c *collection) ErrorCode(err error) gcerr.ErrorCode {
	return gcerr.GRPCCode(err)
}

// resourcePrefixHeader is the name of the metadata header used to indicate
// the resource being operated on.
const resourcePrefixHeader = "google-cloud-resource-prefix"

// withResourceHeader returns a new context that includes resource in a special header.
// Firestore uses the resource header for routing.
func withResourceHeader(ctx context.Context, resource string) context.Context {
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md[resourcePrefixHeader] = []string{resource}
	return metadata.NewOutgoingContext(ctx, md)
}

func (c *collection) As(i interface{}) bool {
	p, ok := i.(**vkit.Client)
	if !ok {
		return false
	}
	*p = c.client
	return true
}
