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

	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"gocloud.dev/blob"
	"gocloud.dev/blob/azureblob"
)

func ExampleOpenBucket() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	const (
		// Fill in with your Azure Storage Account and Access Key.
		accountName azureblob.AccountName = "my-account"
		accountKey  azureblob.AccountKey  = "my-account-key"
		// Fill in with the storage container to access.
		containerName = "my-container"
	)

	// Create a credentials object.
	credential, err := azureblob.NewCredential(accountName, accountKey)
	if err != nil {
		log.Fatal(err)
	}

	// Create a Pipeline, using whatever PipelineOptions you need.
	pipeline := azureblob.NewPipeline(credential, azblob.PipelineOptions{})

	// Create a *blob.Bucket.
	// The credential Option is required if you're going to use blob.SignedURL.
	bucket, err := azureblob.OpenBucket(ctx, pipeline, accountName, containerName,
		&azureblob.Options{Credential: credential})
	if err != nil {
		log.Fatal(err)
	}
	defer bucket.Close()
}

func ExampleOpenBucket_usingSASToken() {
	const (
		// Your Azure Storage Account and SASToken.
		accountName = azureblob.AccountName("my-account")
		sasToken    = azureblob.SASToken("my-SAS-token")
		// The storage container to access.
		containerName = "my-container"
	)

	// Since we're using a SASToken, we can use anonymous credentials.
	credential := azblob.NewAnonymousCredential()

	// Create a Pipeline, using whatever PipelineOptions you need.
	pipeline := azureblob.NewPipeline(credential, azblob.PipelineOptions{})

	// Create a *blob.Bucket.
	// Note that we're not supplying azureblob.Options.Credential, so SignedURL
	// won't work. To use SignedURL, you need a real credential (see the other
	// example).
	ctx := context.Background()
	b, err := azureblob.OpenBucket(ctx, pipeline, accountName, containerName, &azureblob.Options{SASToken: sasToken})
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
	// credentials found in the environment variables
	// AZURE_STORAGE_ACCOUNT plus at least one of AZURE_STORAGE_KEY
	// and AZURE_STORAGE_SAS_TOKEN.
	bucket, err := blob.OpenBucket(ctx, "azblob://my-container")
	if err != nil {
		log.Fatal(err)
	}
	defer bucket.Close()
}

func ExampleOpenBucket_usingAADCredentials() {
	const (
		// Your Azure Storage Account.
		accountName = azureblob.AccountName("my-account")

		// Your Azure AAD Service Principal with access to the storage account.
		// https://docs.microsoft.com/en-us/azure/storage/common/storage-auth-aad-app
		clientID     = "123"
		clientSecret = "456"
		tenantID     = "789"

		// The storage container to access.
		containerName = "my-container"
	)

	// Get an Oauth2 token for the account for use with Azure Storage.
	ccc := auth.NewClientCredentialsConfig(clientID, clientSecret, tenantID)

	// Set the target resource to the Azure storage. This is available as a
	// constant using "azure.PublicCloud.ResourceIdentifiers.Storage".
	ccc.Resource = "https://storage.azure.com/"
	token, err := ccc.ServicePrincipalToken()
	if err != nil {
		log.Fatal(err)
	}

	// Refresh OAuth2 token.
	if err := token.RefreshWithContext(context.Background()); err != nil {
		log.Fatal(err)
	}

	// Create the credential using the OAuth2 token.
	credential := azblob.NewTokenCredential(token.OAuthToken(), nil)

	// Create a Pipeline, using whatever PipelineOptions you need.
	pipeline := azureblob.NewPipeline(credential, azblob.PipelineOptions{})

	// Create a *blob.Bucket.
	// Note that we're not supplying azureblob.Options.Credential, so SignedURL
	// won't work. To use SignedURL, you need a real credential (see the other
	// example).
	ctx := context.Background()
	b, err := azureblob.OpenBucket(ctx, pipeline, accountName, containerName, new(azureblob.Options))
	if err != nil {
		log.Fatal(err)
	}
	defer b.Close()
}
