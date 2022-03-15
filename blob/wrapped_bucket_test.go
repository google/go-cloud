package blob_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gocloud.dev/blob"
	"gocloud.dev/blob/fileblob"
)

func TestPrefixedBucket(t *testing.T) {
	dir, cleanup := newTempDir()
	defer cleanup()
	bucket, err := fileblob.OpenBucket(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	const contents = "contents"
	ctx := context.Background()
	if err := bucket.WriteAll(ctx, "foo/bar/baz.txt", []byte(contents), nil); err != nil {
		t.Fatal(err)
	}

	wrapped := blob.PrefixedBucket(bucket, "foo/bar/")
	defer wrapped.Close()

	got, err := wrapped.ReadAll(ctx, "baz.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != contents {
		t.Errorf("got %q want %q", string(got), contents)
	}
}

func TestSingleKeyBucket(t *testing.T) {
	dir, cleanup := newTempDir()
	defer cleanup()
	bucket, err := fileblob.OpenBucket(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	const contents = "contents"
	ctx := context.Background()
	if err := bucket.WriteAll(ctx, "foo/bar.txt", []byte(contents), nil); err != nil {
		t.Fatal(err)
	}

	dirpath := filepath.ToSlash(dir)
	if os.PathSeparator != '/' && !strings.HasPrefix(dirpath, "/") {
		dirpath = "/" + dirpath
	}

	wrapped, err := blob.OpenBucket(ctx, "file://"+dirpath+"?key=foo/bar.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer wrapped.Close()
	got, err := wrapped.ReadAll(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != contents {
		t.Errorf("got %q want %q", string(got), contents)
	}
}
