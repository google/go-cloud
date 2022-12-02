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

package azureblob_test

import (
	"context"
	"log"

	"gocloud.dev/blob"
	"gocloud.dev/blob/azureblob"
)

func ExampleOpenBucket() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	const (
		// The storage container to access.
		containerName = "my-container"
	)

	// Construct the service URL.
	// There are many forms of service URLs, see ServiceURLOptions.
	opts := azureblob.NewDefaultServiceURLOptions()
	serviceURL, err := azureblob.NewServiceURL(opts)
	if err != nil {
		log.Fatal(err)
	}

	// There are many ways to authenticate to Azure.
	// This approach uses environment variables as described in azureblob package
	// documentation.
	// For example, to use shared key authentication, you would set
	// AZURE_STORAGE_ACCOUNT and AZURE_STORAGE_KEY.
	// To use a SAS token, you would set AZURE_STORAGE_ACCOUNT and AZURE_STORAGE_SAS_TOKEN.
	// You can also construct a client using the azblob constructors directly, like
	// azblob.NewServiceClientWithSharedKey.
	client, err := azureblob.NewDefaultClient(serviceURL, containerName)
	if err != nil {
		log.Fatal(err)
	}

	// Create a *blob.Bucket.
	b, err := azureblob.OpenBucket(ctx, client, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer b.Close()

	// Now we can use b to read or write files to the container.
	data, err := b.ReadAll(ctx, "my-key")
	if err != nil {
		log.Fatal(err)
	}
	_ = data
}

func Example_openBucketFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/blob/azureblob"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// blob.OpenBucket creates a *blob.Bucket from a URL.
	// This URL will open the container "my-container" using default
	// credentials found in environment variables as documented in
	// the package.
	// Assuming AZURE_STORAGE_ACCOUNT is set to "myaccount",
	// and other options aren't set, the service URL will look like:
	// "https://myaccount.blob.core.windows.net/my-container".
	bucket, err := blob.OpenBucket(ctx, "azblob://my-container")
	if err != nil {
		log.Fatal(err)
	}
	defer bucket.Close()

	// Another example, against a local emulator.
	// Assuming AZURE_STORAGE_ACCOUNT is set to "myaccount",
	// the service URL will look like:
	// "http://localhost:10001/myaccount/my-container".
	localbucket, err := blob.OpenBucket(ctx, "azblob://my-container?protocol=http&domain=localhost:10001")
	if err != nil {
		log.Fatal(err)
	}
	defer localbucket.Close()
}
