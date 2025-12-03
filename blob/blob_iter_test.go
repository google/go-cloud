package blob_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/blob/memblob"
)

// Verify ListIterator.All.
func TestListIterator_All(t *testing.T) {
	ctx := context.Background()
	b := memblob.OpenBucket(nil)
	defer b.Close()

	// Initialize the bucket with some keys.
	want := map[string]string{}
	for _, key := range []string{"a", "b", "c"} {
		contents := fmt.Sprintf("%s-contents", key)
		if err := b.WriteAll(ctx, key, []byte(contents), nil); err != nil {
			t.Fatalf("failed to initialize key %q: %v", key, err)
		}
		want[key] = contents
	}

	// Iterate over the bucket using iter.All.
	got := map[string]string{}
	li := b.List(nil)
	iter, errFn := li.All(ctx)
	for obj, download := range iter {
		var buf bytes.Buffer
		if dErr := download(&buf, nil); dErr != nil {
			t.Errorf("failed to download %q: %v", obj.Key, dErr)
		}
		got[obj.Key] = string(buf.Bytes())
	}
	if err := errFn(); err != nil {
		t.Fatalf("iteration failed: %v", err)
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("got %v, want %v, diff %s", got, want, diff)
	}
}
