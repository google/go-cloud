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

package constantvar

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/driver"
	"github.com/google/go-cloud/runtimevar/drivertest"
)

type harness struct {
	// vars stores the variable value(s) that have been set using CreateVariable.
	vars map[string][]byte
}

func newHarness(t *testing.T) (drivertest.Harness, error) {
	return &harness{vars: map[string][]byte{}}, nil
}

func (h *harness) MakeWatcher(ctx context.Context, name string, decoder *runtimevar.Decoder) (driver.Watcher, error) {
	rawVal, found := h.vars[name]
	if !found {
		// The variable isn't set. Create a Variable that always returns an error.
		return &watcher{err: errors.New("not found")}, nil
	}
	val, err := decoder.Decode(rawVal)
	if err != nil {
		// The variable didn't decode.
		return &watcher{err: errors.New("not found")}, nil
	}
	return &watcher{value: val, t: time.Now()}, nil
}

func (h *harness) CreateVariable(ctx context.Context, name string, val []byte) error {
	h.vars[name] = val
	return nil
}

func (h *harness) UpdateVariable(ctx context.Context, name string, val []byte) error {
	return errors.New("not supported")
}

func (h *harness) DeleteVariable(ctx context.Context, name string) error {
	return errors.New("not supported")
}

func (h *harness) Close() {}

func (h *harness) Mutable() bool { return false }

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness)
}
