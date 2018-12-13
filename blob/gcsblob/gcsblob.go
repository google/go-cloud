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

// Package gcsblob provides a blob implementation that uses GCS. Use OpenBucket
// to construct a blob.Bucket.
//
// Open URLs
//
// For blob.Open URLs, gcsblob registers for the scheme "gs"; URLs start
// with "gs://".
//
// The URL's Host is used as the bucket name.
// The following query options are supported:
//
//  - cred_path: Sets path to the Google credentials file. If unset, default
//    credentials are loaded.
//    See https://cloud.google.com/docs/authentication/production.
//  - access_id: Sets Options.GoogleAccessID.
//  - private_key_path: Sets path to a private key, which is read and used
//    to set Options.PrivateKey.
// Example URL:
//  gs://mybucket
//
// As
//
// gcsblob exposes the following types for As:
//  - Bucket: *storage.Client
//  - Error: *googleapi.Error
//  - ListObject: storage.ObjectAttrs
//  - ListOptions.BeforeList: *storage.Query
//  - Reader: storage.Reader
//  - Attributes: storage.ObjectAttrs
//  - WriterOptions.BeforeWrite: *storage.Writer
package gcsblob // import "gocloud.dev/blob/gcsblob"

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"net/url"
	"sort"
	"strings"
	"time"

	"gocloud.dev/blob"
	"gocloud.dev/blob/driver"
	"gocloud.dev/gcp"

	"cloud.google.com/go/storage"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

const defaultPageSize = 1000

func init() {
	blob.Register("gs", openURL)
}

func openURL(ctx context.Context, u *url.URL) (driver.Bucket, error) {
	q := u.Query()
	opts := &Options{}

	if accessID := q["access_id"]; len(accessID) > 0 {
		opts.GoogleAccessID = accessID[0]
	}

	if keyPath := q["private_key_path"]; len(keyPath) > 0 {
		pk, err := ioutil.ReadFile(keyPath[0])
		if err != nil {
			return nil, err
		}
		opts.PrivateKey = pk
	}

	var creds *google.Credentials
	if credPath := q["cred_path"]; len(credPath) == 0 {
		var err error
		creds, err = gcp.DefaultCredentials(ctx)
		if err != nil {
			return nil, err
		}
	} else {
		jsonCreds, err := ioutil.ReadFile(credPath[0])
		if err != nil {
			return nil, err
		}
		creds, err = google.CredentialsFromJSON(ctx, jsonCreds)
		if err != nil {
			return nil, err
		}
	}

	client, err := gcp.NewHTTPClient(gcp.DefaultTransport(), gcp.CredentialsTokenSource(creds))
	if err != nil {
		return nil, err
	}
	return openBucket(ctx, u.Host, client, opts)
}

// Options sets options for constructing a *blob.Bucket backed by GCS.
type Options struct {
	// GoogleAccessID represents the authorizer for SignedURL.
	// Required to use SignedURL.
	// See https://godoc.org/cloud.google.com/go/storage#SignedURLOptions.
	GoogleAccessID string

	// PrivateKey is the Google service account private key.
	// Exactly one of PrivateKey or SignBytes must be non-nil to use SignedURL.
	// See https://godoc.org/cloud.google.com/go/storage#SignedURLOptions.
	PrivateKey []byte

	// SignBytes is a function for implementing custom signing.
	// Exactly one of PrivateKey or SignBytes must be non-nil to use SignedURL.
	// See https://godoc.org/cloud.google.com/go/storage#SignedURLOptions.
	SignBytes func([]byte) ([]byte, error)
}

// openBucket returns a GCS Bucket that communicates using the given HTTP client.
func openBucket(ctx context.Context, bucketName string, client *gcp.HTTPClient, opts *Options) (*bucket, error) {
	if client == nil {
		return nil, errors.New("gcsblob.OpenBucket: client is required")
	}
	if bucketName == "" {
		return nil, errors.New("gcsblob.OpenBucket: bucketName is required")
	}
	c, err := storage.NewClient(ctx, option.WithHTTPClient(&client.Client))
	if err != nil {
		return nil, err
	}
	if opts == nil {
		opts = &Options{}
	}
	return &bucket{name: bucketName, client: c, opts: opts}, nil
}

// OpenBucket returns a *blob.Bucket backed by GCS. See the package
// documentation for an example.
func OpenBucket(ctx context.Context, bucketName string, client *gcp.HTTPClient, opts *Options) (*blob.Bucket, error) {
	drv, err := openBucket(ctx, bucketName, client, opts)
	if err != nil {
		return nil, err
	}
	return blob.NewBucket(drv), nil
}

// bucket represents a GCS bucket, which handles read, write and delete operations
// on objects within it.
type bucket struct {
	name   string
	client *storage.Client
	opts   *Options
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

// IsNotExist implements driver.IsNotExist.
func (b *bucket) IsNotExist(err error) bool {
	return err == storage.ErrObjectNotExist
}

// IsNotImplemented implements driver.IsNotImplemented.
func (b *bucket) IsNotImplemented(err error) bool {
	return false
}

// ListPaged implements driver.ListPaged.
func (b *bucket) ListPaged(ctx context.Context, opts *driver.ListOptions) (*driver.ListPage, error) {
	bkt := b.client.Bucket(b.name)
	query := &storage.Query{
		Prefix:    opts.Prefix,
		Delimiter: opts.Delimiter,
	}
	if opts.BeforeList != nil {
		asFunc := func(i interface{}) bool {
			p, ok := i.(**storage.Query)
			if !ok {
				return false
			}
			*p = query
			return true
		}
		if err := opts.BeforeList(asFunc); err != nil {
			return nil, err
		}
	}
	pageSize := opts.PageSize
	if pageSize == 0 {
		pageSize = defaultPageSize
	}
	iter := bkt.Objects(ctx, query)
	pager := iterator.NewPager(iter, pageSize, string(opts.PageToken))
	var objects []*storage.ObjectAttrs
	nextPageToken, err := pager.NextPage(&objects)
	if err != nil {
		return nil, err
	}
	page := driver.ListPage{NextPageToken: []byte(nextPageToken)}
	if len(objects) > 0 {
		page.Objects = make([]*driver.ListObject, len(objects))
		for i, obj := range objects {
			asFunc := func(i interface{}) bool {
				p, ok := i.(*storage.ObjectAttrs)
				if !ok {
					return false
				}
				*p = *obj
				return true
			}
			if obj.Prefix == "" {
				// Regular blob.
				page.Objects[i] = &driver.ListObject{
					Key:     obj.Name,
					ModTime: obj.Updated,
					Size:    obj.Size,
					AsFunc:  asFunc,
				}
			} else {
				// "Directory".
				page.Objects[i] = &driver.ListObject{
					Key:    obj.Prefix,
					IsDir:  true,
					AsFunc: asFunc,
				}
			}
		}
		// GCS always returns "directories" at the end; sort them.
		sort.Slice(page.Objects, func(i, j int) bool {
			return page.Objects[i].Key < page.Objects[j].Key
		})
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

// As implements driver.ErrorAs.
func (b *bucket) ErrorAs(err error, i interface{}) bool {
	switch v := err.(type) {
	case *googleapi.Error:
		if p, ok := i.(**googleapi.Error); ok {
			*p = v
			return true
		}
	}
	return false
}

// Attributes implements driver.Attributes.
func (b *bucket) Attributes(ctx context.Context, key string) (driver.Attributes, error) {
	bkt := b.client.Bucket(b.name)
	obj := bkt.Object(key)
	attrs, err := obj.Attrs(ctx)
	if err != nil {
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
func (b *bucket) NewRangeReader(ctx context.Context, key string, offset, length int64, opts *driver.ReaderOptions) (driver.Reader, error) {
	bkt := b.client.Bucket(b.name)
	obj := bkt.Object(key)
	r, err := obj.NewRangeReader(ctx, offset, length)
	if err != nil {
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
	w.ChunkSize = bufferSize(opts.BufferSize)
	w.Metadata = opts.Metadata
	w.MD5 = opts.ContentMD5
	if opts.BeforeWrite != nil {
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
	return obj.Delete(ctx)
}

func (b *bucket) SignedURL(ctx context.Context, key string, dopts *driver.SignedURLOptions) (string, error) {
	if b.opts.GoogleAccessID == "" || (b.opts.PrivateKey == nil && b.opts.SignBytes == nil) {
		return "", errors.New("to use SignedURL, you must call OpenBucket with a valid Options.GoogleAccessID and exactly one of Options.PrivateKey or Options.SignBytes")
	}
	opts := &storage.SignedURLOptions{
		Expires:        time.Now().Add(dopts.Expiry),
		Method:         "GET",
		GoogleAccessID: b.opts.GoogleAccessID,
		PrivateKey:     b.opts.PrivateKey,
		SignBytes:      b.opts.SignBytes,
	}
	return storage.SignedURL(b.name, key, opts)
}

func bufferSize(size int) int {
	if size == 0 {
		return googleapi.DefaultUploadChunkSize
	} else if size > 0 {
		return size
	}
	return 0 // disable buffering
}
