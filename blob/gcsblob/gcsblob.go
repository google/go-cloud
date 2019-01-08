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
// to construct a *blob.Bucket.
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
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"gocloud.dev/blob"
	"gocloud.dev/blob/driver"
	"gocloud.dev/gcp"
	"gocloud.dev/internal/useragent"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

const defaultPageSize = 1000

// Scheme is the URL scheme conventionally used for GCS in a URLMux.
const Scheme = "gs"

// URLOpener opens GCS URLs like "gs://mybucket".
type URLOpener struct {
	Client  *gcp.HTTPClient
	Options Options

	// AllowURLOverrides permits the fields above to be overridden in the
	// URL using the following query parameters:
	//
	//     access_id: sets Options.GoogleAccessID
	//     cred_path: path to credentials for the Client
	//     private_key_path: path to read for Options.PrivateKey
	//
	// If this is true and Client is nil and cred_path is not given, then
	// OpenBucketURL will use Application Default Credentials.
	AllowURLOverrides bool
}

// OpenBucketURL opens the GCS bucket with the same name of the URL's host.
func (o *URLOpener) OpenBucketURL(ctx context.Context, u *url.URL) (*blob.Bucket, error) {
	var err error
	o, err = o.forParams(ctx, u.Query())
	if err != nil {
		return nil, fmt.Errorf("open bucket %v: %v", u, err)
	}
	return OpenBucket(ctx, o.Client, u.Host, &o.Options)
}

func (o *URLOpener) forParams(ctx context.Context, q url.Values) (*URLOpener, error) {
	for k := range q {
		if !o.AllowURLOverrides || k != "access_id" && k != "cred_path" && k != "private_key_path" {
			return nil, fmt.Errorf("unknown GCS query parameter %s", k)
		}
	}
	if !o.AllowURLOverrides {
		return o, nil
	}
	o2 := new(URLOpener)
	*o2 = *o
	if accessID := q.Get("access_id"); accessID != "" {
		o2.Options.GoogleAccessID = accessID
	}

	if keyPath := q.Get("private_key_path"); keyPath != "" {
		pk, err := ioutil.ReadFile(keyPath)
		if err != nil {
			return nil, err
		}
		o2.Options.PrivateKey = pk
	}

	if credPath := q.Get("cred_path"); credPath != "" {
		jsonCreds, err := ioutil.ReadFile(credPath)
		if err != nil {
			return nil, err
		}
		creds, err := google.CredentialsFromJSON(ctx, jsonCreds)
		if err != nil {
			return nil, err
		}
		o2.Client, err = gcp.NewHTTPClient(gcp.DefaultTransport(), creds.TokenSource)
		if err != nil {
			return nil, err
		}
	}
	if o2.Client == nil {
		var err error
		creds, err := gcp.DefaultCredentials(ctx)
		if err != nil {
			return nil, err
		}
		o2.Client, err = gcp.NewHTTPClient(gcp.DefaultTransport(), creds.TokenSource)
		if err != nil {
			return nil, err
		}
	}
	return o2, nil
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
func openBucket(ctx context.Context, client *gcp.HTTPClient, bucketName string, opts *Options) (*bucket, error) {
	if client == nil {
		return nil, errors.New("gcsblob.OpenBucket: client is required")
	}
	if bucketName == "" {
		return nil, errors.New("gcsblob.OpenBucket: bucketName is required")
	}
	// We wrap the provided http.Client to add a Go Cloud User-Agent.
	c, err := storage.NewClient(ctx, option.WithHTTPClient(useragent.HTTPClient(&client.Client, "blob")))
	if err != nil {
		return nil, err
	}
	if opts == nil {
		opts = &Options{}
	}
	return &bucket{name: bucketName, client: c, opts: opts}, nil
}

// OpenBucket returns a *blob.Bucket backed by an existing GCS bucket. See the
// package documentation for an example.
func OpenBucket(ctx context.Context, client *gcp.HTTPClient, bucketName string, opts *Options) (*blob.Bucket, error) {
	drv, err := openBucket(ctx, client, bucketName, opts)
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
					MD5:     obj.MD5,
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
		CacheControl:       attrs.CacheControl,
		ContentDisposition: attrs.ContentDisposition,
		ContentEncoding:    attrs.ContentEncoding,
		ContentLanguage:    attrs.ContentLanguage,
		ContentType:        attrs.ContentType,
		Metadata:           attrs.Metadata,
		ModTime:            attrs.Updated,
		Size:               attrs.Size,
		MD5:                attrs.MD5,
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
	w.CacheControl = opts.CacheControl
	w.ContentDisposition = opts.ContentDisposition
	w.ContentEncoding = opts.ContentEncoding
	w.ContentLanguage = opts.ContentLanguage
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
