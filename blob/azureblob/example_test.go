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
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/Azure/azure-storage-blob-go/azblob"
	"gocloud.dev/blob"
	"gocloud.dev/blob/azureblob"
)

func Example() {
	ctx := context.Background()
	// A fake account name and key. The key must be base64 encoded;
	// real Azure Storage Access Keys are already base64 encoded.
	accountName := azureblob.AccountName("myaccount")
	accountKey := azureblob.AccountKey(base64.StdEncoding.EncodeToString([]byte("FAKECREDS")))
	bucketName := "my-bucket"

	// Create a credentials object.
	credential, err := azureblob.NewCredential(accountName, accountKey)
	if err != nil {
		log.Fatal(err)
	}

	// Create a Pipeline, using whatever PipelineOptions you need.
	// This example overrides the default retry policy so calls can return promptly
	// for this test. Please review the timeout guidelines and set accordingly.
	// See https://docs.microsoft.com/en-us/rest/api/storageservices/setting-timeouts-for-blob-service-operations for more information.
	popts := azblob.PipelineOptions{
		Retry: azblob.RetryOptions{
			Policy:        azblob.RetryPolicyFixed,
			TryTimeout:    5 * time.Second,
			MaxTries:      1,
			RetryDelay:    0 * time.Second,
			MaxRetryDelay: 0 * time.Second,
		},
	}
	pipeline := azureblob.NewPipeline(credential, popts)

	// Create a *blob.Bucket.
	opts := &azureblob.Options{
		// This credential is required only if you're going to use the SignedURL
		// function.
		Credential: credential,
	}
	b, err := azureblob.OpenBucket(ctx, pipeline, accountName, bucketName, opts)
	if err != nil {
		log.Fatal(err)
		return
	}

	// Now we can use b!
	_, err = b.ReadAll(ctx, "my-key")
	if err != nil {
		// This is expected due to the fake credentials we used above.
		fmt.Println("ReadAll failed due to invalid credentials")
	}

	// Output:
	// ReadAll failed due to invalid credentials
}
func Example_sasToken() {
	ctx := context.Background()
	// A fake account name and SASToken.
	accountName := azureblob.AccountName("myaccount")
	sasToken := azureblob.SASToken("https://myaccount.blob.core.windows.net/sascontainer/sasblob.txt?sv=2015-04-05&st=2015-04-29T22%3A18%3A26Z&se=2015-04-30T02%3A23%3A26Z&sr=b&sp=rw&sip=168.1.5.60-168.1.5.70&spr=https&sig=Z%2FRHIX5Xcg0Mq2rqI3OlWTjEg2tYkboXr1P9ZUXDtkk%3D")
	bucketName := "my-bucket"

	// Since we're using a SASToken, we can use anonymous credentials.
	credential := azblob.NewAnonymousCredential()

	// Create a Pipeline, using whatever PipelineOptions you need.
	// This example overrides the default retry policy so calls can return promptly
	// for this test. Please review the timeout guidelines and set accordingly.
	// See https://docs.microsoft.com/en-us/rest/api/storageservices/setting-timeouts-for-blob-service-operations for more information.
	popts := azblob.PipelineOptions{
		Retry: azblob.RetryOptions{
			Policy:        azblob.RetryPolicyFixed,
			TryTimeout:    5 * time.Second,
			MaxTries:      1,
			RetryDelay:    0 * time.Second,
			MaxRetryDelay: 0 * time.Second,
		},
	}
	pipeline := azureblob.NewPipeline(credential, popts)

	// Create a blob.Bucket.
	// Note that we're not supplying azureblob.Options.Credential, so SignedURL
	// won't work. To use SignedURL, you need a real credential (see the other
	// example).
	b, err := azureblob.OpenBucket(ctx, pipeline, accountName, bucketName, &azureblob.Options{SASToken: sasToken})
	if err != nil {
		log.Fatal(err)
		return
	}

	// Now we can use b!
	_, err = b.ReadAll(ctx, "my-key")
	if err != nil {
		// This is expected due to the fake SAS token we used above.
		fmt.Println("ReadAll failed due to invalid SAS token")
	}

	// Output:
	// ReadAll failed due to invalid SAS token
}

func Example_open() {
	ctx := context.Background()

	// Open creates a *Bucket from a URL.
	// This URL will open the container "mycontainer" using default
	// credentials found in the environment variables
	// AZURE_STORAGE_ACCOUNT plus at least one of AZURE_STORAGE_KEY
	// and AZURE_STORAGE_SAS_TOKEN.
	_, err := blob.Open(ctx, "azblob://mycontainer")

	// Alternatively, you can use the query parameter "cred_path" to load
	// credentials from a file in JSON format.
	// See the package documentation for the credentials file schema.
	_, err = blob.Open(ctx, "azblob://mycontainer?cred_path=replace-with-path-to-credentials-file")
	if err != nil {
		// This is expected due to the invalid cred_path argument used above.
		fmt.Println("blob.Open failed due to invalid creds_path argument")
	}
	// Output:
	// blob.Open failed due to invalid creds_path argument
}

func Example_as() {
	ctx := context.Background()
	// A fake account name and key. The key must be base64 encoded;
	// real Azure Storage Access Keys are already base64 encoded.
	accountName := azureblob.AccountName("myaccount")
	accountKey := azureblob.AccountKey(base64.StdEncoding.EncodeToString([]byte("FAKECREDS")))
	bucketName := "my-bucket"

	// Create a credentials object.
	credential, err := azureblob.NewCredential(accountName, accountKey)
	if err != nil {
		log.Fatal(err)
	}

	// Create a Pipeline, using whatever PipelineOptions you need.
	// This example overrides the default retry policy so calls can return promptly
	// for this test. Please review the timeout guidelines and set accordingly.
	// See https://docs.microsoft.com/en-us/rest/api/storageservices/setting-timeouts-for-blob-service-operations for more information.
	popts := azblob.PipelineOptions{
		Retry: azblob.RetryOptions{
			Policy:        azblob.RetryPolicyFixed,
			TryTimeout:    5 * time.Second,
			MaxTries:      1,
			RetryDelay:    0 * time.Second,
			MaxRetryDelay: 0 * time.Second,
		},
	}
	pipeline := azureblob.NewPipeline(credential, popts)

	// Create a *blob.Bucket.
	opts := &azureblob.Options{
		// This credential is required only if you're going to use the SignedURL
		// function.
		Credential: credential,
	}
	b, err := azureblob.OpenBucket(ctx, pipeline, accountName, bucketName, opts)
	if err != nil {
		log.Fatal(err)
		return
	}

	// Create a *blob.Reader.
	r, err := b.NewReader(ctx, "key", &blob.ReaderOptions{})
	if err != nil {
		fmt.Println("ReadAll failed due to invalid credentials")

		// Due to the fake credentials used above, this test terminates here.
		return
	}

	// IMPORTANT: The calls below are intended to show how to use As() to obtain the Azure Blob Storage SDK types.
	// Due to the fake credentials used above, below calls are not executed.

	// Use Reader.As to obtain SDK type azblob.DownloadResponse.
	// See https://godoc.org/github.com/Azure/azure-storage-blob-go/azblob#DownloadResponse for more information.
	var nativeReader azblob.DownloadResponse
	if r.As(&nativeReader) {
	}

	// Use Attribute.As to obtain SDK type azblob.BlobGetPropertiesResponse.
	// See https://godoc.org/github.com/Azure/azure-storage-blob-go/azblob#BlobGetPropertiesResponse for more information.
	var nativeAttrs azblob.BlobGetPropertiesResponse
	attr, _ := b.Attributes(ctx, "key")
	if attr.As(&nativeAttrs) {
	}

	// Use Bucket.As to obtain SDK type azblob.ContainerURL.
	// See https://godoc.org/github.com/Azure/azure-storage-blob-go/azblob for more information.
	var nativeBucket *azblob.ContainerURL
	if b.As(&nativeBucket) {
	}

	// Use WriterOptions.BeforeWrite to obtain SDK type azblob.UploadStreamToBlockBlobOptions.
	// See https://godoc.org/github.com/Azure/azure-storage-blob-go/azblob#UploadStreamToBlockBlobOptions for more information.
	beforeWrite := func(as func(i interface{}) bool) error {
		var nativeWriteOptions *azblob.UploadStreamToBlockBlobOptions
		if as(&nativeWriteOptions) {
		}
		return nil
	}
	wopts := &blob.WriterOptions{
		ContentType: "application/json",
		BeforeWrite: beforeWrite,
	}
	// Create a *blob.Writer.
	w, _ := b.NewWriter(ctx, "key", wopts)
	w.Write([]byte("{"))
	w.Write([]byte(" message: 'hello' "))
	w.Write([]byte("}"))
	w.Close()

	// Use ListOptions.BeforeList to obtain SDK type azblob.ListBlobsSegmentOptions.
	// See https://godoc.org/github.com/Azure/azure-storage-blob-go/azblob#ListBlobsSegmentOptions for more information.
	beforeList := func(as func(i interface{}) bool) error {
		var nativeListOptions *azblob.ListBlobsSegmentOptions
		if as(&nativeListOptions) {
		}
		return nil
	}

	// Iterate through a virtual directory.
	iter := b.List(&blob.ListOptions{Prefix: "blob-for-delimiters", Delimiter: "/", BeforeList: beforeList})
	for {
		if p, err := iter.Next(ctx); err == io.EOF {
			break
		} else if err == nil {
			if p.IsDir {
				// Use ListObject.As to obtain SDK type azblob.BlobPrefix.
				// See https://godoc.org/github.com/Azure/azure-storage-blob-go/azblob#BlobPrefix for more information.
				var nativeDirObj azblob.BlobPrefix
				if p.As(&nativeDirObj) {
				}
			} else {
				// Use ListObject.As to obtain SDK type azblob.BlobItem.
				// See https://godoc.org/github.com/Azure/azure-storage-blob-go/azblob#BlobItem for more information.
				var nativeBlobObj azblob.BlobItem
				if p.As(&nativeBlobObj) {
				}
			}
		}
	}

	// Output:
	// ReadAll failed due to invalid credentials
}
