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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSnapshotTree(t *testing.T) {
	mkdir := func(dir, name string) error {
		return os.Mkdir(filepath.Join(dir, filepath.FromSlash(name)), 0777)
	}
	touchFile := func(dir, name string) error {
		joined := filepath.Join(dir, filepath.FromSlash(name))
		f, err := os.OpenFile(joined, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			return err
		}
		info, statErr := f.Stat()
		if err := f.Close(); err != nil {
			return err
		}
		if statErr != nil {
			return err
		}
		newTime := info.ModTime().Add(5 * time.Second)
		if err := os.Chtimes(joined, newTime, newTime); err != nil {
			return err
		}
		return nil
	}
	t.Run("NoChange", func(t *testing.T) {
		// TODO(light): Use constant when it's available.
		dir, err := ioutil.TempDir("", "gocdk-test")
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := os.RemoveAll(dir); err != nil {
				t.Error("clean up:", err)
			}
		}()
		if err := mkdir(dir, "foo"); err != nil {
			t.Fatal(err)
		}
		if err := touchFile(dir, "foo/bar.txt"); err != nil {
			t.Fatal(err)
		}
		if err := touchFile(dir, "baz.txt"); err != nil {
			t.Fatal(err)
		}

		snap, err := snapshotTree(dir)
		if err != nil {
			t.Fatalf("snapshotTree: %+v", err)
		}

		isSame, err := snap.sameAs(dir)
		if !isSame || err != nil {
			t.Errorf("snapshotTree(...).isSame(...) = %t, %+v; want true, <nil>", isSame, err)
		}
	})
	t.Run("AfterChange", func(t *testing.T) {
		// TODO(light): Use constant when it's available.
		dir, err := ioutil.TempDir("", "gocdk-test")
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := os.RemoveAll(dir); err != nil {
				t.Error("clean up:", err)
			}
		}()
		if err := mkdir(dir, "foo"); err != nil {
			t.Fatal(err)
		}
		if err := touchFile(dir, "foo/bar.txt"); err != nil {
			t.Fatal(err)
		}
		if err := touchFile(dir, "baz.txt"); err != nil {
			t.Fatal(err)
		}

		snap, err := snapshotTree(dir)
		if err != nil {
			t.Fatalf("snapshotTree: %+v", err)
		}
		if err := touchFile(dir, "foo/bar.txt"); err != nil {
			t.Fatal(err)
		}

		isSame, err := snap.sameAs(dir)
		if isSame || err != nil {
			t.Errorf("snapshotTree(...).isSame(...) = %t, %+v; want false, <nil>", isSame, err)
		}
	})
}
