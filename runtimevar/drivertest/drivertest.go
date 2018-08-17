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
//
// Package drivertest provides a conformance test for implementations of
// runtimevar.
package drivertest

import (
	"testing"

	"github.com/google/go-cloud/runtimevar"
)

// Initter describes functions used to set up for tests. It returns a handle
// that is passed to VariableMaker and VariableSetter during the test, and
// a function to be called when the test is complete.
// Functions should fail the test on error.
type Initter func(t *testing.T) (interface{}, func())

// VariableMaker describes functions used to create a runtimevar.Variable
// for a test. It may be called more than once per test.
// Functions should fail the test on errors.
type VariableMaker func(t *testing.T, h interface{}, name string, decoder *runtimevar.Decoder) *runtimevar.Variable

// Action is a type of action on a provider variable: Create, Update, Delete.
type Action int

const (
	// CreateAction indicates the config variable doesn't exist and should be created.
	CreateAction = Action(iota)
	// UpdateAction indicates the config variable already exists and should be updated.
	UpdateAction
	// DeleteAction indicates the config variable exists and should be deleted.
	DeleteAction
)

// VariableSetter describes functions used to create/update/delete a variable
// in the provider. Functions should fail the test on errors.
type VariableSetter func(t *testing.T, h interface{}, name string, action Action, val []byte)

// RunConformanceTests runs conformance tests for provider implementations
// of runtimevar.
func RunConformanceTests(t *testing.T, init Initter, makeVar VariableMaker, setter VariableSetter) {
	// TODO(rvangent): Implement conformance tests here.
}
