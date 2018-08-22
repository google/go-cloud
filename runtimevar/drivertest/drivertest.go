// Copyright 2018 The Go Cloud Authors
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

// Package drivertest provides a conformance test for implementations of
// runtimevar.
package drivertest

import (
	"context"
	"testing"

	"github.com/google/go-cloud/runtimevar"
)

// Harness descibes the functionality test harnesses must provide to run
// conformance tests.
// All functions should fail the test on error.
type Harness interface {
	// MakeVar creates a *runtimevar.Variable to watch the given variable.
	MakeVar(ctx context.Context, name string, decoder *runtimevar.Decoder) (*runtimevar.Variable, error)
	// CreateVariable creates the variable with the given contents in the provider.
	CreateVariable(ctx context.Context, name string, val []byte) error
	// UpdateVariable updates an existing variable to have the given contents in the provider.
	UpdateVariable(ctx context.Context, name string, val []byte) error
	// DeleteVariable deletes an existing variable in the provider.
	DeleteVariable(ctx context.Context, name string) error
	// Close is called when the test is complete.
	Close()
}

// HarnessMaker describes functions that construct a harness for running tests.
// It is called exactly once per test; Harness.Close() will be called when the test is complete.
// Functions should fail the test on error, and save t in Harness to allow
// failing the test on errors in subsequent functions like MakeVar.
type HarnessMaker func(t *testing.T) (Harness, error)

// RunConformanceTests runs conformance tests for provider implementations
// of runtimevar.
func RunConformanceTests(t *testing.T, newHarness HarnessMaker) {
	// TODO(rvangent): Implement conformance tests here.
}
