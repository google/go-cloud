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

// Package gcsblob provides an implementation of using blob API on GCS.
// It is a wrapper around GCS client library.
package gcsblob

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"unicode/utf8"

	"github.com/google/go-x-cloud/blob"
	"github.com/google/go-x-cloud/blob/driver"
	"github.com/google/go-x-cloud/gcp"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// NewBucket returns a GCS Bucket that communicates using the passed HTTP client.
func NewBucket(ctx context.Context, bucketName string, client *gcp.HTTPClient) (*blob.Bucket, error) {
	if err := validateBucketChar(bucketName); err != nil {
		return nil, err
	}
	if client != nil {
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

type reader struct {
	*storage.Reader
}

func (r *reader) Attrs() *driver.ObjectAttrs {
	return &driver.ObjectAttrs{
		Size:        r.Size(),
		ContentType: r.ContentType(),
	}
}

// NewRangeReader returns a Reader that reads part of an object, reading at most
// length bytes starting at the given offset. If length is 0, it will read only
// the metadata. If length is negative, it will read till the end of the object.
func (b *bucket) NewRangeReader(ctx context.Context, key string, offset, length int64) (driver.Reader, error) {
	bkt := b.client.Bucket(b.name)
	obj := bkt.Object(key)
	r, err := obj.NewRangeReader(ctx, offset, length)
	if isErrNotExist(err) {
		return nil, gcsError{bucket: b.name, key: key, msg: err.Error(), kind: driver.NotFound}
	}
	return &reader{Reader: r}, err
}

// NewWriter returns Writer that writes to an object associated with key.
//
// A new object will be created unless an object with this key already exists.
// Otherwise any previous object with the same name will be replaced.
// The object will not be available (and any previous object will remain)
// until Close has been called.
//
// A WriterOptions can be given to change the default behavior of the Writer.
//
// The caller must call Close on the returned Writer when done writing.
func (b *bucket) NewWriter(ctx context.Context, key string, contentType string, opts *driver.WriterOptions) (driver.Writer, error) {
	if err := validateObjectChar(key); err != nil {
		return nil, err
	}
	bkt := b.client.Bucket(b.name)
	obj := bkt.Object(key)
	w := obj.NewWriter(ctx)
	w.ContentType = contentType
	if opts != nil {
		w.ChunkSize = bufferSize(opts.BufferSize)
	}
	return w, nil
}

// Delete deletes the object associated with key. It is a no-op if that object
// does not exist.
func (b *bucket) Delete(ctx context.Context, key string) error {
	bkt := b.client.Bucket(b.name)
	obj := bkt.Object(key)
	err := obj.Delete(ctx)
	if isErrNotExist(err) {
		return gcsError{bucket: b.name, key: key, msg: err.Error(), kind: driver.NotFound}
	}
	return err
}

const namingRuleURL = "https://cloud.google.com/storage/docs/naming"

// validateBucketChar checks whether character set and length meet the general requirement
// of bucket naming rule. See https://cloud.google.com/storage/docs/naming for
// the full requirements and best practice.
func validateBucketChar(name string) error {
	v := regexp.MustCompile(`^[a-z0-9][a-z0-9-_.]{1,220}[a-z0-9]$`)
	if !v.MatchString(name) {
		return fmt.Errorf("invalid bucket name, see %s for detailed requirements", namingRuleURL)
	}
	return nil
}

// validateObjectChar checks whether name is a valid UTF-8 encoded string, and its
// length is between 1-1024 bytes. See https://cloud.google.com/storage/docs/naming
// for the full requirements and best practice.
func validateObjectChar(name string) error {
	if name == "" {
		return errors.New("object name is empty")
	}
	if !utf8.ValidString(name) {
		return fmt.Errorf("object name is not valid UTF-8, see %s for detailed requirements", namingRuleURL)
	}
	if len(name) > 1024 {
		return fmt.Errorf("object name is longer than 1024 bytes, see %s for detailed requirements", namingRuleURL)
	}
	return nil
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

func (e gcsError) BlobError() driver.ErrorKind {
	return e.kind
}

func isErrNotExist(err error) bool {
	return err == storage.ErrObjectNotExist
}
