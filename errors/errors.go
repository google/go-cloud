// Copyright 2019 The Go Cloud Authors
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

// Package errors provides an error type for Go Cloud APIs.
package errors

import "fmt"

// A Code describes the error's category. Programs should act upon an error's
// code, not its message.
type Code int

const (
	// The error could not be categorized.
	Unknown Code = iota

	// The resource was not found.
	NotFound

	// The resource exists, but it should not.
	AlreadyExists

	// The caller is not authorized to perform the operation.
	PermissionDenied

	// A value given to a Go Cloud API is incorrect.
	InvalidArgument

	// Something unexpected happened. Internal errors always indicate
	// bugs in Go Cloud (or possibly the underlying provider).
	Internal
)

var codeStrings = []string{
	"Unknown",
	"NotFound",
	"AlreadyExists",
	"PermissionDenied",
	"InvalidArgument",
	"Internal",
}

// An Error described a Go Cloud error.
type Error struct {
	Code Code
	msg  string
	err  error
}

func (e *Error) Error() string {
	cs := "?badCode?"
	if e.Code >= 0 && int(e.Code) < len(codeStrings) {
		cs = codeStrings[e.Code]
	}
	return fmt.Sprintf("%s: %s", cs, e.msg)
}

// TODO(jba): use the new errors formatting code to print the underlying error.

// Unwrap returns the error underlying the receiver.
func (e *Error) Unwrap() error {
	return e.err
}

// New returns a new error with the given code, underlying error and message.
func New(c Code, err error, msg string) *Error {
	return &Error{
		Code: c,
		msg:  msg,
		err:  err,
	}
}

// Newf uses format and args to format a message, then calls New.
func Newf(c Code, err error, format string, args ...interface{}) *Error {
	return New(c, err, fmt.Sprintf(format, args...))
}
