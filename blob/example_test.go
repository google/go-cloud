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

package blob_test

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/fileblob"
)

func ExampleBucket_NewReader() {
	// Connect to a bucket when your program starts up.
	// This example uses the file-based implementation.
	dir, cleanup := newTempDir()
	defer cleanup()
	// Write a file to read using the bucket.
	err := ioutil.WriteFile(filepath.Join(dir, "foo.txt"), []byte("Hello, World!\n"), 0666)
	if err != nil {
		log.Fatal(err)
	}
	// Create the file-based bucket.
	bucket, err := fileblob.NewBucket(dir)
	if err != nil {
		log.Fatal(err)
	}

	// Open a reader using the blob's key.
	ctx := context.Background()
	r, err := bucket.NewReader(ctx, "foo.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()
	// The blob reader implements io.Reader, so we can use any function that
	// accepts an io.Reader.
	if _, err := io.Copy(os.Stdout, r); err != nil {
		log.Fatal(err)
	}

	// Output:
	// Hello, World!
}

func ExampleBucket_NewRangeReader() {
	// Connect to a bucket when your program starts up.
	// This example uses the file-based implementation.
	dir, cleanup := newTempDir()
	defer cleanup()
	// Write a file to read using the bucket.
	err := ioutil.WriteFile(filepath.Join(dir, "foo.txt"), []byte("Hello, World!\n"), 0666)
	if err != nil {
		log.Fatal(err)
	}
	// Create the file-based bucket.
	bucket, err := fileblob.NewBucket(dir)
	if err != nil {
		log.Fatal(err)
	}

	// Open a reader using the blob's key at a specific offset at length.
	ctx := context.Background()
	r, err := bucket.NewRangeReader(ctx, "foo.txt", 1, 4)
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()
	// The blob reader implements io.Reader, so we can use any function that
	// accepts an io.Reader.
	if _, err := io.Copy(os.Stdout, r); err != nil {
		log.Fatal(err)
	}

	// Output:
	// ello
}

func ExampleBucket_NewWriter() {
	// Connect to a bucket when your program starts up.
	// This example uses the file-based implementation.
	dir, cleanup := newTempDir()
	defer cleanup()
	bucket, err := fileblob.NewBucket(dir)
	if err != nil {
		log.Fatal(err)
	}

	// Open a writer using the key "foo.txt" and the default options.
	ctx := context.Background()
	// fileblob doesn't support custom content-type yet, see
	// https://github.com/google/go-cloud/issues/111.
	w, err := bucket.NewWriter(ctx, "foo.txt", &blob.WriterOptions{
		ContentType: "application/octet-stream",
	})
	if err != nil {
		log.Fatal(err)
	}
	// The blob writer implements io.Writer, so we can use any function that
	// accepts an io.Writer. A writer must always be closed.
	_, printErr := fmt.Fprintln(w, "Hello, World!")
	closeErr := w.Close()
	if printErr != nil {
		log.Fatal(printErr)
	}
	if closeErr != nil {
		log.Fatal(closeErr)
	}
	// Copy the written blob to stdout.
	r, err := bucket.NewReader(ctx, "foo.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()
	if _, err := io.Copy(os.Stdout, r); err != nil {
		log.Fatal(err)
	}

	// Output:
	// Hello, World!
}

func newTempDir() (string, func()) {
	dir, err := ioutil.TempDir("", "go-cloud-blob-example")
	if err != nil {
		panic(err)
	}
	return dir, func() { os.RemoveAll(dir) }
}
