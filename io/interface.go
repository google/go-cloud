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

package io

import (
	"context"
	baseio "io"
)

// BlobReader reads from a Blob and provides the total size of the Blob.
type BlobReader interface {
	baseio.Reader
	baseio.Closer
	Size() int64
}

// BlobWriter writes to a Blob.
type BlobWriter interface {
	baseio.Writer
	baseio.Closer
}

// A Blob is a handle to a linear container of bytes. It is a simpler and lazier
// interface than that of a file, suitable for use with vary large objects
// and/or objects accessed over a network.
type Blob interface {
	// NewReader creates a new BlobReader to read the contents of the Blob, or
	// returns an error.
	NewReader(ctx context.Context) (BlobReader, error)

	// NewRangeReader creates a new BlobReader to read part of a Blob, reading at
	// most length bytes starting at offset. If length is negative the Blob is
	// read until the end.
	NewRangeReader(ctx context.Context, offset, length int64) (BlobReader, error)

	// NewWriter creates a new BlobWriter to write to a Blob. If a Blob does not
	// exist, NewWriter will attempt to create it. If the Blob already exists, its
	// contents may be truncated (deleted) when NewWriter is called.
	//
	// The caller must call Close after writing is complete to ensure all data is
	// committed to storage.
	NewWriter(ctx context.Context) (BlobWriter, error)

	// Remove removes the Blob if it exists. Remove is a no-op if the Blob does
	// not exist.
	Remove(ctx context.Context) error
}

// BlobProvider is passed to RegisterBlobProvider to add a new blob storage
// implementations to the library.
type BlobProvider func(context.Context, string) (Blob, error)

// RegisterBlobProvider registers a BlobProvider for the given named service.
func RegisterBlobProvider(service string, f BlobProvider) {
	blobProviders[service] = f
}
