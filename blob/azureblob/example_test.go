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

	// Create a *blob.Bucket for Azure using AccountName and AccountKey
	b, _ := azureblob.OpenBucket(ctx, &azureblob.Settings{
		DefaultDelimiter: "/",
		AccountName:      accountName,
		AccountKey:       accountKey,
	}, bucketName)

	// Obtain the Azure SDK type for attributes
	var nativeAttrs azblob.BlobGetPropertiesResponse
	attr, _ := b.Attributes(ctx, key)
	if attr.As(&nativeAttrs) {
		// Use the native SDK object
	}

	// Obtain the Azure SDK type for container which represents a go-cloud bucket
	var nativeBucket azblob.ContainerURL
	if b.As(&nativeBucket) {
		// Use the native SDK object
	}

	// Write data to a new blob
	opts := &blob.WriterOptions{
		ContentType: "application/json",
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

	// Obtain the Azure SDK type for blockblob
	var nativeReader azblob.BlockBlobURL
	if r.As(&nativeReader) {
		// Use the native SDK object
	}

	// Iterate through a virtual directory
	iter := b.List(&blob.ListOptions{Prefix: "blob-for-delimiters", Delimiter: "/"})
	for {
		if p, err := iter.Next(ctx); err == io.EOF {
			break
		} else if err == nil {
			if p.IsDir {
				var nativeDirObj azblob.BlobPrefix
				if p.As(&nativeDirObj) {
					// Use the native SDK object
				}
			} else {
				var nativeBlobObj azblob.BlobItem
				if p.As(&nativeBlobObj) {
					// Use the native SDK object
				}
			}
		}
	}
}
