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

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestProcessContextModuleRoot(t *testing.T) {
	ctx := context.Background()
	t.Run("SameDirAsModule", func(t *testing.T) {
		pctx, cleanup, err := newTestProject(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()

		got, err := pctx.ModuleRoot(ctx)
		if got != pctx.workdir || err != nil {
			t.Errorf("got %q/%v want %q/<nil>", got, err, pctx.workdir)
		}
	})
	t.Run("NoBiomesDir", func(t *testing.T) {
		pctx, cleanup, err := newTestProject(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()
		if err := os.RemoveAll(biomesRootDir(pctx.workdir)); err != nil {
			t.Fatal(err)
		}

		if _, err = pctx.ModuleRoot(ctx); err == nil {
			t.Errorf("got nil error, want non-nil error due to missing biomes/ dir")
		}
	})
	t.Run("NoModFile", func(t *testing.T) {
		pctx, cleanup, err := newTestProject(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()
		if err := os.Remove(filepath.Join(pctx.workdir, "go.mod")); err != nil {
			t.Fatal(err)
		}

		if _, err = pctx.ModuleRoot(ctx); err == nil {
			t.Errorf("got nil error, want non-nil error due to missing go.mod")
		}
	})
	t.Run("ParentDirectory", func(t *testing.T) {
		pctx, cleanup, err := newTestProject(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()

		rootdir := pctx.workdir
		subdir := filepath.Join(rootdir, "subdir")
		if err := os.Mkdir(subdir, 0777); err != nil {
			t.Fatal(err)
		}
		pctx.workdir = subdir

		got, err := pctx.ModuleRoot(ctx)
		if got != rootdir || err != nil {
			t.Errorf("got %q/%v want %q/<nil>", got, err, rootdir)
		}
	})
}
