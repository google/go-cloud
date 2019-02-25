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

package gcsblob_test

import (
	"context"
	"log"

	"cloud.google.com/go/storage"
	"gocloud.dev/blob"
	"gocloud.dev/blob/gcsblob"
	"gocloud.dev/gcp"
)

func Example_read() {
	// Your GCP credentials.
	// See https://cloud.google.com/docs/authentication/production
	// for more info on alternatives.
	ctx := context.Background()
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Create an HTTP client.
	// This example uses the default HTTP transport and the credentials created
	// above.
	client, err := gcp.NewHTTPClient(gcp.DefaultTransport(), gcp.CredentialsTokenSource(creds))
	if err != nil {
		return
	}

	// Create a *blob.Bucket.
	b, err := gcsblob.OpenBucket(ctx, client, "my-bucket", nil)
	if err != nil {
		log.Fatal(err)
	}

	// Now we can use b to read or write files to the container.
	data, err := b.ReadAll(ctx, "my-key")
	if err != nil {
		log.Fatal(err)
	}
	_ = data
}

func Example_open() {
	ctx := context.Background()

	// Open creates a *blob.Bucket from a URL.
	// This URL will open the bucket "my-bucket" using default credentials.
	b, err := blob.OpenBucket(ctx, "gs://my-bucket")
	_ = b
	_ = err
}

func Example_As() {
	ctx := context.Background()

	// Open creates a *blob.Bucket from a URL.
	// This URL will open the bucket "my-bucket" using default credentials.
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
