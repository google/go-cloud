// Copyright 2018 The Go Cloud Development Kit Authors
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
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage"
	"gocloud.dev/blob"
	"gocloud.dev/blob/fileblob"
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
	bucket, err := fileblob.OpenBucket(dir, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Open a reader using the blob's key.
	ctx := context.Background()
	r, err := bucket.NewReader(ctx, "foo.txt", nil)
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
	bucket, err := fileblob.OpenBucket(dir, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Open a reader using the blob's key at a specific offset at length.
	ctx := context.Background()
	r, err := bucket.NewRangeReader(ctx, "foo.txt", 1, 4, nil)
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
	bucket, err := fileblob.OpenBucket(dir, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Open a writer using the key "foo.txt" and the default options.
	ctx := context.Background()
	w, err := bucket.NewWriter(ctx, "foo.txt", nil)
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
	r, err := bucket.NewReader(ctx, "foo.txt", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()
	if _, err := io.Copy(os.Stdout, r); err != nil {
		log.Fatal(err)
	}
	// Since we didn't specify a WriterOptions.ContentType for NewWriter, blob
	// auto-determined one using http.DetectContentType.
	fmt.Println(r.ContentType())

	// Output:
	// Hello, World!
	// text/plain; charset=utf-8
}

func Example() {
	// Connect to a bucket when your program starts up.
	// This example uses the file-based implementation in fileblob, and creates
	// a temporary directory to use as the root directory.
	dir, cleanup := newTempDir()
	defer cleanup()
	bucket, err := fileblob.OpenBucket(dir, nil)
	if err != nil {
		log.Fatal(err)
	}

	// We now have a *blob.Bucket! We can write our application using the
	// *blob.Bucket type, and have the freedom to change the initialization code
	// above to choose a different provider later.

	// In this example, we'll write a blob and then read it.
	ctx := context.Background()
	if err := bucket.WriteAll(ctx, "foo.txt", []byte("Go Cloud Development Kit"), nil); err != nil {
		log.Fatal(err)
	}
	b, err := bucket.ReadAll(ctx, "foo.txt")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(b))

	// Output:
	// Go Cloud Development Kit
}

func ExampleBucket_List() {
	// Connect to a bucket when your program starts up.
	// This example uses the file-based implementation.
	dir, cleanup := newTempDir()
	defer cleanup()

	// Create the file-based bucket.
	bucket, err := fileblob.OpenBucket(dir, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Create some blob objects for listing: "foo[0..4].txt".
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if err := bucket.WriteAll(ctx, fmt.Sprintf("foo%d.txt", i), []byte("Go Cloud Development Kit"), nil); err != nil {
			log.Fatal(err)
		}
	}

	// Iterate over them.
	// This will list the blobs created above because fileblob is strongly
	// consistent, but is not guaranteed to work on all providers.
	iter := bucket.List(nil)
	for {
		obj, err := iter.Next(ctx)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(obj.Key)
	}

	// Output:
	// foo0.txt
	// foo1.txt
	// foo2.txt
	// foo3.txt
	// foo4.txt
}

func ExampleBucket_List_withDelimiter() {
	// Connect to a bucket when your program starts up.
	// This example uses the file-based implementation.
	dir, cleanup := newTempDir()
	defer cleanup()

	// Create the file-based bucket.
	bucket, err := fileblob.OpenBucket(dir, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Create some blob objects in a hierarchy.
	ctx := context.Background()
	for _, key := range []string{
		"dir1/subdir/a.txt",
		"dir1/subdir/b.txt",
		"dir2/c.txt",
		"d.txt",
	} {
		if err := bucket.WriteAll(ctx, key, []byte("Go Cloud Development Kit"), nil); err != nil {
			log.Fatal(err)
		}
	}

	// list lists files in b starting with prefix. It uses the delimiter "/",
	// and recurses into "directories", adding 2 spaces to indent each time.
	// It will list the blobs created above because fileblob is strongly
	// consistent, but is not guaranteed to work on all providers.
	var list func(context.Context, *blob.Bucket, string, string)
	list = func(ctx context.Context, b *blob.Bucket, prefix, indent string) {
		iter := b.List(&blob.ListOptions{
			Delimiter: "/",
			Prefix:    prefix,
		})
		for {
			obj, err := iter.Next(ctx)
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("%s%s\n", indent, obj.Key)
			if obj.IsDir {
				list(ctx, b, obj.Key, indent+"  ")
			}
		}
	}
	list(ctx, bucket, "", "")

	// Output:
	// d.txt
	// dir1/
	//   dir1/subdir/
	//     dir1/subdir/a.txt
	//     dir1/subdir/b.txt
	// dir2/
	//   dir2/c.txt
}

func ExampleBucket_As() {
	// This example is specific to the gcsblob implementation; it demonstrates
	// access to the underlying cloud.google.com/go/storage.Client type.
	// The types exposed for As by gcsblob are documented in
	// https://godoc.org/gocloud.dev/blob/gcsblob#hdr-As

	// This URL will open the bucket "my-bucket" using default credentials.
	ctx := context.Background()
	b, err := blob.OpenBucket(ctx, "gs://my-bucket")
	if err != nil {
		log.Fatal(err)
	}

	// Bucket exposes the internal storage.Client type through the As method. Use
	// it to access a non-portable method of storage.Client.
	var gcsClient *storage.Client
	if b.As(&gcsClient) {
		email, err := gcsClient.ServiceAccount(ctx, "project-name")
		if err != nil {
			log.Fatal(err)
		}
		_ = email
	} else {
		log.Fatal("Unable to access storage.Client through Bucket.As")
	}
}

func ExampleOpenBucket() {
	// Connect to a bucket using a URL.
	// This example uses the file-based implementation, which registers for
	// the "file" scheme.
	dir, cleanup := newTempDir()
	defer cleanup()

	ctx := context.Background()
	if _, err := blob.OpenBucket(ctx, "file:///nonexistentpath"); err == nil {
		log.Fatal("Expected an error opening nonexistent path")
	}
	fmt.Println("Got expected error opening a nonexistent path")

	// Ensure the path has a leading slash; fileblob ignores the URL's
	// Host field, so URLs should always start with "file:///". On
	// Windows, the leading "/" will be stripped, so "file:///c:/foo"
	// will refer to c:/foo.
	urlpath := url.PathEscape(filepath.ToSlash(dir))
	if !strings.HasPrefix(urlpath, "/") {
		urlpath = "/" + urlpath
	}
	if _, err := blob.OpenBucket(ctx, "file://"+urlpath); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Got a bucket for valid path")

	// Output:
	// Got expected error opening a nonexistent path
	// Got a bucket for valid path
}

func newTempDir() (string, func()) {
	dir, err := ioutil.TempDir("", "go-cloud-blob-example")
	if err != nil {
		panic(err)
	}
	return dir, func() { os.RemoveAll(dir) }
}
