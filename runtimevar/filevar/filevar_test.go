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

package filevar

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/drivertest"
)

type harness struct {
	dir    string
	closer func()
}

func newHarness(t *testing.T) (drivertest.Harness, error) {
	dir, err := ioutil.TempDir("", "filevar_test-")
	if err != nil {
		return nil, err
	}
	return &harness{
		dir:    dir,
		closer: func() { _ = os.RemoveAll(dir) },
	}, nil
}

func (h *harness) MakeVar(ctx context.Context, name string, decoder *runtimevar.Decoder) (*runtimevar.Variable, error) {
	path := filepath.Join(h.dir, name)
	return NewVariable(path, decoder, &WatchOptions{WaitTime: 50 * time.Millisecond})
}

func (h *harness) CreateVariable(ctx context.Context, name string, val []byte) error {
	path := filepath.Join(h.dir, name)
	return ioutil.WriteFile(path, val, 0666)
}

func (h *harness) UpdateVariable(ctx context.Context, name string, val []byte) error {
	return h.CreateVariable(ctx, name, val)
}

func (h *harness) DeleteVariable(ctx context.Context, name string) error {
	path := filepath.Join(h.dir, name)
	return os.Remove(path)
}

func (h *harness) Close() {
	h.closer()
}

func (h *harness) Mutable() bool { return true }

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness)
}
