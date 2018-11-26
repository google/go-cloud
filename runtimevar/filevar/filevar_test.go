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
	"github.com/google/go-cloud/runtimevar/driver"
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

func (h *harness) MakeWatcher(ctx context.Context, name string, decoder *runtimevar.Decoder) (driver.Watcher, error) {
	// filevar uses a goroutine in the background that poll every WaitDuration if
	// the file is deleted. Make this fast for tests.
	return newWatcher(filepath.Join(h.dir, name), decoder, &Options{WaitDuration: 1 * time.Millisecond})
}

func (h *harness) CreateVariable(ctx context.Context, name string, val []byte) error {
	// Write to a temporary file and rename; otherwise,
	// Watch can read an empty file during the write.
	tmp, err := ioutil.TempFile(h.dir, "tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(val); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	return os.Rename(tmp.Name(), filepath.Join(h.dir, name))
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
