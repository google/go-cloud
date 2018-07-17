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

package fileblob

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cloud/blob"
)

func TestNewBucket(t *testing.T) {
	t.Run("DirMissing", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "fileblob")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		_, err = NewBucket(filepath.Join(dir, "notfound"))
		if err == nil {
			t.Error("NewBucket did not return error")
		}
	})
	t.Run("File", func(t *testing.T) {
		f, err := ioutil.TempFile("", "fileblob")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(f.Name())
		_, err = NewBucket(f.Name())
		if err == nil {
			t.Error("NewBucket did not return error")
		}
	})
	t.Run("DirExists", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "fileblob")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		_, err = NewBucket(dir)
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestReader(t *testing.T) {
	t.Run("MetadataOnly", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "fileblob")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		const fileContent = "Hello, World!\n"
		err = ioutil.WriteFile(filepath.Join(dir, "foo.txt"), []byte(fileContent), 0666)
		if err != nil {
			t.Fatal(err)
		}

		b, err := NewBucket(dir)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		r, err := b.NewRangeReader(ctx, "foo.txt", 0, 0)
		if err != nil {
			t.Fatal("NewRangeReader:", err)
		}
		defer func() {
			if err := r.Close(); err != nil {
				t.Error("Close:", err)
			}
		}()
		if got := r.Size(); got != int64(len(fileContent)) {
			t.Errorf("r.Attrs().Size at beginning = %d; want %d", got, len(fileContent))
		}
		if got, err := ioutil.ReadAll(r); err != nil {
			t.Errorf("Read error: %v", err)
		} else if len(got) > 0 {
			t.Errorf("ioutil.ReadAll(r) = %q; fileContent \"\"", got)
		}
		if got := r.Size(); got != int64(len(fileContent)) {
			t.Errorf("r.Attrs().Size at end = %d; want %d", got, len(fileContent))
		}
	})
	t.Run("WholeBlob", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "fileblob")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		const want = "Hello, World!\n"
		err = ioutil.WriteFile(filepath.Join(dir, "foo.txt"), []byte(want), 0666)
		if err != nil {
			t.Fatal(err)
		}

		b, err := NewBucket(dir)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		r, err := b.NewReader(ctx, "foo.txt")
		if err != nil {
			t.Fatal("NewReader:", err)
		}
		defer func() {
			if err := r.Close(); err != nil {
				t.Error("Close:", err)
			}
		}()
		if got := r.Size(); got != int64(len(want)) {
			t.Errorf("r.Attrs().Size at beginning = %d; want %d", got, len(want))
		}
		if got, err := ioutil.ReadAll(r); err != nil {
			t.Errorf("Read error: %v", err)
		} else if string(got) != want {
			t.Errorf("ioutil.ReadAll(r) = %q; want %q", got, want)
		}
		if got := r.Size(); got != int64(len(want)) {
			t.Errorf("r.Attrs().Size at end = %d; want %d", got, len(want))
		}
	})
	t.Run("WithSlashSep", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "fileblob")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		const want = "Hello, World!\n"
		if err := os.Mkdir(filepath.Join(dir, "foo"), 0777); err != nil {
			t.Fatal(err)
		}
		err = ioutil.WriteFile(filepath.Join(dir, "foo", "bar.txt"), []byte(want), 0666)
		if err != nil {
			t.Fatal(err)
		}

		b, err := NewBucket(dir)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		r, err := b.NewReader(ctx, "foo/bar.txt")
		if err != nil {
			t.Fatal("NewReader:", err)
		}
		defer func() {
			if err := r.Close(); err != nil {
				t.Error("Close:", err)
			}
		}()
		if got, err := ioutil.ReadAll(r); err != nil {
			t.Errorf("Read error: %v", err)
		} else if string(got) != want {
			t.Errorf("ioutil.ReadAll(r) = %q; want %q", got, want)
		}
	})
	t.Run("BadNames", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "fileblob")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		subdir := filepath.Join(dir, "root")
		if err := os.Mkdir(subdir, 0777); err != nil {
			t.Fatal(err)
		}
		const want = "Hello, World!\n"
		if err := os.Mkdir(filepath.Join(subdir, "foo"), 0777); err != nil {
			t.Fatal(err)
		}
		err = ioutil.WriteFile(filepath.Join(subdir, "foo", "bar.txt"), []byte(want), 0666)
		if err != nil {
			t.Fatal(err)
		}
		err = ioutil.WriteFile(filepath.Join(subdir, "baz.txt"), []byte(want), 0666)
		if err != nil {
			t.Fatal(err)
		}
		err = ioutil.WriteFile(filepath.Join(dir, "passwd.txt"), []byte(want), 0666)
		if err != nil {
			t.Fatal(err)
		}

		b, err := NewBucket(subdir)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		names := []string{
			// Backslashes not allowed.
			"foo\\bar.txt",
			// Aliasing problems with unclean paths.
			"./baz.txt",
			"foo//bar.txt",
			"foo/../baz.txt",
			// Reaching outside directory.
			"../passwd.txt",
			"foo/../../passwd.txt",
			"/baz.txt",
			"C:\\baz.txt",
			"C:/baz.txt",
			"foo.txt.attrs",
		}
		for _, name := range names {
			r, err := b.NewReader(ctx, name)
			if err == nil {
				r.Close()
				t.Errorf("b.NewReader(ctx, %q) did not return error", name)
			}
		}
	})
	t.Run("Range", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "fileblob")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		const wholeFile = "Hello, World!\n"
		const offset, rangeLen = 1, 4
		const want = "ello"
		err = ioutil.WriteFile(filepath.Join(dir, "foo.txt"), []byte(wholeFile), 0666)
		if err != nil {
			t.Fatal(err)
		}

		b, err := NewBucket(dir)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		r, err := b.NewRangeReader(ctx, "foo.txt", offset, rangeLen)
		if err != nil {
			t.Fatal("NewRangeReader:", err)
		}
		defer func() {
			if err := r.Close(); err != nil {
				t.Error("Close:", err)
			}
		}()
		if got := r.Size(); got != int64(len(wholeFile)) {
			t.Errorf("r.Attrs().Size at beginning = %d; want %d", got, len(want))
		}
		if got, err := ioutil.ReadAll(r); err != nil {
			t.Errorf("Read error: %v", err)
		} else if string(got) != want {
			t.Errorf("ioutil.ReadAll(r) = %q; want %q", got, want)
		}
		if got := r.Size(); got != int64(len(wholeFile)) {
			t.Errorf("r.Attrs().Size at end = %d; want %d", got, len(want))
		}
	})
	t.Run("ObjectDoesNotExist", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "fileblob")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)

		b, err := NewBucket(dir)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		if _, err := b.NewRangeReader(ctx, "foo_not_exist.txt", 0, 0); err == nil || !blob.IsNotExist(err) {
			t.Errorf("NewReader: got %#v, want not exist error", err)
		}
	})
	// TODO(light): For sake of conformance test completionism, this should also
	// test range that goes past the end of the blob, but then we're just testing
	// the OS for fileblob.
}

func TestWriter(t *testing.T) {
	t.Run("Basic", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "fileblob")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)

		b, err := NewBucket(dir)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		w, err := b.NewWriter(ctx, "foo.txt", &blob.WriterOptions{
			ContentType: "application/octet-stream",
		})
		if err != nil {
			t.Fatal("NewWriter:", err)
		}
		const want = "Hello, World!\n"
		if n, err := w.Write([]byte(want)); n != len(want) || err != nil {
			t.Errorf("w.Write(%q) = %d, %v; want %d, <nil>", want, n, err, len(want))
		}
		if err := w.Close(); err != nil {
			t.Errorf("w.Close() = %v", err)
		}

		if got, err := ioutil.ReadFile(filepath.Join(dir, "foo.txt")); err != nil {
			t.Errorf("Read foo.txt: %v", err)
		} else if string(got) != want {
			t.Errorf("ioutil.ReadFile(\".../foo.txt\") = %q; want %q", got, want)
		}
	})
	t.Run("WithSlashSep", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "fileblob")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)

		b, err := NewBucket(dir)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		w, err := b.NewWriter(ctx, "foo/bar.txt", &blob.WriterOptions{
			ContentType: "application/octet-stream",
		})
		if err != nil {
			t.Fatal("NewWriter:", err)
		}
		const want = "Hello, World!\n"
		if n, err := w.Write([]byte(want)); n != len(want) || err != nil {
			t.Errorf("w.Write(%q) = %d, %v; want %d, <nil>", want, n, err, len(want))
		}
		if err := w.Close(); err != nil {
			t.Errorf("w.Close() = %v", err)
		}

		fpath := filepath.Join("foo", "bar.txt")
		if got, err := ioutil.ReadFile(filepath.Join(dir, fpath)); err != nil {
			t.Errorf("Read %s: %v", fpath, err)
		} else if string(got) != want {
			t.Errorf("ioutil.ReadFile(%q) = %q; want %q", filepath.Join("...", fpath), got, want)
		}
	})
	t.Run("BadNames", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "fileblob")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		subdir := filepath.Join(dir, "foo")
		if err := os.Mkdir(subdir, 0777); err != nil {
			t.Fatal(err)
		}

		b, err := NewBucket(subdir)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		names := []string{
			// Backslashes not allowed.
			"foo\\bar.txt",
			// Aliasing problems with unclean paths.
			"./baz.txt",
			"foo//bar.txt",
			"foo/../baz.txt",
			// Reaching outside directory.
			"../passwd.txt",
			"foo/../../passwd.txt",
			"/baz.txt",
			"C:\\baz.txt",
			"C:/baz.txt",
			"foo.bar.attrs",
		}
		for _, name := range names {
			w, err := b.NewWriter(ctx, name, &blob.WriterOptions{
				ContentType: "application/octet-stream",
			})
			if err == nil {
				w.Close()
				t.Errorf("b.NewWriter(ctx, %q, nil) did not return an error", name)
			}
		}
	})
}

func TestDelete(t *testing.T) {
	t.Run("Exists", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "fileblob")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		const fileContent = "Hello, World!\n"
		err = ioutil.WriteFile(filepath.Join(dir, "foo.txt"), []byte(fileContent), 0666)
		if err != nil {
			t.Fatal(err)
		}

		b, err := NewBucket(dir)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		if err := b.Delete(ctx, "foo.txt"); err != nil {
			t.Error("Delete:", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "foo.txt")); !os.IsNotExist(err) {
			t.Errorf("os.Stat(\".../foo.txt\") = _, %v; want not exist", err)
		}
	})
	t.Run("DoesNotExistError", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "fileblob")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)

		b, err := NewBucket(dir)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		if err := b.Delete(ctx, "foo.txt"); err == nil || !blob.IsNotExist(err) {
			t.Errorf("Delete: got %#v, want not exist error", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "foo.txt")); !os.IsNotExist(err) {
			t.Errorf("os.Stat(\".../foo.txt\") = _, %v; want not exist", err)
		}
	})
	t.Run("BadNames", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "fileblob")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		subdir := filepath.Join(dir, "root")
		if err := os.Mkdir(subdir, 0777); err != nil {
			t.Fatal(err)
		}
		const want = "Hello, World!\n"
		if err := os.Mkdir(filepath.Join(subdir, "foo"), 0777); err != nil {
			t.Fatal(err)
		}
		err = ioutil.WriteFile(filepath.Join(subdir, "foo", "bar.txt"), []byte(want), 0666)
		if err != nil {
			t.Fatal(err)
		}
		err = ioutil.WriteFile(filepath.Join(subdir, "baz.txt"), []byte(want), 0666)
		if err != nil {
			t.Fatal(err)
		}
		err = ioutil.WriteFile(filepath.Join(dir, "passwd.txt"), []byte(want), 0666)
		if err != nil {
			t.Fatal(err)
		}

		b, err := NewBucket(subdir)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		names := []string{
			// Backslashes not allowed.
			"foo\\bar.txt",
			// Aliasing problems with unclean paths.
			"./baz.txt",
			"foo//bar.txt",
			"foo/../baz.txt",
			// Reaching outside directory.
			"../passwd.txt",
			"foo/../../passwd.txt",
			"/baz.txt",
			"C:\\baz.txt",
			"C:/baz.txt",
			"foo.txt.attrs",
		}
		for _, name := range names {
			err := b.Delete(ctx, name)
			if err == nil {
				t.Errorf("b.Delete(ctx, %q) did not return error", name)
			}
		}
		mustExist := []string{
			filepath.Join(subdir, "foo", "bar.txt"),
			filepath.Join(subdir, "baz.txt"),
			filepath.Join(dir, "passwd.txt"),
		}
		for _, name := range mustExist {
			if _, err := os.Stat(name); err != nil {
				t.Errorf("os.Stat(%q): %v", name, err)
			}
		}
	})
}
