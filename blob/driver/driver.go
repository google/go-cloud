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

// Package driver defines a set of interfaces that the blob package uses to interact
// with the underlying blob services.
package driver

import (
	"context"
	"io"
	"time"
)

// ErrorKind is a code to indicate the kind of failure.
type ErrorKind int

const (
	// GenericError is the default ErrorKind.
	GenericError ErrorKind = iota
	// NotFound indicates that the referenced key does not exist.
	NotFound
	// NotImplemented indicates that the provider does not support this operation.
	NotImplemented
)

// Error is an interface that may be implemented by an error returned by
// a driver to indicate the kind of failure.  If an error does not have the
// Kind method, then it is assumed to be GenericError.
type Error interface {
	error
	Kind() ErrorKind
}

// Reader reads an object from the blob.
type Reader interface {
	io.ReadCloser

	// Attributes returns a subset of attributes about the blob.
	Attributes() ReaderAttributes

	// As allows providers to expose provider-specific types;
	// see Bucket.As for more details.
	As(interface{}) bool
}

// Writer writes an object to the blob.
type Writer interface {
	io.WriteCloser
}

// WriterOptions controls behaviors of Writer.
type WriterOptions struct {
	// BufferSize changes the default size in byte of the maximum part Writer can
	// write in a single request, if supported. Larger objects will be split into
	// multiple requests.
	BufferSize int
	// Metadata holds key/value strings to be associated with the blob.
	// Keys are guaranteed to be non-empty and lowercased.
	Metadata map[string]string
	// BeforeWrite is a callback that must be called exactly once before
	// any data is written, unless NewTypedWriter returns an error, in
	// which case it should not be called.
	// asFunc allows providers to expose provider-specific types;
	// see Bucket.As for more details.
	BeforeWrite func(asFunc func(interface{}) bool) error
}

// ReaderAttributes contains a subset of attributes about a blob that are
// accessible from Reader.
type ReaderAttributes struct {
	// ContentType is the MIME type of the blob object. It must not be empty.
	ContentType string
	// ModTime is the time the blob object was last modified.
	ModTime time.Time
	// Size is the size of the object in bytes.
	Size int64
}

// Attributes contains attributes about a blob.
type Attributes struct {
	// ContentType is the MIME type of the blob object. It must not be empty.
	ContentType string
	// Metadata holds key/value pairs associated with the blob.
	// Keys will be lowercased by the concrete type before being returned
	// to the user. If there are duplicate case-insensitive keys (e.g.,
	// "foo" and "FOO"), only one value will be kept, and it is undefined
	// which one.
	Metadata map[string]string
	// ModTime is the time the blob object was last modified.
	ModTime time.Time
	// Size is the size of the object in bytes.
	Size int64
	// AsFunc allows providers to expose provider-specific types;
	// see Bucket.As for more details.
	// If not set, no provider-specific types are supported.
	AsFunc func(interface{}) bool
}

// ListOptions sets options for listing objects in the bucket.
// TODO(Issue #541): Add Delimiter.
type ListOptions struct {
	// Prefix indicates that only results with the given prefix should be
	// returned.
	Prefix string
	// PageSize sets the maximum number of objects that will be returned in
	// a single call. It is guaranteed to be > 0 and <= blob.MaxPageSize.
	PageSize int
	// PageToken may be filled in with the NextPageToken from a previous
	// ListPaged call.
	PageToken []byte
	// BeforeList is a callback that must be called exactly once during ListPaged,
	// before the underlying provider's list is executed.
	// asFunc allows providers to expose provider-specific types;
	// see Bucket.As for more details.
	BeforeList func(asFunc func(interface{}) bool) error
}

// ListObject represents a specific blob object returned from ListPaged.
type ListObject struct {
	// Key is the key for this blob.
	Key string
	// ModTime is the time the blob object was last modified.
	ModTime time.Time
	// Size is the size of the object in bytes.
	Size int64
	// AsFunc allows providers to expose provider-specific types;
	// see Bucket.As for more details.
	// If not set, no provider-specific types are supported.
	AsFunc func(interface{}) bool
}

// ListPage represents a page of results return from ListPaged.
type ListPage struct {
	// Objects is the slice of objects found. It should have at most
	// ListOptions.PageSize entries.
	Objects []*ListObject
	// NextPageToken should be left empty unless there are more objects
	// to return. The value may be returned as ListOptions.PageToken on a
	// subsequent ListPaged call, to fetch the next page of results.
	// It can be an arbitrary []byte; it need not be a valid key.
	NextPageToken []byte
}

// Bucket provides read, write and delete operations on objects within it on the
// blob service.
type Bucket interface {
	// As allows providers to expose provider-specific types.
	//
	// i will be a pointer to the type the user wants filled in.
	// As should either fill it in and return true, or return false.
	//
	// Mutable objects should be exposed as a pointer to the object;
	// i will therefore be a **.
	//
	// A provider should document the type(s) it support in package
	// comments, and add conformance tests verifying them.
	//
	// A sample implementation might look like this, for supporting foo.MyType:
	//   mt, ok := i.(*foo.MyType)
	//   if !ok {
	//     return false
	//   }
	//   *i = foo.MyType{}  // or, more likely, the existing value
	//   return true
	//
	// See
	// https://github.com/google/go-cloud/blob/master/internal/docs/design.md#as
	// for more background.
	As(i interface{}) bool

	// Attributes returns attributes for the blob. If the specified object does
	// not exist, Attributes must return an error whose Kind method returns
	// NotFound.
	Attributes(ctx context.Context, key string) (Attributes, error)

	// ListPaged lists objects in the bucket, in lexicographical order by
	// UTF-8-encoded key, returning pages of objects at a time.
	// Providers are only required to be eventually consistent with respect
	// to recently written or deleted objects. That is to say, there is no
	// guarantee that an object that's been written will immediately be returned
	// from ListPaged.
	// opt is guaranteed to be non-nil.
	ListPaged(ctx context.Context, opt *ListOptions) (*ListPage, error)

	// NewRangeReader returns a Reader that reads part of an object, reading at
	// most length bytes starting at the given offset. If length is negative, it
	// will read until the end of the object. If the specified object does not
	// exist, NewRangeReader must return an error whose Kind method returns
	// NotFound.
	NewRangeReader(ctx context.Context, key string, offset, length int64) (Reader, error)

	// NewTypedWriter returns Writer that writes to an object associated with key.
	//
	// A new object will be created unless an object with this key already exists.
	// Otherwise any previous object with the same key will be replaced.
	// The object may not be available (and any previous object will remain)
	// until Close has been called.
	//
	// contentType sets the MIME type of the object to be written. It must not be
	// empty.
	//
	// The caller must call Close on the returned Writer when done writing.
	//
	// Implementations should abort an ongoing write if ctx is later canceled,
	// and do any necessary cleanup in Close. Close should then return ctx.Err().
	NewTypedWriter(ctx context.Context, key string, contentType string, opt *WriterOptions) (Writer, error)

	// Delete deletes the object associated with key. If the specified object does
	// not exist, NewRangeReader must return an error whose Kind method
	// returns NotFound.
	Delete(ctx context.Context, key string) error

	// SignedURL returns a URL that can be used to GET the blob for the duration
	// specified in opts.Expiry. opts is guaranteed to be non-nil.
	// If not supported, return an error whose Kind method returns NotImplemented.
	SignedURL(ctx context.Context, key string, opts *SignedURLOptions) (string, error)
}

// SignedURLOptions sets options for SignedURL.
type SignedURLOptions struct {
	// Expiry sets how long the returned URL is valid for. It is guaranteed to be > 0.
	Expiry time.Duration
}
