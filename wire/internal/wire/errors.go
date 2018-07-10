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
	"fmt"
	"go/token"
)

// problemCollector manages a list of problems. The zero value is an empty list
// of problems.
type problemCollector struct {
	problems []problem
}

// add appends an error to the list of problems in pc. If e is nil, then nothing
// is added.
func (pc *problemCollector) add(e error) {
	pc.problems = append(pc.problems, problemsFromError(e)...)
}

// err returns the collected problems as an error or nil if pc has not collected
// any errors.
func (pc *problemCollector) err() error {
	if len(pc.problems) == 0 {
		return nil
	}
	// pc.problems could be appended to later. Defensively limiting the capacity
	// forbids the later elements from being accessed or modified from the
	// returned list.
	return problemList(pc.problems[0:len(pc.problems):len(pc.problems)])
}

// errorList returns the collected problems as a list of errors.
func (pc *problemCollector) errorList() []error {
	return errorsFromProblems(pc.problems)
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
	case *problem, problemList:
		return e
	default:
		return &problem{error: e, position: p}
	}
}

// Error returns the error message prefixed by the position if valid.
func (p *problem) Error() string {
	if !p.position.IsValid() {
		return p.error.Error()
	}
	return p.position.String() + ": " + p.error.Error()
}

// problemsFromError unboxes an error (which may be nil) into zero or more
// problems. The caller should not modify the returned slice.
func problemsFromError(e error) []problem {
	switch e := e.(type) {
	case nil:
		return nil
	case *problem:
		return []problem{*e}
	case problemList:
		return []problem(e)
	default:
		return []problem{{error: e}}
	}
}

// errorsFromProblems converts a slice of problems into a slice of errors.
func errorsFromProblems(p []problem) []error {
	if len(p) == 0 {
		return nil
	}
	errs := make([]error, len(p))
	for i := range errs {
		errs[i] = &p[i]
	}
	return errs
}

// problemList aggregates problems into a type that implements the error
// interface. It should not be used directly; it serves as a marker type for
// problemsFromError.
type problemList []problem

func (list problemList) Error() string {
	switch len(list) {
	case 0:
		panic("problem list should not be empty")
	case 1:
		return list[0].Error()
	case 2:
		return list[0].Error() + " (and 1 other)"
	default:
		return fmt.Sprintf("%v (and %d others)", list[0].Error(), len(list)-1)
	}
}
