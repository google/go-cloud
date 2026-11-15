package blob_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gocloud.dev/blob"
	"gocloud.dev/blob/fileblob"
	"gocloud.dev/internal/gcerr"
)

func TestPrefixedBucket(t *testing.T) {
	dir := t.TempDir()

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
	defer closeWithErrorCheck(t, wrapped)

	got, err := wrapped.ReadAll(ctx, "baz.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != contents {
		t.Errorf("got %q want %q", string(got), contents)
	}
}

func TestSingleKeyBucket(t *testing.T) {
	dir := t.TempDir()

	bucket, err := fileblob.OpenBucket(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	const contents = "contents"
	const key = "foo/bar.txt"
	ctx := context.Background()
	if err := bucket.WriteAll(ctx, key, []byte(contents), nil); err != nil {
		t.Fatal(err)
	}

	dirpath := filepath.ToSlash(dir)
	if os.PathSeparator != '/' && !strings.HasPrefix(dirpath, "/") {
		dirpath = "/" + dirpath
	}

	wrapped, err := blob.OpenBucket(ctx, "file://"+dirpath+"?key="+key)
	if err != nil {
		t.Fatal(err)
	}
	defer closeWithErrorCheck(t, wrapped)
	got, err := wrapped.ReadAll(ctx, "unused")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != contents {
		t.Errorf("got %q want %q", string(got), contents)
	}

	if err := wrapped.List(nil); err == nil {
		t.Errorf("List expected to fail for SingleKey bucket")
	}

	if attr, err := wrapped.Attributes(ctx, "unused"); err != nil {
		t.Errorf("Attributes failed: %v", err)
	} else if attr.Size != int64(len(contents)) {
		t.Errorf("Attributes.size got %d, want %d", attr.Size, len(contents))
	}

	const updatedContents = "new contents"
	if err := wrapped.WriteAll(ctx, "unused", []byte(updatedContents), nil); err != nil {
		t.Fatalf("WriteAll failed: %v", err)
	}
	got, err = wrapped.ReadAll(ctx, "unused")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != updatedContents {
		t.Errorf("got %q want %q", string(got), updatedContents)
	}

	if _, err := wrapped.SignedURL(ctx, "unused", nil); err == nil || !errors.Is(err, gcerr.ErrUnimplemented) {
		t.Errorf("SignedURL unexpectedly succeeded, or failed with unexpected error code: %v", err)
	}
}
