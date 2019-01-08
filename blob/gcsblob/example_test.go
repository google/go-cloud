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

package gcsblob_test

import (
	"context"
	"fmt"
	"log"

	"gocloud.dev/blob"
	"gocloud.dev/blob/gcsblob"
	"gocloud.dev/gcp"
	"golang.org/x/oauth2/google"
)

// jsonCreds is a fake GCP JSON credentials file.
const jsonCreds = `
{
  "type": "service_account",
  "project_id": "my-project-id"
}
`

func Example() {
	ctx := context.Background()

	// Get GCP credentials.
	// Here we use a fake JSON credentials file, but you could also use
	// gcp.DefaultCredentials(ctx) to use the default GCP credentials from
	// the environment.
	// See https://cloud.google.com/docs/authentication/production
	// for more info on alternatives.
	creds, err := google.CredentialsFromJSON(ctx, []byte(jsonCreds))
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
	_, err = b.ReadAll(ctx, "my-key")
	if err != nil {
		// This is expected due to the fake credentials we used above.
		fmt.Println("ReadAll failed due to invalid credentials")
	}

	// Output:
	// ReadAll failed due to invalid credentials
}

func ExampleURLOpener() {
	ctx := context.Background()

	// Get GCP credentials.
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Create an HTTP client.
	// This example uses the default HTTP transport and the credentials created
	// above.
	client, err := gcp.NewHTTPClient(gcp.DefaultTransport(), gcp.CredentialsTokenSource(creds))
	if err != nil {
		log.Fatal(err)
	}

	// Create a URL mux with the gcsblob.URLOpener.
	// This would typically happen once in your application.
	mux := blob.NewURLMux(map[string]blob.BucketURLOpener{
		gcsblob.Scheme: &gcsblob.URLOpener{Client: client},
	})

	// Open a bucket using the mux.
	bucket, err := mux.Open(ctx, "gs://my-bucket")
	if err != nil {
		log.Fatal(err)
	}
	_ = bucket
}
