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
	"io/ioutil"
	"log"
	"os"
	"path"

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
	_, err = fileblob.OpenBucket(dir, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Output:
}

func Example_open() {
	// For this example, create a temporary directory.
	dir, err := ioutil.TempDir("", "go-cloud-fileblob-example")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	_, err = blob.Open(context.Background(), path.Join("file://", dir))
	if err != nil {
		log.Fatal(err)
	}

	// Output:
}
