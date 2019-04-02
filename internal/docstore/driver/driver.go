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

	"gocloud.dev/internal/gcerr"
)

// A Collection is a set of documents.
type Collection interface {
	// RunActions executes a sequence of actions.
	// Implementations are free to execute the actions however they wish, but it must
	// appear as if they were executed in order. The actions need not happen
	// atomically.
	//
	// RunActions should return immediately after the first action that fails.
	// The first return value is the number of actions successfully
	// executed, and the second is the error that caused the action to fail.
	//
	// If all actions succeed, RunActions returns (number of actions, nil).
	RunActions(context.Context, []*Action) (int, error)

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
	FieldPath []string    // the field path to filter
	Op        string      // the operation, supports =, >, >=, <, <=
	Value     interface{} // the value to compare using the operation
}

// A DocumentIterator is an iterator that can iterator through the results (for Get
// action), and returns the next page that can be passed into a Query as a
// starting point.
type DocumentIterator interface {
	Next(doc Document) error
	Stop()
}
