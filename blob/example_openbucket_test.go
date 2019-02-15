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
	"context"
	"fmt"
	"log"
	"net/url"
	"path/filepath"
	"strings"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/fileblob"
)

func ExampleOpenBucket() {

	// Connect to a bucket using a URL.
	// This example uses "fileblob", the file-based implementation.
	// We need to add a blank import line to register the fileblob provider's
	// URLOpener, which implements blob.BucketURLOpener:
	// import _ "gocloud.dev/blob/fileblob"
	// fileblob registers for the "file" scheme.
	dir, cleanup := newTempDir()
	defer cleanup()

	// Ensure the path has a leading slash; fileblob ignores the URL's
	// Host field, so URLs should always start with "file:///". On
	// Windows, the leading "/" will be stripped, so "file:///c:/foo"
	// will refer to c:/foo.
	urlpath := url.PathEscape(filepath.ToSlash(dir))
	if !strings.HasPrefix(urlpath, "/") {
		urlpath = "/" + urlpath
	}
	ctx := context.Background()
	b, err := blob.OpenBucket(ctx, "file://"+urlpath)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Got a bucket for valid path")

	// Now we can use b to read or write files to the container.
	if err := b.WriteAll(ctx, "my-key", []byte("hello world"), nil); err != nil {
		log.Fatal(err)
	}
	data, err := b.ReadAll(ctx, "my-key")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(data))
	// Output:
	// Got a bucket for valid path
	// hello world
}
