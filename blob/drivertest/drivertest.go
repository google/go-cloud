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

// Package drivertest provides a conformance test for implementations of
// driver.
package drivertest

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/google/go-cloud/blob"
	"github.com/google/go-cmp/cmp"
)

// Harness descibes the functionality test harnesses must provide to run
// conformance tests.
type Harness interface {
	// MakeBucket creates a *blob.Bucket to test.
	// Multiple calls to MakeBucket during a test run must refer to	the
	// same storage bucket; i.e., a blob created using one *blob.Bucket must
	// be readable by a subsequent *blob.Bucket.
	MakeBucket(ctx context.Context) (*blob.Bucket, error)
	Close()
}

// HarnessMaker describes functions that construct a harness for running tests.
// It is called exactly once per test; Harness.Close() will be called when the test is complete.
type HarnessMaker func(ctx context.Context, t *testing.T) (Harness, error)

// RunConformanceTests runs conformance tests for provider implementations
// of blob.
// pathToTestdata is a (possibly relative) path to a directory containing
// blob/testdata/* (e.g., test-small.txt).
func RunConformanceTests(t *testing.T, newHarness HarnessMaker, pathToTestdata string) {
	t.Run("TestRead", func(t *testing.T) {
		testRead(t, newHarness)
	})
	t.Run("TestAttributes", func(t *testing.T) {
		testAttributes(t, newHarness)
	})
	t.Run("TestWrite", func(t *testing.T) {
		testWrite(t, newHarness, pathToTestdata)
	})
	t.Run("TestDelete", func(t *testing.T) {
		testDelete(t, newHarness)
	})
}

// testRead tests the functionality of NewReader, NewRangeReader, and Reader.
func testRead(t *testing.T, newHarness HarnessMaker) {
	const key = "blob-for-reading"
	content := []byte("abcdefghijklmnopqurstuvwxyz")
	contentSize := int64(len(content))

	tests := []struct {
		name           string
		key            string
		offset, length int64
		want           []byte
		wantReadSize   int64
		wantErr        bool
	}{
		{
			name:    "read of nonexistent key fails",
			key:     "key-does-not-exist",
			length:  -1,
			wantErr: true,
		},
		{
			name:    "length 0 read of nonexistent key fails",
			key:     "key-does-not-exist",
			wantErr: true,
		},
		{
			name:    "negative offset fails",
			key:     key,
			offset:  -1,
			wantErr: true,
		},
		{
			name:         "read from positive offset to end",
			key:          key,
			offset:       10,
			length:       -1,
			want:         content[10:],
			wantReadSize: contentSize - 10,
		},
		{
			name:         "read a part in middle",
			key:          key,
			offset:       10,
			length:       5,
			want:         content[10:15],
			wantReadSize: 5,
		},
		{
			name:         "read in full",
			key:          key,
			offset:       0,
			length:       -1,
			want:         content,
			wantReadSize: contentSize,
		},
	}

	ctx := context.Background()

	// Creates a blob for sub-tests below.
	init := func(t *testing.T) (*blob.Bucket, func()) {
		h, err := newHarness(ctx, t)
		if err != nil {
			t.Fatal(err)
		}

		b, err := h.MakeBucket(ctx)
		if err != nil {
			t.Fatal(err)
		}
		w, err := b.NewWriter(ctx, key, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(content)
		if err == nil {
			err = w.Close()
		}
		if err != nil {
			t.Fatal(err)
		}
		return b, func() {
			_ = b.Delete(ctx, key)
			h.Close()
		}
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, done := init(t)
			defer done()

			r, err := b.NewRangeReader(ctx, tc.key, tc.offset, tc.length)
			if (err != nil) != tc.wantErr {
				t.Errorf("got err %v want error %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			defer r.Close()
			// Make the buffer bigger than needed to make sure we actually only read
			// the expected number of bytes.
			got := make([]byte, tc.wantReadSize+10)
			n, err := r.Read(got)
			if err != nil {
				t.Errorf("unexpected error during read: %v", err)
			}
			if int64(n) != tc.wantReadSize {
				t.Errorf("got read length %d want %d", n, tc.wantReadSize)
			}
			if !cmp.Equal(got[:tc.wantReadSize], tc.want) {
				t.Errorf("got %q want %q", string(got), string(tc.want))
			}
			if r.Size() != contentSize {
				t.Errorf("got size %d want %d", r.Size(), contentSize)
			}
			r.Close()
		})
	}
}

// testAttributes tests the behavior of attributes returned by Reader.
func testAttributes(t *testing.T, newHarness HarnessMaker) {
	const (
		key         = "blob-for-attributes"
		contentType = "text/plain"
	)
	content := []byte("Hello World!")

	ctx := context.Background()

	// Creates a blob for sub-tests below.
	init := func(t *testing.T) (*blob.Bucket, func()) {
		h, err := newHarness(ctx, t)
		if err != nil {
			t.Fatal(err)
		}
		b, err := h.MakeBucket(ctx)
		if err != nil {
			t.Fatal(err)
		}
		opts := &blob.WriterOptions{
			ContentType: contentType,
		}
		w, err := b.NewWriter(ctx, key, opts)
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(content)
		if err == nil {
			err = w.Close()
		}
		if err != nil {
			t.Fatal(err)
		}
		return b, func() {
			_ = b.Delete(ctx, key)
			h.Close()
		}
	}

	for _, rLen := range []int64{0, -1} {
		t.Run(fmt.Sprintf("ReadLength %d", rLen), func(t *testing.T) {

			t.Run("ContentType", func(t *testing.T) {
				b, done := init(t)
				defer done()

				r, err := b.NewRangeReader(ctx, key, 0, 0)
				if err != nil {
					t.Fatalf("failed NewRangeReader: %v", err)
				}
				defer r.Close()
				if r.ContentType() != contentType {
					t.Errorf("got ContentType %q want %q", r.ContentType(), contentType)
				}
			})

			t.Run("Size", func(t *testing.T) {
				b, done := init(t)
				defer done()

				r, err := b.NewRangeReader(ctx, key, 0, rLen)
				if err != nil {
					t.Fatalf("failed NewRangeReader: %v", err)
				}
				defer r.Close()
				if r.Size() != int64(len(content)) {
					t.Errorf("got Size %d want %d", r.Size(), len(content))
				}
			})

			t.Run("ModTime", func(t *testing.T) {
				b, done := init(t)
				defer done()

				r, err := b.NewRangeReader(ctx, key, 0, rLen)
				if err != nil {
					t.Fatalf("failed NewRangeReader: %v", err)
				}
				defer r.Close()
				t1 := r.ModTime()
				if t1.IsZero() {
					// This provider doesn't support ModTime.
					// TODO(issue #315): There should be a way to tell if the provider
					// is supposed to return it.
					return
				}
				w, err := b.NewWriter(ctx, key, nil)
				if err != nil {
					t.Fatalf("failed NewWriter: %v", err)
				}
				if _, err := w.Write(content); err != nil {
					t.Fatalf("failed Write content: %v", err)
				}
				if err = w.Close(); err != nil {
					t.Errorf("failed Close Writer: %v", err)
				}
				r2, err := b.NewRangeReader(ctx, key, 0, rLen)
				if err != nil {
					t.Errorf("failed NewRangeReader#2: %v", err)
				}
				defer r2.Close()
				t2 := r2.ModTime()
				if t2.Before(t1) {
					t.Errorf("ModTime %v is before %v", t2, t1)
				}
			})
		})
	}
}

// loadTestFile loads a file from the blob/testdata/ directory.
// TODO(rvangent): Consider using go-bindata to inline these as source code.
func loadTestFile(t *testing.T, pathToTestdata, filename string) []byte {
	data, err := ioutil.ReadFile(filepath.Join(pathToTestdata, filename))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// testWrite tests the functionality of NewWriter and Writer.
func testWrite(t *testing.T, newHarness HarnessMaker, pathToTestdata string) {
	const key = "blob-for-reading"
	smallText := loadTestFile(t, pathToTestdata, "test-small.txt")
	mediumHTML := loadTestFile(t, pathToTestdata, "test-medium.html")
	largeJpg := loadTestFile(t, pathToTestdata, "test-large.jpg")

	tests := []struct {
		name            string
		key             string
		content         []byte
		contentType     string
		firstChunk      int
		wantContentType string
		wantErr         bool
	}{
		{
			name:    "write to empty key fails",
			wantErr: true,
		},
		{
			name: "no write then close results in empty blob",
			key:  key,
		},
		{
			name:        "invalid ContentType fails",
			key:         key,
			contentType: "application/octet/stream",
			wantErr:     true,
		},
		{
			name:            "ContentType is discovered if not provided",
			key:             key,
			content:         mediumHTML,
			wantContentType: "text/html",
		},
		{
			name:            "write with explicit ContentType overrides discovery",
			key:             key,
			content:         mediumHTML,
			contentType:     "application/json",
			wantContentType: "application/json",
		},
		{
			name:            "a small text file",
			key:             key,
			content:         smallText,
			wantContentType: "text/html",
		},
		{
			name:            "a large jpg file",
			key:             key,
			content:         largeJpg,
			wantContentType: "image/jpg",
		},
		{
			name:            "a large jpg file written in two chunks",
			key:             key,
			firstChunk:      10,
			content:         largeJpg,
			wantContentType: "image/jpg",
		},
		// TODO(issue #304): Fails for GCS.
		/*
			{
				name:            "ContentType is parsed and reformatted",
				key:             key,
				content:         []byte("foo"),
				contentType:     `FORM-DATA;name="foo"`,
				wantContentType: `form-data; name=foo`,
			},
		*/
	}

	ctx := context.Background()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, err := newHarness(ctx, t)
			if err != nil {
				t.Fatal(err)
			}
			defer h.Close()
			b, err := h.MakeBucket(ctx)
			if err != nil {
				t.Fatal(err)
			}

			// Write the content.
			opts := &blob.WriterOptions{
				ContentType: tc.contentType,
			}
			w, err := b.NewWriter(ctx, tc.key, opts)
			if err == nil {
				if len(tc.content) > 0 {
					if tc.firstChunk == 0 {
						// Write the whole thing.
						_, err = w.Write(tc.content)
					} else {
						// Write it in 2 chunks.
						_, err = w.Write(tc.content[:tc.firstChunk])
						if err == nil {
							_, err = w.Write(tc.content[tc.firstChunk:])
						}
					}
				}
				if err == nil {
					err = w.Close()
				}
			}
			if (err != nil) != tc.wantErr {
				t.Errorf("NewWriter or Close got err %v want error %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			defer func() { _ = b.Delete(ctx, tc.key) }()

			// Read it back.
			r, err := b.NewReader(ctx, tc.key)
			if err != nil {
				t.Fatalf("failed to NewReader: %v", err)
			}
			defer r.Close()
			buf := make([]byte, len(tc.content))
			_, err = r.Read(buf)
			if err != nil {
				t.Errorf("failed to Read: %v", err)
			}
			if !bytes.Equal(buf, tc.content) {
				if len(buf) < 100 && len(tc.content) < 100 {
					t.Errorf("read didn't match write; got \n%s\n want \n%s", string(buf), string(tc.content))
				} else {
					t.Error("read didn't match write, content too large to display")
				}
			}
		})
	}
}

// testDelete tests the functionality of Delete.
func testDelete(t *testing.T, newHarness HarnessMaker) {
	const key = "blob-for-deleting"

	ctx := context.Background()
	t.Run("NonExistentFails", func(t *testing.T) {
		h, err := newHarness(ctx, t)
		if err != nil {
			t.Fatal(err)
		}
		defer h.Close()
		b, err := h.MakeBucket(ctx)
		if err != nil {
			t.Fatal(err)
		}

		err = b.Delete(ctx, "does-not-exist")
		if err == nil {
			t.Errorf("want error, got nil")
		} else if !blob.IsNotExist(err) {
			t.Errorf("want IsNotExist error, got %v", err)
		}
	})

	t.Run("Works", func(t *testing.T) {
		h, err := newHarness(ctx, t)
		if err != nil {
			t.Fatal(err)
		}
		defer h.Close()
		b, err := h.MakeBucket(ctx)
		if err != nil {
			t.Fatal(err)
		}

		// Create the blob.
		writer, err := b.NewWriter(ctx, key, nil)
		if err != nil {
			t.Fatalf("failed to NewWriter: %v", err)
		}
		_, err = io.WriteString(writer, "Hello world")
		if err != nil {
			t.Errorf("failed to write: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Errorf("failed to close Writer: %v", err)
		}
		// Delete it.
		if err := b.Delete(ctx, key); err != nil {
			t.Errorf("got unexpected error deleting blob: %v", err)
		}
		// Subsequent read fails with IsNotExist.
		_, err = b.NewReader(ctx, key)
		if err == nil {
			t.Errorf("read after delete want error, got nil")
		} else if !blob.IsNotExist(err) {
			t.Errorf("read after delete want IsNotExist error, got %v", err)
		}
		// Subsequent delete also fails.
		err = b.Delete(ctx, key)
		if err == nil {
			t.Errorf("delete after delete want error, got nil")
		} else if !blob.IsNotExist(err) {
			t.Errorf("delete after delete want IsNotExist error, got %v", err)
		}
	})
}
