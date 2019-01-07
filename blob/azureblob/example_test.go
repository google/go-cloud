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
	"io"

	"github.com/Azure/azure-storage-blob-go/azblob"
	"gocloud.dev/blob"
	"gocloud.dev/blob/azureblob"
)

func Example() {
	ctx := context.Background()

	accountName, accountKey := "", ""
	bucketName := "my-bucket"
	key := "my-file.json"

	// Initializes an azblob.ServiceURL which represents the Azure Storage Account using Shared Key authorization
	serviceURL, _ := azureblob.ServiceURLFromAccountKey(accountName, accountKey)

	// Credential represents the authorizer for SignedURL
	creds, _ := azblob.NewSharedKeyCredential(accountName, accountKey)
	azureOpts := azureblob.Options{
		Credential: creds,
	}

	// Creates a *blob.Bucket backed by Azure Storage Account
	b, _ := azureblob.OpenBucket(ctx, serviceURL, bucketName, &azureOpts)

	// Get the native blob attributes
	// See https://godoc.org/github.com/Azure/azure-storage-blob-go/azblob#BlobGetPropertiesResponse
	var nativeAttrs azblob.BlobGetPropertiesResponse
	attr, _ := b.Attributes(ctx, key)
	if attr.As(&nativeAttrs) {
		// Use the native SDK type
	}

	// Get the native type from a bucket
	// See https://godoc.org/github.com/Azure/azure-storage-blob-go/azblob
	var nativeBucket *azblob.ContainerURL
	if b.As(&nativeBucket) {
		// Use the native SDK type
	}

	// Write data to a new blob
	beforeWrite := func(as func(i interface{}) bool) error {
		// See https://godoc.org/github.com/Azure/azure-storage-blob-go/azblob#UploadStreamToBlockBlobOptions
		var nativeWriteOptions *azblob.UploadStreamToBlockBlobOptions
		if as(&nativeWriteOptions) {
			// Use the native SDK type
		}
		return nil
	}
	opts := &blob.WriterOptions{
		ContentType: "application/json",
		BeforeWrite: beforeWrite,
	}
	w, _ := b.NewWriter(ctx, key, opts)
	w.Write([]byte("{"))
	w.Write([]byte(" message: 'hello' "))
	w.Write([]byte("}"))
	w.Close()

	// Read data from a blob
	r, _ := b.NewRangeReader(ctx, key, 0, -1, nil)
	got := make([]byte, 100)
	r.Read(got)

	// Get the native type from reader
	// See https://godoc.org/github.com/Azure/azure-storage-blob-go/azblob#DownloadResponse
	var nativeReader azblob.DownloadResponse
	if r.As(&nativeReader) {
		// Use the native SDK type
	}

	beforeList := func(as func(i interface{}) bool) error {
		// See https://godoc.org/github.com/Azure/azure-storage-blob-go/azblob#ListBlobsSegmentOptions
		var nativeListOptions *azblob.ListBlobsSegmentOptions
		if as(&nativeListOptions) {
			// Use the native SDK type
		}
		return nil
	}

	// Iterate through a virtual directory
	iter := b.List(&blob.ListOptions{Prefix: "blob-for-delimiters", Delimiter: "/", BeforeList: beforeList})
	for {
		if p, err := iter.Next(ctx); err == io.EOF {
			break
		} else if err == nil {
			if p.IsDir {
				// See https://godoc.org/github.com/Azure/azure-storage-blob-go/azblob#BlobPrefix
				var nativeDirObj azblob.BlobPrefix
				if p.As(&nativeDirObj) {
					// Use the native SDK type
				}
			} else {
				// See https://godoc.org/github.com/Azure/azure-storage-blob-go/azblob#BlobItem
				var nativeBlobObj azblob.BlobItem
				if p.As(&nativeBlobObj) {
					// Use the native SDK type
				}
			}
		}
	}
}
func Example_open() {

	// Open creates a *Bucket from a URL.
	// Example URL: azblob://my-bucket?cred_path=replace-with-path-to-credentials-file
	// The schema "azblob" represents the provider for Azure Blob (Azure Storage Account).
	// The host "my-bucket" represents the blob bucket (Azure Storage Account's Container).
	// The queryString "cred_path: represents the credentials file in JSON format. See the package documentation for credentials file schema.

	_, _ = blob.Open(context.Background(), "azblob://my-bucket?cred_path=replace-with-path-to-credentials-file")

	// Output:
}
