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

package fileblob_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gocloud.dev/blob"
	"gocloud.dev/blob/fileblob"
)

func ExampleOpenBucket() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.

	// The directory you pass to fileblob.OpenBucket must exist first.
	const myDir = "path/to/local/directory"
	if err := os.MkdirAll(myDir, 0777); err != nil {
		log.Fatal(err)
	}

	// Create a file-based bucket.
	bucket, err := fileblob.OpenBucket(myDir, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer bucket.Close()
}

func Example_openBucketFromURL() {
	// Create a temporary directory.
	dir, err := ioutil.TempDir("", "go-cloud-fileblob-example")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// On Unix, append the dir to "file://".
	// On Windows, convert "\" to "/" and add a leading "/":
	dirpath := filepath.ToSlash(dir)
	if os.PathSeparator != '/' && !strings.HasPrefix(dirpath, "/") {
		dirpath = "/" + dirpath
	}

	// blob.OpenBucket creates a *blob.Bucket from a URL.
	ctx := context.Background()
	b, err := blob.OpenBucket(ctx, "file://"+dirpath)
	if err != nil {
		log.Fatal(err)
	}
	defer b.Close()

	// Now we can use b to read or write files to the container.
	err = b.WriteAll(ctx, "my-key", []byte("hello world"), nil)
	if err != nil {
		log.Fatal(err)
	}
	data, err := b.ReadAll(ctx, "my-key")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(data))

	// Output:
	// hello world
}
