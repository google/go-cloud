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

// Package blob provides an easy way to interact with Blob objects within
// a bucket. It utilizes standard io packages to handle reads and writes.
package blob

import (
	"context"
	"errors"

	"github.com/google/go-cloud/blob/driver"
)

// Reader implements io.ReadCloser to read from blob. It must be closed after
// reads finished. It also provides the size of the blob.
type Reader struct {
	r driver.Reader
}

// Read implements io.ReadCloser to read from this reader.
func (r *Reader) Read(p []byte) (int, error) {
	return r.r.Read(p)
}

// Close implements io.ReadCloser to close this reader.
func (r *Reader) Close() error {
	return r.r.Close()
}

// Size returns the content size of the blob object.
func (r *Reader) Size() int64 {
	return r.r.Size()
}

// Writer implements io.WriteCloser to write to blob. It must be closed after
// all writes are done.
type Writer struct {
	w driver.Writer
}

// Write implements the io.Writer interface.
//
// The writes happen asynchronously, which means the returned error can be nil
// even if the actual write fails. Use the error returned from Close method to
// check and handle error.
func (w *Writer) Write(p []byte) (n int, err error) {
	return w.w.Write(p)
}

// Close flushes any buffered data and completes the Write. It is user's responsibility
// to call it after finishing the write and handle the error if returned.
func (w *Writer) Close() error {
	return w.w.Close()
}

// Bucket manages the underlying blob service and provides read, write and delete
// operations on given object within it.
type Bucket struct {
	b driver.Bucket
}

// NewBucket creates a new Bucket for a group of objects for a blob service.
func NewBucket(b driver.Bucket) *Bucket {
	return &Bucket{b: b}
}

// NewReader returns a Reader to read from an object, or an error when the object
// is not found by the given key, use IsErrNotExist to check for it.
//
// The caller must call Close on the returned Reader when done reading.
func (b *Bucket) NewReader(ctx context.Context, key string) (*Reader, error) {
	return b.NewRangeReader(ctx, key, 0, -1)
}

// NewRangeReader returns a Reader that reads part of an object, reading at
// most length bytes starting at the given offset. If length is 0, it will read
// only the metadata. If length is negative, it will read till the end of the
// object. It returns an error if that object does not exist, which can be
// checked by calling IsErrNotExist.
//
// The caller must call Close on the returned Reader when done reading.
func (b *Bucket) NewRangeReader(ctx context.Context, key string, offset, length int64) (*Reader, error) {
	if offset < 0 {
		return nil, errors.New("new blob range reader: offset must be non-negative")
	}
	r, err := b.b.NewRangeReader(ctx, key, offset, length)
	return &Reader{r}, err
}

// NewWriter returns Writer that writes to an object associated with key.
//
// A new object will be created unless an object with this key already exists.
// Otherwise any previous object with the same name will be replaced.
// The object will not be available (and any previous object will remain)
// until Close has been called.
//
// The caller must call Close on the returned Writer when done writing.
func (b *Bucket) NewWriter(ctx context.Context, key string, opt *WriterOptions) (*Writer, error) {
	var dopt *driver.WriterOptions
	if opt != nil {
		dopt = &driver.WriterOptions{
			BufferSize: opt.BufferSize,
		}
	}
	w, err := b.b.NewWriter(ctx, key, dopt)
	return &Writer{w}, err
}

// Delete deletes the object associated with key. It returns an error if that
// object does not exist, which can be checked by calling IsErrNotExist.
func (b *Bucket) Delete(ctx context.Context, key string) error {
	return b.b.Delete(ctx, key)
}

// WriterOptions controls behaviors of Writer.
type WriterOptions struct {
	// BufferSize changes the default size in byte of the maximum part Writer can
	// write in a single request. Larger objects will be split into multiple requests.
	//
	// The support specification of this operation varies depending on the underlying
	// blob service. If zero value is given, it is set to a reasonable default value.
	// If negative value is given, it will be either disabled (if supported by the
	// service), which means Writer will write as a whole, or reset to default value.
	// It could be a no-op when not supported at all.
	//
	// If the Writer is used to write small objects concurrently, set the buffer size
	// to a smaller size to avoid high memory usage.
	BufferSize int
}

// IsErrObjectNotExist returns wheter an error is a driver.Error with NotFound kind.
func IsErrObjectNotExist(err error) bool {
	if e, ok := err.(driver.Error); ok {
		return e.BlobError() == driver.NotFound
	}
	return false
}
