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

// Package gcsblob provides an implementation of blob that uses GCS.
//
// It exposes the following types for As:
// Bucket: *storage.Client
// Reader: storage.Reader
// Attributes: storage.ObjectAttrs
// Writer: *storage.Writer
package gcsblob

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/driver"
	"github.com/google/go-cloud/gcp"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// OpenBucket returns a GCS Bucket that communicates using the given HTTP client.
func OpenBucket(ctx context.Context, bucketName string, client *gcp.HTTPClient) (*blob.Bucket, error) {
	if client == nil {
		return nil, fmt.Errorf("NewBucket requires an HTTP client to communicate using")
	}
	c, err := storage.NewClient(ctx, option.WithHTTPClient(&client.Client))
	if err != nil {
		return nil, err
	}
	return blob.NewBucket(&bucket{name: bucketName, client: c}), nil
}

// bucket represents a GCS bucket, which handles read, write and delete operations
// on objects within it.
type bucket struct {
	name   string
	client *storage.Client
}

var emptyBody = ioutil.NopCloser(strings.NewReader(""))

// reader reads a GCS object. It implements driver.Reader.
type reader struct {
	body  io.ReadCloser
	attrs driver.ReaderAttributes
	raw   *storage.Reader
}

func (r *reader) Read(p []byte) (int, error) {
	return r.body.Read(p)
}

// Close closes the reader itself. It must be called when done reading.
func (r *reader) Close() error {
	return r.body.Close()
}

func (r *reader) Attributes() driver.ReaderAttributes {
	return r.attrs
}

func (r *reader) As(i interface{}) bool {
	p, ok := i.(*storage.Reader)
	if !ok {
		return false
	}
	*p = *r.raw
	return true
}

// ListPaged implements driver.ListPaged.
func (b *bucket) ListPaged(ctx context.Context, opt *driver.ListOptions) (*driver.ListPage, error) {
	bkt := b.client.Bucket(b.name)
	iter := bkt.Objects(ctx, &storage.Query{Prefix: opt.Prefix})
	pager := iterator.NewPager(iter, opt.PageSize, opt.PageToken)

	var objects []*storage.ObjectAttrs
	nextPageToken, err := pager.NextPage(&objects)
	if err != nil {
		return nil, err
	}
	page := driver.ListPage{NextPageToken: nextPageToken}
	if len(objects) > 0 {
		page.Objects = make([]*driver.ListObject, len(objects))
		for i, obj := range objects {
			page.Objects[i] = &driver.ListObject{
				Key:     obj.Name,
				ModTime: obj.Updated,
				Size:    obj.Size,
			}
		}
	}
	return &page, nil
}

// As implements driver.As.
func (b *bucket) As(i interface{}) bool {
	p, ok := i.(**storage.Client)
	if !ok {
		return false
	}
	*p = b.client
	return true
}

// Attributes implements driver.Attributes.
func (b *bucket) Attributes(ctx context.Context, key string) (driver.Attributes, error) {
	bkt := b.client.Bucket(b.name)
	obj := bkt.Object(key)
	attrs, err := obj.Attrs(ctx)
	if err != nil {
		if isErrNotExist(err) {
			return driver.Attributes{}, gcsError{bucket: b.name, key: key, msg: err.Error(), kind: driver.NotFound}
		}
		return driver.Attributes{}, err
	}
	return driver.Attributes{
		ContentType: attrs.ContentType,
		Metadata:    attrs.Metadata,
		ModTime:     attrs.Updated,
		Size:        attrs.Size,
		AsFunc: func(i interface{}) bool {
			p, ok := i.(*storage.ObjectAttrs)
			if !ok {
				return false
			}
			*p = *attrs
			return true
		},
	}, nil
}

// NewRangeReader implements driver.NewRangeReader.
func (b *bucket) NewRangeReader(ctx context.Context, key string, offset, length int64) (driver.Reader, error) {
	bkt := b.client.Bucket(b.name)
	obj := bkt.Object(key)
	r, err := obj.NewRangeReader(ctx, offset, length)
	if err != nil {
		if isErrNotExist(err) {
			return nil, gcsError{bucket: b.name, key: key, msg: err.Error(), kind: driver.NotFound}
		}
		return nil, err
	}
	modTime, _ := r.LastModified()
	return &reader{
		body: r,
		attrs: driver.ReaderAttributes{
			ContentType: r.ContentType(),
			ModTime:     modTime,
			Size:        r.Size(),
		},
		raw: r,
	}, nil
}

// NewTypedWriter implements driver.NewTypedWriter.
func (b *bucket) NewTypedWriter(ctx context.Context, key string, contentType string, opts *driver.WriterOptions) (driver.Writer, error) {
	bkt := b.client.Bucket(b.name)
	obj := bkt.Object(key)
	w := obj.NewWriter(ctx)
	w.ContentType = contentType
	if opts != nil {
		w.ChunkSize = bufferSize(opts.BufferSize)
		w.Metadata = opts.Metadata
	}
	if opts != nil && opts.BeforeWrite != nil {
		asFunc := func(i interface{}) bool {
			p, ok := i.(**storage.Writer)
			if !ok {
				return false
			}
			*p = w
			return true
		}
		if err := opts.BeforeWrite(asFunc); err != nil {
			return nil, err
		}
	}
	return w, nil
}

// Delete implements driver.Delete.
func (b *bucket) Delete(ctx context.Context, key string) error {
	bkt := b.client.Bucket(b.name)
	obj := bkt.Object(key)
	err := obj.Delete(ctx)
	if isErrNotExist(err) {
		return gcsError{bucket: b.name, key: key, msg: err.Error(), kind: driver.NotFound}
	}
	return err
}

func bufferSize(size int) int {
	if size == 0 {
		return googleapi.DefaultUploadChunkSize
	} else if size > 0 {
		return size
	}
	return 0 // disable buffering
}

type gcsError struct {
	bucket, key, msg string
	kind             driver.ErrorKind
}

func (e gcsError) Error() string {
	return fmt.Sprintf("gcs://%s/%s: %s", e.bucket, e.key, e.msg)
}

func (e gcsError) Kind() driver.ErrorKind {
	return e.kind
}

func isErrNotExist(err error) bool {
	return err == storage.ErrObjectNotExist
}
