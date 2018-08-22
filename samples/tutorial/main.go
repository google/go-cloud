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

// Command upload saves files to blob storage on GCP and AWS.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
)

func main() {
	// Define our input.
	cloud := flag.String("cloud", "", "Cloud storage to use")
	bucketName := flag.String("bucket", "go-cloud-bucket", "Name of bucket")

	flag.Parse()
	if flag.NArg() != 1 {
		log.Fatal("Failed to provide file to upload")
	}
	file := flag.Arg(0)

	ctx := context.Background()
	// Open a connection to the bucket.
	b, err := setupBucket(context.Background(), *cloud, *bucketName)
	if err != nil {
		log.Fatalf("Failed to setup bucket: %s", err)
	}

	// Prepare the file for upload.
	data, err := ioutil.ReadFile(file)
	if err != nil {
		log.Fatalf("Failed to read file: %s", err)
	}

	w, err := b.NewWriter(ctx, file, nil)
	if err != nil {
		log.Fatalf("Failed to obtain writer: %s", err)
	}
	_, err = w.Write(data)
	if err != nil {
		log.Fatalf("Failed to write to bucket: %s", err)
	}
	if err = w.Close(); err != nil {
		log.Fatalf("Failed to close: %s", err)
	}

	if *cloud == "azure" {
		// Read the blob content

		buf := make([]byte, 256)
		r, rdrErr := b.NewRangeReader(ctx, file, 100, -1)

		if rdrErr == nil {
			fmt.Println(r.ContentType())
			fmt.Println(r.Size())
			io.CopyBuffer(os.Stdout, r, buf)
		}

		// Delete the blob
		err := b.Delete(ctx, file)
		if  err != nil {
			log.Fatalf("Failed to delete: %s", err)
		}
	}
}

// to help test Azure
func init() {
	os.Setenv("AZURE_CLIENT_ID", "")
	os.Setenv("AZURE_TENANT_ID", "")
	os.Setenv("AZURE_CLIENT_SECRET", "")
	os.Setenv("SUBSCRIPTION_ID", "")
	os.Setenv("RESOURCE_GROUP_NAME", "")
	os.Setenv("STORAGE_ACCOUNT_NAME", "")
}
