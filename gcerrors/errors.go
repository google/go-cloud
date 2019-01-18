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

// Package gcerrors provides support for getting error codes from
// errors returned by Go Cloud APIs.
package gcerrors

import "gocloud.dev/internal/gcerr"

// An ErrorCode describes the error's category. Programs should act upon an error's
// code, not its message.
type ErrorCode = gcerr.ErrorCode

const (
	// Returned by the Code function on a nil error. It is not a valid
	// code for an error.
	OK ErrorCode = gcerr.OK

	// The error could not be categorized.
	Unknown ErrorCode = gcerr.Unknown

	// The resource was not found.
	NotFound ErrorCode = gcerr.NotFound

	// The resource exists, but it should not.
	AlreadyExists ErrorCode = gcerr.AlreadyExists

	// a value given to a Go Cloud API is incorrect.
	InvalidArgument ErrorCode = gcerr.InvalidArgument

	// Something unexpected happened. Internal errors always indicate
	// bugs in Go Cloud (or possibly the underlying provider).
	Internal ErrorCode = gcerr.Internal
)

// Code returns the ErrorCode of err if it is an *Error.
// It returns Unknown if err is a non-nil error of a different type.
// If err is nil, it returns the special code OK.
func Code(err error) ErrorCode {
	if err == nil {
		return OK
	}
	if e, ok := err.(*gcerr.Error); ok {
		return e.Code
	}
	return Unknown
}
