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

// Package driver defines a set of interfaces that the blob package uses to interact
// with the underlying blob services.
package driver

import (
	"context"
	"io"
)

// Reader reads an object from the blob.
type Reader interface {
	io.ReadCloser

	// Attrs returns the ObjectAttrs associated with the object.
	Attrs() *ObjectAttrs
}

// Writer writes an object to the blob.
type Writer interface {
	io.WriteCloser
}

// WriterOptions controls behaviors of Writer.
type WriterOptions struct {
	// BufferSize changes the default size in byte of the maximum part Writer can
	// write in a single request. Larger objects will be split into multiple
	// requests.
	//
	// The support specification of this operation varies depending on the
	// underlying blob service. If zero value is given, it is set to a reasonable
	// default value. If negative value is given, it will be either disabled (if
	// supported by the service), which means Writer will write as a whole, or
	// reset to default value. It could be a no-op when not supported at all.
	//
	// If the Writer is used to write small objects concurrently, set the buffer
	// size to a smaller size to avoid high memory usage.
	BufferSize int

	// ContentType sets the MIME type of an object before writing to blob
	// service. If not set, the content-type will be sniffed using
	// net/http.DefaultContentType.
	ContentType string
}

// ObjectAttrs contains metadata of an object.
type ObjectAttrs struct {
	Size        int64
	ContentType string
}

// Bucket provides read, write and delete operations on objects within it on the
// blob service.
type Bucket interface {
	// NewRangeReader returns a Reader that reads part of an object, reading at most
	// length bytes starting at the given offset. If length is 0, it will read only
	// the metadata. If length is negative, it will read till the end of the object.
	NewRangeReader(ctx context.Context, key string, offset, length int64) (Reader, error)

	// NewWriter returns Writer that writes to an object associated with key.
	//
	// A new object will be created unless an object with this key already exists.
	// Otherwise any previous object with the same name will be replaced.
	// The object may not be available (and any previous object will remain)
	// until Close has been called.
	//
	// The caller must call Close on the returned Writer when done writing.
	NewWriter(ctx context.Context, key string, opt *WriterOptions) (Writer, error)

	// Delete deletes the object associated with key. It is a no-op if that object
	// does not exist.
	Delete(ctx context.Context, key string) error
}
