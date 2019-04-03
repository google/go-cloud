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

// Package driver defines a set of interfaces that the docstore package uses to
// interact with the underlying services.
package driver // import "gocloud.dev/internal/docstore/driver"

import (
	"context"
	"fmt"
	"strings"

	"gocloud.dev/internal/gcerr"
)

// A Collection is a set of documents.
type Collection interface {
	// RunActions executes a slice of actions.
	//
	// If unordered is false, it must appear as if the actions were executed in the
	// order they appear in the slice, from the client's point of view. The actions
	// need not happen atomically, nor does eventual consistency in the provider
	// system need to be taken into account. For example, after a write returns
	// successfully, the driver can immediately perform a read on the same document,
	// even though the provider's semantics does not guarantee that the read will see
	// the write. RunActions should return immediately after the first action that fails.
	//
	// If unordered is true, the actions can be executed in any order, perhaps
	// concurrently. All of the actions should be executed, even if some fail.
	//
	// RunActions should return one error value per argument action, in the same
	// order as the argument slice. If an action was never executed, the error should
	// be docstore.ErrNotExecuted. If all actions succeed, RunActions may (but need
	// not) return nil. As a convenience, RunActions may return a slice that is
	// shorter than the number of actions, meaning that the unrepresented actions
	// were not executed. In other words, RunActions need not pad the slice with
	// ErrNotExecuted errors.
	RunActions(ctx context.Context, actions []*Action, unordered bool) ActionListError

	// RunQuery executes a Query.
	//
	// Implementations can choose to execute the Query as one single request or
	// multiple ones, depending on their service offerings.
	RunQuery(context.Context, *Query) error

	// ErrorCode should return a code that describes the error, which was returned by
	// one of the other methods in this interface.
	ErrorCode(error) gcerr.ErrorCode
}

// ActionKind describes the type of an action.
type ActionKind int

const (
	Create ActionKind = iota
	Replace
	Put
	Get
	Delete
	Update
)

// An Action describes a single operation on a single document.
type Action struct {
	Kind       ActionKind // the kind of action
	Doc        Document   // the document on which to perform the action
	FieldPaths [][]string // field paths to retrieve, for Get only
	Mods       []Mod      // modifications to make, for Update only
}

// A Mod is a modification to a field path in a document.
// At present, the only modifications supported are:
// - set the value at the field path, or create the field path if it doesn't exist
// - delete the field path (when Value is nil)
type Mod struct {
	FieldPath []string
	Value     interface{}
}

// An ActionListError contains all the errors encountered from a call to RunActions,
// and the positions of the corresponding actions.
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

func (e ActionListError) Unwrap() error {
	if len(e) == 1 {
		return e[0].Err
	}
	// Return nil when e is nil, or has more than one error.
	// When there are multiple errors, it doesn't make sense to return any of them.
	return nil
}

// A Query defines a query operation to find documents within a collection based
// on a set of requirements.
type Query struct {
	FieldPaths []string         // the selected field paths
	Filters    []Filter         // a list of filter for the query
	Limit      int              // the number of result in one query request
	StartAt    interface{}      // define the starting point of the query
	Iter       DocumentIterator // the iterator for Get action
}

// A Filter defines a filter expression used to filter the query result.
type Filter struct {
	Field string      // the field path to filter
	Op    string      // the operation, supports =, >, >=, <, <=
	Value interface{} // the value to filter
}

// A DocumentIterator is an iterator that can iterator through the results (for Get
// action), and returns the next page that can be passed into a Query as a
// starting point.
type DocumentIterator interface {
	Next(doc Document) error
	Stop()
}
