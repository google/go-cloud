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

// Package gcpfirestore provides a docstore implementation backed by Google Cloud
// Firestore.
// Use OpenCollection to construct a *docstore.Collection.
//
// Docstore types not supported by the Go firestore client, cloud.google.com/go/firestore:
//   - unsigned integers: encoded is int64s
//   - arrays: encoded as Firestore array values
//
// Firestore types not supported by Docstore:
//   - Document reference (a pointer to another Firestore document)
//
// # URLs
//
// For docstore.OpenCollection, gcpfirestore registers for the scheme
// "firestore".
// The default URL opener will create a connection using default credentials
// from the environment, as described in
// https://cloud.google.com/docs/authentication/production.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// # As
//
// gcpfirestore exposes the following types for as functions.
// The pb package is google.golang.org/genproto/googleapis/firestore/v1.
// The firestore  package is cloud.google.com/go/firestore/apiv1.
//   - Collection.As: *firestore.Client
//   - ActionList.BeforeDo: *pb.BatchGetDocumentsRequest or *pb.CommitRequest.
//   - Query.BeforeQuery: *pb.RunQueryRequest
//   - DocumentIterator: firestore.Firestore_RunQueryClient
//   - Error: *google.golang.org/grpc/status.Status
//
// # Queries
//
// Firestore allows only one field in a query to be compared with an inequality
// operator (one other than "="). This driver selects the first field in a Where
// clause with an inequality to pass to Firestore and handles the rest locally. For
// example, if the query specifies the three clauses A > 1, B > 2 and A < 3, then
// A > 1 and A < 3 will be sent to Firestore, and the results will be filtered by
// B > 2 in this driver.
//
// Firestore requires a composite index if a query contains both an equality and an
// inequality comparison. This driver returns an error if the necessary index does
// not exist. You must create the index manually. See
// https://cloud.google.com/firestore/docs/query-data/indexing for details.
//
// See https://cloud.google.com/firestore/docs/query-data/queries for more information on Firestore queries.
package gcpfirestore // import "gocloud.dev/docstore/gcpfirestore"

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"strings"

	vkit "cloud.google.com/go/firestore/apiv1"
	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"github.com/google/wire"
	"gocloud.dev/docstore"
	"gocloud.dev/docstore/driver"
	"gocloud.dev/gcerrors"
	"gocloud.dev/gcp"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/internal/useragent"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	tspb "google.golang.org/protobuf/types/known/timestamppb"
)

// Dial returns a client to use with Firestore and a clean-up function to close
// the client after used.
// If the 'FIRESTORE_EMULATOR_HOST' environment variable is set the client connects
// to the GCP firestore emulator by overriding the default endpoint.
func Dial(ctx context.Context, ts gcp.TokenSource) (*vkit.Client, func(), error) {
	opts := []option.ClientOption{
		useragent.ClientOption("docstore"),
	}
	if host := os.Getenv("FIRESTORE_EMULATOR_HOST"); host != "" {
		conn, err := grpc.DialContext(ctx, host, grpc.WithInsecure())
		if err != nil {
			return nil, nil, err
		}
		opts = append(opts,
			option.WithEndpoint(host),
			option.WithGRPCConn(conn),
		)
	} else {
		opts = append(opts, option.WithTokenSource(ts))
	}
	c, err := vkit.NewClient(ctx, opts...)
	return c, func() { c.Close() }, err
}

// Set holds Wire providers for this package.
var Set = wire.NewSet(
	Dial,
	wire.Struct(new(URLOpener), "Client"),
)

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
	// The name of the field holding the document revision.
	// Defaults to docstore.DefaultRevisionField.
	RevisionField string

	// The maximum number of RPCs that can be in progress for a single call to
	// ActionList.Do.
	// If less than 1, there is no limit.
	MaxOutstandingActionRPCs int
}

// CollectionResourceID constructs a resource ID for a collection from the project ID and the collection path.
// See the OpenCollection example for use.
func CollectionResourceID(projectID, collPath string) string {
	return fmt.Sprintf("projects/%s/databases/(default)/documents/%s", projectID, collPath)
}

// CollectResoureceIDWithDatabase constructs a resource ID for a collection from the project ID, database ID, and the collection path.
// See the OpenCollection example for use.
func CollectionResourceIDWithDatabase(projectID, databaseID, collPath string) string {
	return fmt.Sprintf("projects/%s/databases/%s/documents/%s", projectID, databaseID, collPath)
}

// OpenCollection creates a *docstore.Collection representing a Firestore collection.
//
// collResourceID must be of the form "project/<projectID>/databases/(default)/documents/<collPath>".
// <collPath> may be a top-level collection, like "States", or it may be a path to a nested
// collection, like "States/Wisconsin/Cities".
// See https://cloud.google.com/firestore/docs/reference/rest/ for more detail.
//
// gcpfirestore requires that a single field, nameField, be designated the primary
// key. Its values must be strings, and must be unique over all documents in the
// collection. The primary key must be provided to retrieve a document.
func OpenCollection(client *vkit.Client, collResourceID, nameField string, opts *Options) (*docstore.Collection, error) {
	c, err := newCollection(client, collResourceID, nameField, nil, opts)
	if err != nil {
		return nil, err
	}
	return docstore.NewCollection(c), nil
}

// OpenCollectionWithNameFunc creates a *docstore.Collection representing a Firestore collection.
//
// collResourceID must be of the form "project/<projectID>/databases/(default)/documents/<collPath>".
// <collPath> may be a top-level collection, like "States", or it may be a path to a nested
// collection, like "States/Wisconsin/Cities".
//
// The nameFunc argument is function that accepts a document and returns the value to
// be used for the document's primary key. It should return the empty string if the
// document is missing the information to construct a name. This will cause all
// actions, even Create, to fail.
//
// Providing a function to construct the primary key is useful in two situations: if
// your desired primary key field is not a string, or if there is more than one field
// you want to use as a primary key.
//
// For the collection to be usable with Query.Delete and Query.Update, nameFunc
// must work with both map and struct types representing the same underlying
// data structure. See gocloud.dev/docstore/drivertest.HighScoreKey for an example.
func OpenCollectionWithNameFunc(client *vkit.Client, collResourceID string, nameFunc func(docstore.Document) string, opts *Options) (*docstore.Collection, error) {
	c, err := newCollection(client, collResourceID, "", nameFunc, opts)
	if err != nil {
		return nil, err
	}
	return docstore.NewCollection(c), nil
}

var resourceIDRE = regexp.MustCompile(`^(projects/[^/]+/databases/[^/]+)/documents/.+`)

func newCollection(client *vkit.Client, collResourceID, nameField string, nameFunc func(docstore.Document) string, opts *Options) (*collection, error) {
	if nameField == "" && nameFunc == nil {
		return nil, gcerr.Newf(gcerr.InvalidArgument, nil, "one of nameField or nameFunc must be provided")
	}
	matches := resourceIDRE.FindStringSubmatch(collResourceID)
	if matches == nil {
		return nil, gcerr.Newf(gcerr.InvalidArgument, nil, "bad collection resource ID %q; must match %v",
			collResourceID, resourceIDRE)
	}
	dbPath := matches[1]
	if opts == nil {
		opts = &Options{}
	}
	if opts.RevisionField == "" {
		opts.RevisionField = docstore.DefaultRevisionField
	}
	return &collection{
		client:    client,
		nameField: nameField,
		nameFunc:  nameFunc,
		dbPath:    dbPath,
		collPath:  collResourceID,
		opts:      opts,
	}, nil
}

// Key returns the document key, if present. This is either the value of the field
// called c.nameField, or the result of calling c.nameFunc.
func (c *collection) Key(doc driver.Document) (interface{}, error) {
	if c.nameField != "" {
		name, err := doc.GetField(c.nameField)
		vn := reflect.ValueOf(name)
		if err != nil || name == nil || driver.IsEmptyValue(vn) {
			// missing field is not an error
			return nil, nil
		}
		// Check that the reflect kind is String so we can support any type whose underlying type
		// is string. E.g. "type DocName string".
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

func (c *collection) RevisionField() string {
	return c.opts.RevisionField
}

// RunActions implements driver.RunActions.
func (c *collection) RunActions(ctx context.Context, actions []*driver.Action, opts *driver.RunActionsOptions) driver.ActionListError {
	errs := make([]error, len(actions))
	beforeGets, gets, writes, afterGets := driver.GroupActions(actions)
	calls := c.buildCommitCalls(writes, errs)
	// runGets does not issue concurrent RPCs, so it doesn't need a throttle.
	c.runGets(ctx, beforeGets, errs, opts)
	t := driver.NewThrottle(c.opts.MaxOutstandingActionRPCs)
	for _, call := range calls {
		call := call
		t.Acquire()
		go func() {
			defer t.Release()
			c.doCommitCall(ctx, call, errs, opts)
		}()
	}
	t.Acquire()
	c.runGets(ctx, gets, errs, opts)
	t.Release()
	t.Wait()
	c.runGets(ctx, afterGets, errs, opts)
	return driver.NewActionListError(errs)
}

// runGets executes a group of Get actions by calling the BatchGetDocuments RPC.
// It may make several calls, because all gets in a single RPC must have the same set of field paths.
func (c *collection) runGets(ctx context.Context, actions []*driver.Action, errs []error, opts *driver.RunActionsOptions) {
	for _, group := range driver.GroupByFieldPath(actions) {
		c.batchGet(ctx, group, errs, opts)
	}
}

// Run a single BatchGet RPC with the given Get actions, all of which have the same set of field paths.
// Populate errs, a slice of per-action errors indexed by the original action list position.
func (c *collection) batchGet(ctx context.Context, gets []*driver.Action, errs []error, opts *driver.RunActionsOptions) {
	setErr := func(err error) {
		for _, g := range gets {
			errs[g.Index] = err
		}
	}

	req, err := c.newGetRequest(gets)
	if err != nil {
		setErr(err)
		return
	}
	indexByPath := map[string]int{} // from document path to index in gets
	for i, path := range req.Documents {
		indexByPath[path] = i
	}
	if opts.BeforeDo != nil {
		if err := opts.BeforeDo(driver.AsFunc(req)); err != nil {
			setErr(err)
			return
		}
	}
	streamClient, err := c.client.BatchGetDocuments(withResourceHeader(ctx, req.Database), req)
	if err != nil {
		setErr(err)
		return
	}
	for {
		resp, err := streamClient.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			setErr(err)
			return
		}
		switch r := resp.Result.(type) {
		case *pb.BatchGetDocumentsResponse_Found:
			pdoc := r.Found
			i, ok := indexByPath[pdoc.Name]
			if !ok {
				setErr(gcerr.Newf(gcerr.Internal, nil, "no index for path %s", pdoc.Name))
			} else {
				errs[gets[i].Index] = decodeDoc(pdoc, gets[i].Doc, c.nameField, c.opts.RevisionField)
			}
		case *pb.BatchGetDocumentsResponse_Missing:
			i := indexByPath[r.Missing]
			errs[gets[i].Index] = gcerr.Newf(gcerr.NotFound, nil, "document at path %q is missing", r.Missing)
		default:
			setErr(gcerr.Newf(gcerr.Internal, nil, "unknown BatchGetDocumentsResponse result type"))
			return
		}
	}
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

// commitCall holds information needed to make a Commit RPC and to follow up after it is done.
type commitCall struct {
	writes   []*pb.Write      // writes to commit
	actions  []*driver.Action // actions corresponding to those writes
	newNames []string         // new names for Create; parallel to actions
}

// Construct a set of concurrently runnable calls to Commit.
func (c *collection) buildCommitCalls(actions []*driver.Action, errs []error) []*commitCall {
	// Convert each action to one or more writes, collecting names for newly created
	// documents along the way. Divide writes into those with preconditions and those without.
	// Writes without preconditions can't fail, so we can execute them all in one Commit RPC.
	// All other writes must be run as separate Commits.
	var (
		nCall  = &commitCall{} // for writes without preconditions
		pCalls []*commitCall   // for writes with preconditions
	)
	for _, a := range actions {
		ws, nn, err := c.actionToWrites(a)
		if err != nil {
			errs[a.Index] = err
		} else if ws[0].CurrentDocument == nil { // no precondition
			nCall.writes = append(nCall.writes, ws...)
			nCall.actions = append(nCall.actions, a)
			nCall.newNames = append(nCall.newNames, nn)
		} else { // writes have a precondition
			pCalls = append(pCalls, &commitCall{
				writes:   ws,
				actions:  []*driver.Action{a},
				newNames: []string{nn},
			})
		}
	}
	if len(nCall.writes) == 0 {
		return pCalls
	}
	return append(pCalls, nCall)
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
		pc, perr := c.revisionPrecondition(a.Doc)
		if perr != nil {
			return nil, "", perr
		}
		// Otherwise, just require that the document exists.
		if pc == nil {
			pc = &pb.Precondition{ConditionType: &pb.Precondition_Exists{Exists: true}}
		}
		w, err = c.putWrite(a.Doc, docName, pc)

	case driver.Put:
		pc, perr := c.revisionPrecondition(a.Doc)
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
	pc, err := c.revisionPrecondition(doc)
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
	ts, err := c.revisionTimestamp(doc)
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

// doCommitCall Calls the Commit RPC with a list of writes, and handles the results.
func (c *collection) doCommitCall(ctx context.Context, call *commitCall, errs []error, opts *driver.RunActionsOptions) {
	wrs, err := c.commit(ctx, call.writes, opts)
	if err != nil {
		for _, a := range call.actions {
			errs[a.Index] = err
		}
		return
	}
	// Set the revision fields of the documents.
	// The actions and writes may not correspond, because Update actions may require
	// two writes. We can tell which writes correspond to actions by the type of write.
	j := 0
	for i, a := range call.actions {
		wr := wrs[j]
		if a.Doc.HasField(c.opts.RevisionField) {
			if err := a.Doc.SetField(c.opts.RevisionField, wr.UpdateTime); err != nil {
				errs[a.Index] = err
			}
		}
		if call.newNames[i] != "" {
			// c.nameField should not be empty since we only create new names when there
			// is a nameField.
			_ = a.Doc.SetField(c.nameField, call.newNames[i])
		}
		if hasFollowingTransform(call.writes, j) {
			j = j + 2
		} else {
			j++
		}
	}
	return
}

func hasFollowingTransform(writes []*pb.Write, i int) bool {
	if i >= len(writes)-1 {
		return false
	}
	curr, ok := writes[i].Operation.(*pb.Write_Update)
	if !ok {
		return false
	}
	next, ok := writes[i+1].Operation.(*pb.Write_Transform)
	if !ok {
		return false
	}
	return curr.Update.Name == next.Transform.Document
}

func (c *collection) commit(ctx context.Context, ws []*pb.Write, opts *driver.RunActionsOptions) ([]*pb.WriteResult, error) {
	req := &pb.CommitRequest{
		Database: c.dbPath,
		Writes:   ws,
	}
	if opts.BeforeDo != nil {
		if err := opts.BeforeDo(driver.AsFunc(req)); err != nil {
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

///////////////
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
var unquotedFieldRE = regexp.MustCompile("^[A-Za-z_][A-Za-z_0-9]*$")

// toServiceFieldPathComponent returns a string that represents key and is a valid
// Firestore field path component. Components must be quoted with backticks if
// they don't match the above regexp.
func toServiceFieldPathComponent(key string) string {
	if unquotedFieldRE.MatchString(key) {
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
func (c *collection) revisionPrecondition(doc driver.Document) (*pb.Precondition, error) {
	rev, err := c.revisionTimestamp(doc)
	if err != nil {
		return nil, err
	}
	return preconditionFromTimestamp(rev), nil
}

// revisionTimestamp extracts the timestamp from the revision field of doc, if there is one.
// It only returns an error if the revision field is present and does not contain the right type.
func (c *collection) revisionTimestamp(doc driver.Document) (*tspb.Timestamp, error) {
	v, err := doc.GetField(c.opts.RevisionField)
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
			c.opts.RevisionField, v)
	}
	return rev, nil
}

func preconditionFromTimestamp(ts *tspb.Timestamp) *pb.Precondition {
	if ts == nil || (ts.Seconds == 0 && ts.Nanos == 0) { // ignore a missing or zero revision
		return nil
	}
	return &pb.Precondition{ConditionType: &pb.Precondition_UpdateTime{ts}}
}

func (c *collection) ErrorCode(err error) gcerrors.ErrorCode {
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

// RevisionToBytes implements driver.RevisionToBytes.
func (c *collection) RevisionToBytes(rev interface{}) ([]byte, error) {
	r, ok := rev.(*tspb.Timestamp)
	if !ok {
		return nil, gcerr.Newf(gcerr.InvalidArgument, nil, "revision %v of type %[1]T is not a proto Timestamp", rev)
	}
	return proto.Marshal(r)
}

// BytesToRevision implements driver.BytesToRevision.
func (c *collection) BytesToRevision(b []byte) (interface{}, error) {
	var ts tspb.Timestamp
	if err := proto.Unmarshal(b, &ts); err != nil {
		return nil, err
	}
	return &ts, nil
}

func (c *collection) As(i interface{}) bool {
	p, ok := i.(**vkit.Client)
	if !ok {
		return false
	}
	*p = c.client
	return true
}

// ErrorAs implements driver.Collection.ErrorAs.
func (c *collection) ErrorAs(err error, i interface{}) bool {
	s, ok := status.FromError(err)
	if !ok {
		return false
	}
	p, ok := i.(**status.Status)
	if !ok {
		return false
	}
	*p = s
	return true
}

// Close implements driver.Collection.Close.
func (c *collection) Close() error { return nil }
