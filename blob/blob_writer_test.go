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

package blob_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/blob"
	"gocloud.dev/blob/memblob"
)

// TestWriteReturnValues verifies that blob.Writer returns the correct n
// even when it is doing content sniffing.
func TestWriteReturnValues(t *testing.T) {
	ctx := context.Background()

	for _, withContentType := range []bool{true, false} {
		t.Run(fmt.Sprintf("withContentType %v", withContentType), func(t *testing.T) {
			bucket := memblob.OpenBucket(nil)
			defer bucket.Close()

			var opts *blob.WriterOptions
			if withContentType {
				opts = &blob.WriterOptions{ContentType: "application/octet-stream"}
			}
			w, err := bucket.NewWriter(ctx, "testkey", opts)
			if err != nil {
				t.Fatalf("couldn't create writer with options: %v", err)
			}
			defer func() {
				if err := w.Close(); err != nil {
					t.Errorf("failed to close writer: %v", err)
				}
			}()
			n, err := io.CopyN(w, rand.Reader, 182)
			if err != nil || n != 182 {
				t.Fatalf("CopyN(182) got %d, want 182: %v", n, err)
			}
			n, err = io.CopyN(w, rand.Reader, 1812)
			if err != nil || n != 1812 {
				t.Fatalf("CopyN(1812) got %d, want 1812: %v", n, err)
			}
		})
	}
}

func randomData(nBytes int64) ([]byte, error) {
	var buf bytes.Buffer
	n, err := io.CopyN(&buf, rand.Reader, nBytes)
	if err != nil || n != nBytes {
		return nil, fmt.Errorf("failed to get random data (%d want %d): %v", n, nBytes, err)
	}
	return buf.Bytes(), nil
}

func TestReadFrom(t *testing.T) {
	const dstKey = "dstkey"

	// Get some random data, of a large enough size to require multiple
	// reads/writes given our buffer size of 1024.
	data, err := randomData(1024*10 + 10)

	bucket := memblob.OpenBucket(nil)
	defer bucket.Close()

	// Create a blob.Writer and write to it using ReadFrom given a buffer
	// holding the random data.
	ctx := context.Background()
	w, err := bucket.NewWriter(ctx, dstKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	n, err := w.ReadFrom(bytes.NewBuffer(data))
	if err != nil || n != int64(len(data)) {
		t.Fatalf("failed to ReadFrom (%d want %d): %v", n, len(data), err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	// Verify the data was copied correctly.
	got, err := bucket.ReadAll(ctx, dstKey)
	if err != nil {
		t.Fatal(err)
	}
	if !cmp.Equal(got, data) {
		t.Errorf("got %v, want %v", got, data)
	}
}

// Ensure that blob.Reader implements io.WriterTo.
var _ io.WriterTo = &blob.Reader{}

// Ensure that blob.Writer implements io.ReaderFrom.
var _ io.ReaderFrom = &blob.Writer{}

func TestWriteTo(t *testing.T) {
	const srcKey = "srckey"

	// Get some random data, of a large enough size to require multiple
	// reads/writes given our buffer size of 1024.
	data, err := randomData(1024*10 + 10)

	bucket := memblob.OpenBucket(nil)
	defer bucket.Close()

	// Write the data to a key.
	ctx := context.Background()
	if err := bucket.WriteAll(ctx, srcKey, data, nil); err != nil {
		t.Fatal(err)
	}

	// Create a blob.Reader for that key and read from it, writing to a buffer.
	r, err := bucket.NewReader(ctx, srcKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	n, err := r.WriteTo(&buf)
	if err != nil || n != int64(len(data)) {
		t.Fatalf("failed to WriteTo (%d want %d): %v", n, len(data), err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}

	// Verify the data was copied correctly.
	got := buf.Bytes()
	if !cmp.Equal(got, data) {
		t.Errorf("got %v, want %v", got, data)
	}
}
