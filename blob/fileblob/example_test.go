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

package fileblob_test

import (
	"context"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gocloud.dev/blob"
	"gocloud.dev/blob/fileblob"
)

func Example() {
	// For this example, create a temporary directory.
	dir, err := ioutil.TempDir("", "go-cloud-fileblob-example")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// Create a file-based bucket.
	bucket, err := fileblob.OpenBucket(dir, nil)
	if err != nil {
		log.Fatal(err)
	}
	_ = bucket
}

func ExampleURLOpener() {
	// For this example, create a temporary directory.
	dir, err := ioutil.TempDir("", "go-cloud-fileblob-example")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// Create a URL mux with the fileblob.URLOpener.
	// This would typically happen once in your application.
	mux := blob.NewURLMux(map[string]blob.BucketURLOpener{
		fileblob.Scheme: &fileblob.URLOpener{},
	})

	// Open a bucket from a URL.
	bucket, err := mux.Open(context.Background(), "file:///"+strings.Replace(dir, string(filepath.Separator), "/", -1))
	if err != nil {
		log.Fatal(err)
	}
	_ = bucket
}
