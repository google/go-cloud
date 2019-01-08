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
	// Set a fake key which must be base64 encoded. The Azure Storage Access Keys are already base64 encoded.
	accountName, accountKey := "gocloudblobtests", base64.StdEncoding.EncodeToString([]byte("FAKECREDS"))
	bucketName := "my-bucket"

	// Initializes an azblob.ServiceURL which represents the Azure Storage Account using Shared Key authorization.
	serviceURL, err := azureblob.ServiceURLFromAccountKey(accountName, accountKey)
	if err != nil {
		log.Fatal(err)
		return
	}

	// Credential represents the authorizer for SignedURL.
	creds, err := azblob.NewSharedKeyCredential(accountName, accountKey)
	if err != nil {
		log.Fatal(err)
		return
	}

	// Override the default retry policy so calls can return promptly for this test. Please review the timeout guidelines and set accordingly.
	// See https://docs.microsoft.com/en-us/rest/api/storageservices/setting-timeouts-for-blob-service-operations for more information.
	opts := azblob.PipelineOptions{
		Retry: azblob.RetryOptions{
			Policy:        azblob.RetryPolicyFixed,
			TryTimeout:    10 * time.Second,
			MaxTries:      1,
			RetryDelay:    0 * time.Second,
			MaxRetryDelay: 0 * time.Second,
		},
	}
	p := azblob.NewPipeline(creds, opts)
	*serviceURL = serviceURL.WithPipeline(p)

	if err != nil {
		// This is expected since accountName and accountKey is empty.
		log.Fatal(err)
		return
	}

	// Set credential for the SignedURL operation.
	azureOpts := azureblob.Options{
		Credential: creds,
	}

	// Create a *blob.Bucket.
	b, err := azureblob.OpenBucket(ctx, serviceURL, bucketName, &azureOpts)
	if err != nil {
		log.Fatal(err)
		return
	}

	_, err = b.ReadAll(ctx, "my-key")
	if err != nil {
		// This is expected due to the fake credentials we used above.
		fmt.Println("ReadAll failed due to invalid credentials")
	}

	// Output:
	// ReadAll failed due to invalid credentials
}

func Example_open() {

	// Open creates a *Bucket from a URL.
	// Example URL: azblob://my-bucket?cred_path=replace-with-path-to-credentials-file
	// The schema "azblob" represents the provider for Azure Blob (Azure Storage Account).
	// The host "my-bucket" represents the blob bucket (Azure Storage Account's Container).
	// The queryString "cred_path: represents the credentials file in JSON format. See the package documentation for credentials file schema.

	_, err := blob.Open(context.Background(), "azblob://my-bucket?cred_path=replace-with-path-to-credentials-file")
	if err != nil {
		// Windows will return error: replace-with-path-to-credentials-file: The system cannot find the file specified.
		// macOS and Linux will return error: open replace-with-path-to-credentials-file: no such file or directory
		fmt.Println(err)
	}
	// Output:
	// open replace-with-path-to-credentials-file: no such file or directory
}

func Example_as() {

	ctx := context.Background()
	// Set a fake key which must be base64 encoded. The Azure Storage Access Keys are already base64 encoded.
	accountName, accountKey := "gocloudblobtests", base64.StdEncoding.EncodeToString([]byte("FAKECREDS"))
	bucketName, key := "my-bucket", "my-key"

	// Initializes an azblob.ServiceURL which represents the Azure Storage Account using Shared Key authorization.
	serviceURL, err := azureblob.ServiceURLFromAccountKey(accountName, accountKey)
	if err != nil {
		log.Fatal(err)
		return
	}

	// Credential represents the authorizer for SignedURL.
	creds, err := azblob.NewSharedKeyCredential(accountName, accountKey)
	if err != nil {
		log.Fatal(err)
		return
	}

	// Override the default retry policy so calls can return promptly for this test. Please review the timeout guidelines and set accordingly.
	// See https://docs.microsoft.com/en-us/rest/api/storageservices/setting-timeouts-for-blob-service-operations for more information.
	pipelineOpts := azblob.PipelineOptions{
		Retry: azblob.RetryOptions{
			Policy:        azblob.RetryPolicyFixed,
			TryTimeout:    10 * time.Second,
			MaxTries:      1,
			RetryDelay:    0 * time.Second,
			MaxRetryDelay: 0 * time.Second,
		},
	}
	p := azblob.NewPipeline(creds, pipelineOpts)
	*serviceURL = serviceURL.WithPipeline(p)

	// Set credential for the SignedURL operation.
	azureOpts := azureblob.Options{
		Credential: creds,
	}

	// Create a *blob.Bucket.
	b, err := azureblob.OpenBucket(ctx, serviceURL, bucketName, &azureOpts)
	if err != nil {
		log.Fatal(err)
		return
	}

	// Create a *blob.Reader.
	r, err := b.NewReader(ctx, key, &blob.ReaderOptions{})
	if err != nil {
		// This is expected due to the fake credentials we used above.
		fmt.Println("ReadAll failed due to invalid credentials")
		return
	}

	// Use Reader.As to obtain SDK type azblob.DownloadResponse.
	// See https://godoc.org/github.com/Azure/azure-storage-blob-go/azblob#DownloadResponse for more information.
	var nativeReader azblob.DownloadResponse
	if r.As(&nativeReader) {

	}

	// Use Attribute.As to obtain SDK type azblob.BlobGetPropertiesResponse.
	// See https://godoc.org/github.com/Azure/azure-storage-blob-go/azblob#BlobGetPropertiesResponse for more information.
	var nativeAttrs azblob.BlobGetPropertiesResponse
	attr, _ := b.Attributes(ctx, key)
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
	opts := &blob.WriterOptions{
		ContentType: "application/json",
		BeforeWrite: beforeWrite,
	}
	// Create a *blob.Writer.
	w, _ := b.NewWriter(ctx, key, opts)
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
