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

// Package blob provides an easy way to interact with Blob objects within
// a bucket. It utilizes standard io packages to handle reads and writes.
package blob

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"mime"
	"net/http"
	"strings"
	"time"

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
	return r.r.Attributes().ContentType
}

// ModTime is the time the blob object was last modified.
func (r *Reader) ModTime() time.Time {
	return r.r.Attributes().ModTime
}

// Size returns the content size of the blob object.
func (r *Reader) Size() int64 {
	return r.r.Attributes().Size
}

// As converts i to provider-specific types.
// See Bucket.As for more details.
func (r *Reader) As(i interface{}) bool {
	return r.r.As(i)
}

// Attributes contains attributes about a blob.
type Attributes struct {
	// ContentType is the MIME type of the blob object. It will not be empty.
	ContentType string
	// Metadata holds key/value pairs associated with the blob.
	// Keys are guaranteed to be in lowercase, even if the backend provider
	// has case-sensitive keys (although note that Metadata written via
	// this package will always be lowercased). If there are duplicate
	// case-insensitive keys (e.g., "foo" and "FOO"), only one value
	// will be kept, and it is undefined which one.
	Metadata map[string]string
	// ModTime is the time the blob object was last modified.
	ModTime time.Time
	// Size is the size of the object in bytes.
	Size int64

	asFunc func(interface{}) bool
}

// As converts i to provider-specific types.
// See Bucket.As for more details.
func (a *Attributes) As(i interface{}) bool {
	if a.asFunc == nil {
		return false
	}
	return a.asFunc(i)
}

// Writer implements io.WriteCloser to write to blob. It must be closed after
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
// Close will return an error if the context provided to create w is canceled or times out.
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

// MaxPageSize is the maximum value for ListOptions.PageSize.
const MaxPageSize = 1000

// ListOptions sets options for listing objects.
// TODO(Issue #541): Add Delimiter.
type ListOptions struct {
	// Prefix indicates that only objects with a key starting with this prefix
	// should be returned.
	Prefix string

	// PageSize sets the maximum number of objects that will be retrieved
	// at a time from the provider.
	// Must be >= 0 and <= MaxPageSize; 0 defaults to MaxPageSize.
	PageSize int

	// BeforeList is a callback that will be called before each call to the
	// the underlying provider's list functionality.
	// asFunc converts its argument to provider-specific types.
	// See Bucket.As for more details.
	BeforeList func(asFunc func(interface{}) bool) error
}

// ListIterator is used to iterate over List results.
type ListIterator struct {
	b       *Bucket
	opt     *driver.ListOptions
	page    *driver.ListPage
	nextIdx int
}

// Next returns the next object. It returns nil if there are
// no more objects.
func (i *ListIterator) Next(ctx context.Context) (*ListObject, error) {
	if i.page != nil {
		// We've already got a page of results.
		if i.nextIdx < len(i.page.Objects) {
			// Next object is in the page; return it.
			dobj := i.page.Objects[i.nextIdx]
			i.nextIdx++
			return &ListObject{
				Key:     dobj.Key,
				ModTime: dobj.ModTime,
				Size:    dobj.Size,
				asFunc:  dobj.AsFunc,
			}, nil
		}
		if len(i.page.NextPageToken) == 0 {
			// Done with current page, and there are no more; return nil.
			return nil, nil
		}
		// We need to load the next page.
		i.opt.PageToken = i.page.NextPageToken
	}
	// Loading a new page.
	p, err := i.b.b.ListPaged(ctx, i.opt)
	if err != nil {
		return nil, err
	}
	i.page = p
	i.nextIdx = 0
	return i.Next(ctx)
}

// ListObject represents a single blob object returned from List.
type ListObject struct {
	// Key is the key for this blob.
	Key string
	// ModTime is the time the blob object was last modified.
	ModTime time.Time
	// Size is the size of the object in bytes.
	Size int64

	asFunc func(interface{}) bool
}

// As converts i to provider-specific types.
// See Bucket.As for more details.
func (o *ListObject) As(i interface{}) bool {
	if o.asFunc == nil {
		return false
	}
	return o.asFunc(i)
}

// Bucket manages the underlying blob service and provides read, write and delete
// operations on objects within it.
type Bucket struct {
	b driver.Bucket
}

// NewBucket creates a new Bucket for a group of objects for a blob service.
func NewBucket(b driver.Bucket) *Bucket {
	return &Bucket{b: b}
}

// As converts i to provider-specific types. See provider documentation for
// which type(s) are supported.
//
// Usage:
// 1. Declare a variable of the provider-specific type you want to access.
// 2. Pass a pointer to it to As.
// 3. If the type is supported, As will return true and copy the
//    provider-specific type into your variable. Otherwise, it will return false.
//
// Provider-specific types that are intended to be mutable will be exposed
// as a pointer to the underlying type.
//
// See
// https://github.com/google/go-cloud/blob/master/internal/docs/design.md#as
// for more background.
func (b *Bucket) As(i interface{}) bool {
	return b.b.As(i)
}

// ReadAll is a shortcut for creating a Reader via NewReader and reading the entire blob.
func (b *Bucket) ReadAll(ctx context.Context, key string) ([]byte, error) {
	r, err := b.NewReader(ctx, key)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return ioutil.ReadAll(r)
}

// List returns an object that can be used to iterate over objects in a
// bucket, in lexicographical order of UTF-8 encoded keys. The underlying
// implementation fetches results in pages.
// Use ListOptions to control the page size and filtering.
//
// List is not guaranteed to include all recently-written objects;
// some providers are only eventually consistent.
func (b *Bucket) List(ctx context.Context, opt *ListOptions) (*ListIterator, error) {
	if opt == nil {
		opt = &ListOptions{}
	}
	if opt.PageSize < 0 {
		return nil, errors.New("ListOptions.PageSize must be >= 0")
	}
	if opt.PageSize > MaxPageSize {
		return nil, fmt.Errorf("ListOptions.PageSize must be < %d", MaxPageSize)
	}
	if opt.PageSize == 0 {
		opt.PageSize = MaxPageSize
	}
	dopt := &driver.ListOptions{
		Prefix:     opt.Prefix,
		PageSize:   opt.PageSize,
		BeforeList: opt.BeforeList,
	}
	return &ListIterator{b: b, opt: dopt}, nil
}

// Attributes reads attributes for the given key.
func (b *Bucket) Attributes(ctx context.Context, key string) (Attributes, error) {
	a, err := b.b.Attributes(ctx, key)
	if err != nil {
		return Attributes{}, err
	}
	var md map[string]string
	if len(a.Metadata) > 0 {
		// Providers are inconsistent, but at least some treat keys
		// as case-insensitive. To make the behavior consistent, we
		// force-lowercase them when writing and reading.
		md = make(map[string]string, len(a.Metadata))
		for k, v := range a.Metadata {
			md[strings.ToLower(k)] = v
		}
	}
	return Attributes{
		ContentType: a.ContentType,
		Metadata:    md,
		ModTime:     a.ModTime,
		Size:        a.Size,
		asFunc:      a.AsFunc,
	}, nil
}

// NewReader returns a Reader to read from an object, or an error when the object
// is not found by the given key, which can be checked by calling IsNotExist.
//
// The caller must call Close on the returned Reader when done reading.
func (b *Bucket) NewReader(ctx context.Context, key string) (*Reader, error) {
	return b.NewRangeReader(ctx, key, 0, -1)
}

// NewRangeReader returns a Reader that reads part of an object, reading at
// most length bytes starting at the given offset. If length is negative, it
// will read till the end of the object. offset must be >= 0, and length cannot
// be 0.
//
// NewRangeReader returns an error if the object does not exist, which can be checked
// by calling IsNotExist. Bucket.Attributes is a lighter-weight way to check for
// existence.
//
// The caller must call Close on the returned Reader when done reading.
func (b *Bucket) NewRangeReader(ctx context.Context, key string, offset, length int64) (*Reader, error) {
	if offset < 0 {
		return nil, errors.New("new blob range reader: offset must be non-negative")
	}
	if length == 0 {
		return nil, errors.New("new blob range reader: length cannot be 0")
	}
	r, err := b.b.NewRangeReader(ctx, key, offset, length)
	return &Reader{r: r}, err
}

// WriteAll is a shortcut for creating a Writer via NewWriter and writing p.
func (b *Bucket) WriteAll(ctx context.Context, key string, p []byte, opt *WriterOptions) error {
	w, err := b.NewWriter(ctx, key, opt)
	if err != nil {
		return err
	}
	if _, err := w.Write(p); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

// NewWriter returns a Writer that writes to an object associated with key.
//
// A new object will be created unless an object with this key already exists.
// Otherwise any previous object with the same key will be replaced. The object
// is not guaranteed to be available until Close has been called.
//
// The returned Writer will store ctx for later use in Write and/or Close.
// To abort a write, cancel the provided context; otherwise it must remain open until
// Close is called.
//
// The caller must call Close on the returned Writer, even if the write is
// aborted.
func (b *Bucket) NewWriter(ctx context.Context, key string, opt *WriterOptions) (*Writer, error) {
	var dopt *driver.WriterOptions
	var w driver.Writer
	if opt != nil {
		dopt = &driver.WriterOptions{
			BufferSize:  opt.BufferSize,
			BeforeWrite: opt.BeforeWrite,
		}
		if len(opt.Metadata) > 0 {
			// Providers are inconsistent, but at least some treat keys
			// as case-insensitive. To make the behavior consistent, we
			// force-lowercase them when writing and reading.
			md := make(map[string]string, len(opt.Metadata))
			for k, v := range opt.Metadata {
				if k == "" {
					return nil, errors.New("WriterOptions.Metadata keys may not be empty strings")
				}
				lowerK := strings.ToLower(k)
				if _, found := md[lowerK]; found {
					return nil, fmt.Errorf("duplicate case-insensitive metadata key %q", lowerK)
				}
				md[lowerK] = v
			}
			dopt.Metadata = md
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
	return b.b.Delete(ctx, key)
}

// SignedURL returns a URL that can be used to GET the blob for the duration
// specified in opts.Expiry.
// If IsNotImplemented returns true for the returned error, the provider does
// not support SignedURL.
func (b *Bucket) SignedURL(ctx context.Context, key string, opts *SignedURLOptions) (string, error) {
	if opts == nil {
		opts = &SignedURLOptions{}
	}
	if opts.Expiry < 0 {
		return "", errors.New("SignedURLOptions.Expiry must be >= 0")
	}
	if opts.Expiry == 0 {
		opts.Expiry = DefaultSignedURLExpiry
	}
	dopts := driver.SignedURLOptions{
		Expiry: opts.Expiry,
	}
	return b.b.SignedURL(ctx, key, &dopts)
}

// DefaultSignedURLExpiry is the default duration for SignedURLOptions.Expiry.
const DefaultSignedURLExpiry = 1 * time.Hour

// SignedURLOptions sets options for SignedURL.
type SignedURLOptions struct {
	// Expiry sets how long the returned URL is valid for.
	// Defaults to DefaultSignedURLExpiry.
	Expiry time.Duration
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

	// Metadata are key/value strings to be associated with the blob, or nil.
	// Keys may not be empty, and are lowercased before being written.
	// Duplicate case-insensitive keys (e.g., "foo" and "FOO") are an error.
	Metadata map[string]string

	// BeforeWrite is a callback that will be called exactly once, before
	// any data is written (unless NewWriter returns an error, in which case
	// it will not be called at all). Note that this is not necessarily during
	// or after the first Write call, as providers may buffer.
	// asFunc converts its argument to provider-specific types.
	// See Bucket.As for more details.
	BeforeWrite func(asFunc func(interface{}) bool) error
}

// IsNotExist returns true iff err indicates that the referenced blob does not exist.
func IsNotExist(err error) bool {
	if e, ok := err.(driver.Error); ok {
		return e.Kind() == driver.NotFound
	}
	return false
}

// IsNotImplemented returns true iff err indicates that the provider does not
// support the given operation.
func IsNotImplemented(err error) bool {
	if e, ok := err.(driver.Error); ok {
		return e.Kind() == driver.NotImplemented
	}
	return false
}
