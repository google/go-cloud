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

// Package driver defines interfaces to be implemented by docstore drivers, which
// will be used by the docstore package to interact with the underlying services.
// Application code should use package docstore.
package driver // import "gocloud.dev/docstore/driver"

import (
	"context"

	"gocloud.dev/gcerrors"
)

// A Collection is a set of documents.
type Collection interface {
	// Key returns the document key, or nil if the document doesn't have one, which
	// means it is absent or zero value, such as 0, a nil interface value, and any
	// empty array or string.
	//
	// If the collection is able to generate a key for a Create action, then
	// it should not return an error if the key is missing. If the collection
	// can't generate a missing key, it should return an error.
	//
	// The returned key must be comparable.
	//
	// The returned key should not be encoded with the driver's codec; it should
	// be the user-supplied Go value.
	Key(Document) (interface{}, error)

	// RevisionField returns the name of the field used to hold revisions.
	// If the empty string is returned, docstore.DefaultRevisionField will be used.
	RevisionField() string

	// RunActions executes a slice of actions.
	//
	// If unordered is false, it must appear as if the actions were executed in the
	// order they appear in the slice, from the client's point of view. The actions
	// need not happen atomically, nor does eventual consistency in the service
	// need to be taken into account. For example, after a write returns
	// successfully, the driver can immediately perform a read on the same document,
	// even though the service's semantics does not guarantee that the read will see
	// the write. RunActions should return immediately after the first action that fails.
	// The returned slice should have a single element.
	//
	// opts controls the behavior of RunActions and is guaranteed to be non-nil.
	RunActions(ctx context.Context, actions []*Action, opts *RunActionsOptions) ActionListError

	// RunGetQuery executes a Query.
	//
	// Implementations can choose to execute the Query as one single request or
	// multiple ones, depending on their service offerings. The portable type
	// exposes OpenCensus metrics for the call to RunGetQuery (but not for
	// subsequent calls to DocumentIterator.Next), so drivers should prefer to
	// make at least one RPC during RunGetQuery itself instead of lazily waiting
	// for the first call to Next.
	RunGetQuery(context.Context, *Query) (DocumentIterator, error)

	// QueryPlan returns the plan for the query.
	QueryPlan(*Query) (string, error)

	// RevisionToBytes converts a revision to a byte slice.
	RevisionToBytes(interface{}) ([]byte, error)

	// BytesToRevision converts a []byte to a revision.
	BytesToRevision([]byte) (interface{}, error)

	// As converts i to driver-specific types.
	// See https://gocloud.dev/concepts/as/ for background information.
	As(i interface{}) bool

	// ErrorAs allows drivers to expose driver-specific types for returned
	// errors.
	//
	// See https://gocloud.dev/concepts/as/ for background information.
	ErrorAs(err error, i interface{}) bool

	// ErrorCode should return a code that describes the error, which was returned by
	// one of the other methods in this interface.
	ErrorCode(error) gcerrors.ErrorCode

	// Close cleans up any resources used by the Collection. Once Close is called,
	// there will be no method calls to the Collection other than As, ErrorAs, and
	// ErrorCode.
	Close() error
}

// DeleteQueryer should be implemented by Collections that can handle Query.Delete
// efficiently. If a Collection does not implement this interface, then Query.Delete
// will be implemented by calling RunGetQuery and deleting the returned documents.
type DeleteQueryer interface {
	RunDeleteQuery(context.Context, *Query) error
}

// UpdateQueryer should be implemented by Collections that can handle Query.Update
// efficiently. If a Collection does not implement this interface, then Query.Update
// will be implemented by calling RunGetQuery and updating the returned documents.
type UpdateQueryer interface {
	RunUpdateQuery(context.Context, *Query, []Mod) error
}

// ActionKind describes the type of an action.
type ActionKind int

// Values for ActionKind.
const (
	Create ActionKind = iota
	Replace
	Put
	Get
	Delete
	Update
)

//go:generate stringer -type=ActionKind

// An Action describes a single operation on a single document.
type Action struct {
	Kind       ActionKind  // the kind of action
	Doc        Document    // the document on which to perform the action
	Key        interface{} // the document key returned by Collection.Key, to avoid recomputing it
	FieldPaths [][]string  // field paths to retrieve, for Get only
	Mods       []Mod       // modifications to make, for Update only
	Index      int         // the index of the action in the original action list
}

// A Mod is a modification to a field path in a document.
// At present, the only modifications supported are:
// - set the value at the field path, or create the field path if it doesn't exist
// - delete the field path (when Value is nil)
type Mod struct {
	FieldPath []string
	Value     interface{}
}

// IncOp is a value representing an increment modification.
type IncOp struct {
	Amount interface{}
}

// An ActionListError contains all the errors encountered from a call to RunActions,
// and the positions of the corresponding actions.
type ActionListError []struct {
	Index int
	Err   error
}

// NewActionListError creates an ActionListError from a slice of errors.
// If the ith element err of the slice is non-nil, the resulting ActionListError
// will have an item {i, err}.
func NewActionListError(errs []error) ActionListError {
	var alerr ActionListError
	for i, err := range errs {
		if err != nil {
			alerr = append(alerr, struct {
				Index int
				Err   error
			}{i, err})
		}
	}
	return alerr
}

// RunActionsOptions controls the behavior of RunActions.
type RunActionsOptions struct {
	// BeforeDo is a callback that must be called once, sequentially, before each one
	// or group of the underlying service's actions is executed. asFunc allows
	// drivers to expose driver-specific types.
	BeforeDo func(asFunc func(interface{}) bool) error
}

// A Query defines a query operation to find documents within a collection based
// on a set of requirements.
type Query struct {
	// FieldPaths contain a list of field paths the user selects to return in the
	// query results. The returned documents should only have these fields
	// populated.
	FieldPaths [][]string

	// Filters contain a list of filters for the query. If there are more than one
	// filter, they should be combined with AND.
	Filters []Filter

	// Limit sets the maximum number of results returned by running the query. When
	// Limit <= 0, the driver implementation should return all possible results.
	Limit int

	// OrderByField is the field to use for sorting the results.
	OrderByField string

	// OrderAscending specifies the sort direction.
	OrderAscending bool

	// BeforeQuery is a callback that must be called exactly once before the
	// underlying service's query is executed. asFunc allows drivers to expose
	// driver-specific types.
	BeforeQuery func(asFunc func(interface{}) bool) error
}

// A Filter defines a filter expression used to filter the query result.
// If the value is a number type, the filter uses numeric comparison.
// If the value is a string type, the filter uses UTF-8 string comparison.
// TODO(#1762): support comparison of other types.
type Filter struct {
	FieldPath []string    // the field path to filter
	Op        string      // the operation, supports =, >, >=, <, <=
	Value     interface{} // the value to compare using the operation
}

// A DocumentIterator iterates through the results (for Get action).
type DocumentIterator interface {

	// Next tries to get the next item in the query result and decodes into Document
	// with the driver's codec.
	// When there are no more results, it should return io.EOF.
	// Once Next returns a non-nil error, it will never be called again.
	Next(context.Context, Document) error

	// Stop terminates the iterator before Next return io.EOF, allowing any cleanup
	// needed.
	Stop()

	// As converts i to driver-specific types.
	// See https://gocloud.dev/concepts/as/ for background information.
	As(i interface{}) bool
}

// EqualOp is the name of the equality operator.
// It is defined here to avoid confusion between "=" and "==".
const EqualOp = "="
