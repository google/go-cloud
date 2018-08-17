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
//
// Example:
//
// // makeVariable is of type VariableMaker. It creates a *runtimevar.Variable
// // for this provider implementation.
// func makeVariable(t *testing.T, name string, decoder *runtimevar.Decoder) (*runtimevar.Variable, interface{}, func()) {
// ...
// }
//
// // setVariable is of type VariableSetter. It performs action on the variable
// // in the provider.
// func setVariable(t *testing.T, h interface{}, name string, action Action, val []byte) {
// ...
// }
//
// func TestConformance(t *testing.T) {
//   drivertest.RunConformanceTests(t, makeVariable, setVariable)
// }
//
// Implementation note: A new *runtimevar.Variable is created for each sub-test
// so that provider implementations that use a record/replay strategy get
// a separate golden file per test.
package drivertest

import (
	"testing"

	"github.com/google/go-cloud/runtimevar"
)

// VariableMaker describes functions used to create a runtimevar.Variable
// for a test.
// Functions should return the Variable along with an optional handle, which
// will be passed to VariableSetter.
// Functions should fail the test on errors.
// See the package comments for more details on required semantics.
type VariableMaker func(t *testing.T, name string, decoder *runtimevar.Decoder) (*runtimevar.Variable, interface{}, func())

// Action is a type of action on a variable: Create, Update, Delete.
type Action int

const (
	CreateAction = Action(iota)
	UpdateAction
	DeleteAction
)

// VariableSetter describes functions used to create/update/delete a variable
// in the provider. Functions should fail the test on errors.
type VariableSetter func(t *testing.T, h interface{}, name string, action Action, val []byte)

// RunConformanceTests runs conformance tests for provider implementations
// of runtimevar.
func RunConformanceTests(t *testing.T, makeVar VariableMaker, setter VariableSetter) {
	// TODO(rvangent): Implement conformance tests here.
}
