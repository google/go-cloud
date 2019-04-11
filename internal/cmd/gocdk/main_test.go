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

package main

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestFindModuleRoot(t *testing.T) {
	t.Run("SameDirAsModule", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "gocdk-test")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		dir, err = filepath.EvalSymlinks(dir) // in case TMPDIR has a symlink like on darwin
		if err != nil {
			t.Fatal(err)
		}
		err = ioutil.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com\n"), 0666)
		if err != nil {
			t.Fatal(err)
		}

		got, err := findModuleRoot(context.Background(), dir)
		if got != dir || err != nil {
			t.Errorf("findModuleRoot(ctx, %q) = %q, %v; want %q, <nil>", dir, got, err, dir)
		}
	})
	t.Run("NoModFile", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "gocdk-test")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		dir, err = filepath.EvalSymlinks(dir) // in case TMPDIR has a symlink like on darwin
		if err != nil {
			t.Fatal(err)
		}

		got, err := findModuleRoot(context.Background(), dir)
		if err == nil {
			t.Errorf("findModuleRoot(ctx, %q) = %q, %v; want _, non-nil", dir, got, err)
		}
	})
	t.Run("ParentDirectory", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "gocdk-test")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		dir, err = filepath.EvalSymlinks(dir) // in case TMPDIR has a symlink like on darwin
		if err != nil {
			t.Fatal(err)
		}
		subdir := filepath.Join(dir, "subdir")
		if err := os.Mkdir(subdir, 0777); err != nil {
			t.Fatal(err)
		}
		err = ioutil.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com\n"), 0666)
		if err != nil {
			t.Fatal(err)
		}

		got, err := findModuleRoot(context.Background(), subdir)
		if got != dir || err != nil {
			t.Errorf("findModuleRoot(ctx, %q) = %q, %v; want %q, <nil>", dir, got, err, dir)
		}
	})
}
