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
	// This example uses the file-based implementation, which registers for
	// the "file" scheme.
	dir, cleanup := newTempDir()
	defer cleanup()

	ctx := context.Background()

	// The blob package is designed for using with a specific provider package.
	// Here blob.OpenBucket needs a provider that implements blob.BucketURLOpener
	// to work. In this example, we use blob/fileblob, so we need to add a blank
	// import line to register the fileblob provider:
	// import _ "gocloud.dev/blob/fileblob"
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
