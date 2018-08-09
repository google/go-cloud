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
	"bytes"
	"context"
	"errors"
	"mime"
	"net/http"

	"github.com/google/go-cloud/blob/driver"
)

// Reader implements io.ReadCloser to read a blob. It must be closed after
// reads are finished.
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

// ContentType returns the MIME type of the blob object.
func (r *Reader) ContentType() string {
	return r.r.Attrs().ContentType
}

// Size returns the content size of the blob object.
func (r *Reader) Size() int64 {
	return r.r.Attrs().Size
}

// Writer implements io.WriteCloser to write a blob. It must be closed after
// all writes are done.
type Writer struct {
	w driver.Writer

	// These fields exist only when w is not created in the first place when
	// NewWriter is called.
	//
	// A ctx is stored in the Writer since we need to pass it into NewTypedWriter
	// when we finish detecting the content type of the object and create the
	// underlying driver.Writer. This step happens inside Write or Close and
	// neither of them take a context.Context as an argument. The ctx must be set
	// to nil after we have passed it.
	ctx    context.Context
	bucket driver.Bucket
	key    string
	opt    *driver.WriterOptions
	buf    *bytes.Buffer
}

// sniffLen is the byte size of Writer.buf used to detect content-type.
const sniffLen = 512

// Write implements the io.Writer interface.
//
// The writes happen asynchronously, which means the returned error can be nil
// even if the actual write fails. Use the error returned from Close to
// check and handle errors.
func (w *Writer) Write(p []byte) (n int, err error) {
	if w.w != nil {
		return w.w.Write(p)
	}

	// If w is not yet created due to no content-type being passed in, try to sniff
	// the MIME type based on at most 512 bytes of the blob content of p.

	// Detect the content-type directly if the first chunk is at least 512 bytes.
	if w.buf.Len() == 0 && len(p) >= sniffLen {
		return w.open(p)
	}

	// Store p in w.buf and detect the content-type when the size of content in
	// w.buf is at least 512 bytes.
	w.buf.Write(p)
	if w.buf.Len() >= sniffLen {
		return w.open(w.buf.Bytes())
	}
	return len(p), nil
}

// Close flushes any buffered data and completes the Write. It is the user's
// responsibility to call it after finishing the write and handle the error if returned.
func (w *Writer) Close() error {
	if w.w != nil {
		return w.w.Close()
	}
	if _, err := w.open(w.buf.Bytes()); err != nil {
		return err
	}
	return w.w.Close()
}

// open tries to detect the MIME type of p and write it to the blob.
func (w *Writer) open(p []byte) (n int, err error) {
	ct := http.DetectContentType(p)
	if w.w, err = w.bucket.NewTypedWriter(w.ctx, w.key, ct, w.opt); err != nil {
		return 0, err
	}
	w.buf = nil
	w.ctx = nil
	w.key = ""
	w.opt = nil
	return w.w.Write(p)
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
// is not found by the given key, which can be checked by calling IsNotExist.
//
// The caller must call Close on the returned Reader when done reading.
func (b *Bucket) NewReader(ctx context.Context, key string) (*Reader, error) {
	return b.NewRangeReader(ctx, key, 0, -1)
}

// NewRangeReader returns a Reader that reads part of an object, reading at
// most length bytes starting at the given offset. If length is 0, it will read
// only the metadata. If length is negative, it will read till the end of the
// object. It returns an error if that object does not exist, which can be
// checked by calling IsNotExist.
//
// The caller must call Close on the returned Reader when done reading.
func (b *Bucket) NewRangeReader(ctx context.Context, key string, offset, length int64) (*Reader, error) {
	if offset < 0 {
		return nil, errors.New("new blob range reader: offset must be non-negative")
	}
	r, err := b.b.NewRangeReader(ctx, key, offset, length)
	return &Reader{r: r}, newBlobError(err)
}

// NewWriter returns a Writer that writes to an object associated with key.
//
// A new object will be created unless an object with this key already exists.
// Otherwise any previous object with the same key will be replaced. The object
// is not guaranteed to be available until Close has been called.
//
// The call may store the ctx for later use in Write and/or Close. The ctx
// must remain open until the returned Writer is closed.
//
// The caller must call Close on the returned Writer when done writing.
func (b *Bucket) NewWriter(ctx context.Context, key string, opt *WriterOptions) (*Writer, error) {
	var dopt *driver.WriterOptions
	var w driver.Writer
	if opt != nil {
		dopt = &driver.WriterOptions{
			BufferSize: opt.BufferSize,
		}
		if opt.ContentType != "" {
			t, p, err := mime.ParseMediaType(opt.ContentType)
			if err != nil {
				return nil, err
			}
			ct := mime.FormatMediaType(t, p)
			w, err = b.b.NewTypedWriter(ctx, key, ct, dopt)
			return &Writer{w: w}, err
		}
	}
	return &Writer{
		ctx:    ctx,
		bucket: b.b,
		key:    key,
		opt:    dopt,
		buf:    bytes.NewBuffer([]byte{}),
	}, nil
}

// Delete deletes the object associated with key. It returns an error if that
// object does not exist, which can be checked by calling IsNotExist.
func (b *Bucket) Delete(ctx context.Context, key string) error {
	return newBlobError(b.b.Delete(ctx, key))
}

// WriterOptions controls Writer behaviors.
type WriterOptions struct {
	// BufferSize changes the default size in bytes of the maximum part Writer can
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

	// ContentType specifies the MIME type of the object being written. If not set,
	// then it will be inferred from the content using the algorithm described at
	// http://mimesniff.spec.whatwg.org/
	ContentType string
}

type blobError struct {
	msg  string
	kind driver.ErrorKind
}

func (e *blobError) Error() string {
	return e.msg
}

func newBlobError(err error) error {
	if err == nil {
		return nil
	}
	berr := &blobError{msg: err.Error()}
	if e, ok := err.(driver.Error); ok {
		berr.kind = e.BlobError()
	}
	return berr
}

// IsNotExist returns whether an error is a driver.Error with NotFound kind.
func IsNotExist(err error) bool {
	if e, ok := err.(*blobError); ok {
		return e.kind == driver.NotFound
	}
	return false
}
