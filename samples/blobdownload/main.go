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

// Command blobdownload downloads a blob from a supported provider and writes it to
// a local file.
package main

import (
	"context"
	"io/ioutil"
	"log"
	"os"

	"gocloud.dev/blob"

	// Import the blob packages we want to be able to open.
	_ "gocloud.dev/blob/azureblob"
	_ "gocloud.dev/blob/gcsblob"
	_ "gocloud.dev/blob/s3blob"
)

func main() {
	// Define our input.
	if len(os.Args) != 4 {
		log.Fatal("usage: blobdownload <bucket_url> <blob key> <local file to write to>")
	}
	bucketURL := os.Args[1]
	blobKey := os.Args[2]
	file := os.Args[3]

	ctx := context.Background()
	// Open a connection to the bucket.
	b, err := blob.OpenBucket(ctx, bucketURL)
	if err != nil {
		log.Fatalf("Failed to open bucket: %s", err)
	}
	defer b.Close()

	data, err := b.ReadAll(ctx, blobKey)
	if err != nil {
		log.Fatalf("Failed to read %q: %v", blobKey, err)
	}

	if err := ioutil.WriteFile(file, data, os.ModePerm); err != nil {
		log.Fatalf("Failed to write file: %v", err)
	}
}
