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

// Package gcerr provides an error type for Go CDK APIs.
package gcerr

import (
	"fmt"

	xerrors "golang.org/x/exp/errors"
	xfmt "golang.org/x/exp/errors/fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// An ErrorCode describes the error's category.
type ErrorCode int

const (
	// Returned by the Code function on a nil error. It is not a valid
	// code for an error.
	OK ErrorCode = 0

	// The error could not be categorized.
	Unknown ErrorCode = 1

	// The resource was not found.
	NotFound ErrorCode = 2

	// The resource exists, but it should not.
	AlreadyExists ErrorCode = 3

	// A value given to a Go CDK API is incorrect.
	InvalidArgument ErrorCode = 4

	// Something unexpected happened. Internal errors always indicate
	// bugs in the Go CDK (or possibly the underlying provider).
	Internal ErrorCode = 5

	// The feature is not implemented.
	Unimplemented ErrorCode = 6
)

// Call "go generate" whenever you change the above list of error codes.
// To get stringer:
//   go get golang.org/x/tools/cmd/stringer
//   Make sure $GOPATH/bin or $GOBIN in on your path.

//go:generate stringer -type=ErrorCode

// An Error describes a Go CDK error.
type Error struct {
	Code  ErrorCode
	msg   string
	frame xerrors.Frame
	err   error
}

func (e *Error) Error() string {
	if e.msg == "" {
		return fmt.Sprintf("code=%v", e.Code)
	}
	return fmt.Sprintf("%s (code=%v)", e.msg, e.Code)
}

func (e *Error) Format(s fmt.State, c rune) {
	xfmt.FormatError(s, c, e)
}

func (e *Error) FormatError(p xerrors.Printer) (next error) {
	p.Print(e.Error())
	e.frame.Format(p)
	return e.err
}

// Unwrap returns the error underlying the receiver, which may be nil.
func (e *Error) Unwrap() error {
	return e.err
}

// New returns a new error with the given code, underlying error and message. Pass 1
// for the call depth if New is called from the function raising the error; pass 2 if
// it is called from a helper function that was invoked by the original function; and
// so on.
func New(c ErrorCode, err error, callDepth int, msg string) *Error {
	return &Error{
		Code:  c,
		msg:   msg,
		frame: xerrors.Caller(callDepth),
		err:   err,
	}
}

// Newf uses format and args to format a message, then calls New.
func Newf(c ErrorCode, err error, format string, args ...interface{}) *Error {
	return New(c, err, 1, fmt.Sprintf(format, args...))
}

// GRPCCode extracts the gRPC status code and converts it into an ErrorCode.
// It returns Unknown if the error isn't from gRPC.
func GRPCCode(err error) ErrorCode {
	switch status.Code(err) {
	case codes.NotFound:
		return NotFound
	case codes.AlreadyExists:
		return AlreadyExists
	case codes.InvalidArgument:
		return InvalidArgument
	case codes.Internal:
		return Internal
	case codes.Unimplemented:
		return Unimplemented
	default:
		return Unknown
	}
}
