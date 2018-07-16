// Copyright 2018 Google LLC
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

package wire

import (
	"go/token"
)

// errorCollector manages a list of errors. The zero value is an empty list.
type errorCollector struct {
	errors []error
}

// add appends any non-nil errors to the collector.
func (ec *errorCollector) add(errs ...error) {
	for _, e := range errs {
		if e != nil {
			ec.errors = append(ec.errors, e)
		}
	}
}

// mapErrors returns a new slice that wraps any errors using the given function.
func mapErrors(errs []error, f func(error) error) []error {
	if len(errs) == 0 {
		return nil
	}
	newErrs := make([]error, len(errs))
	for i := range errs {
		newErrs[i] = f(errs[i])
	}
	return newErrs
}

// A problem is an error with an optional position.
type problem struct {
	error    error
	position token.Position
}

// notePosition wraps an error with position information if it doesn't already
// have it.
func notePosition(p token.Position, e error) error {
	switch e.(type) {
	case nil:
		return nil
	case *problem:
		return e
	default:
		return &problem{error: e, position: p}
	}
}

// notePositionAll wraps a list of errors with the given position.
func notePositionAll(p token.Position, errs []error) []error {
	return mapErrors(errs, func(e error) error {
		return notePosition(p, e)
	})
}

// Error returns the error message prefixed by the position if valid.
func (p *problem) Error() string {
	if !p.position.IsValid() {
		return p.error.Error()
	}
	return p.position.String() + ": " + p.error.Error()
}
