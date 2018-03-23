// Copyright 2018 Google LLC
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

// Package IO adds multi-platform IO.
//
// IO is performed on simplified file-like objects called Blobs. A Blob is a
// handle to underlying storage and is created from a storage URI:
//
//     blob, err := NewBlob(ctx, "s3://mybucket/myObjectName")
//
// If the binary was not compiled with support for the service indicated in the
// URI, NewBlob will return an error indicating the missing service.
//
// A new Blob can be created using NewWriter:
//
//     w, err := blob.NewWriter(ctx)
//     w.Write([]byte("Hello world"))
//     w.Close()                         // flush data to storage
//
// If a Blob already exists in underlying storage, calling NewWriter or Write
// may delete existing data (depending on the implementation).
//
// Blobs that already contain data can be read via NewReader:
//
//     r, err := blob.NewReader(ctx)
//     data, err := ioutil.ReadAll(r)
//     fmt.Printf("%s", data)            // prints "Hello World"
//
// Large Blobs can be accessed incrementally using NewRangeReader:
//
//     rr, err := blob.NewRangeReader(ctx, 3, 100)
//     data, err := ioutil.ReadAll(rr)
//     fmt.Printf("%s", data)            // prints "lo World"
//
// Simultaneously reading from and writing to a Blob results in undefined
// behavior.
package io

import (
	"context"
	"fmt"

	cloud "github.com/google/go-cloud"
)

var blobProviders = make(map[string]BlobProvider)

// NewBlob returns a handle to a Blob represented by a path. Some example paths:
//
//   local://relative/path/to/file
//   local:///absolute/path/to/file
//   gcs://bucketName/myObjectName
//   s3://bucketName/myObjectName
//
// NewBlob returns an error if a service name cannot be parsed from path, or if
// the service name is not known.
//
// NewBlob does not validate if the underlying blob exists or if the program
// has access to it. These checks are performed when a BlobReader or BlobWriter
// are obtained from the returned Blob.
func NewBlob(ctx context.Context, path string) (Blob, error) {
	service, _, err := cloud.Service(path)
	if err != nil {
		return nil, err
	}
	if handler, ok := blobProviders[service]; ok {
		return handler(ctx, path)
	}
	return nil, fmt.Errorf("no blob provider for '%s'", path)
}
