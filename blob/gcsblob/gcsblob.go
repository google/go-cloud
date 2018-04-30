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

	"cloud.google.com/go/storage"
	"github.com/google/go-cloud/blob/driver"
	"github.com/google/go-cloud/gcp"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

var _ driver.Bucket = (*Bucket)(nil)

// New returns a GCS Bucket. It handles creation of a client used to communicate
// to GCS service.
func New(ctx context.Context, bucketName string, opts *BucketOptions) (*Bucket, error) {
	if err := validateBucketChar(bucketName); err != nil {
		return nil, err
	}
	var o []option.ClientOption
	if opts != nil {
		o = append(o, option.WithTokenSource(opts.TokenSource))
	}
	c, err := storage.NewClient(ctx, o...)
	if err != nil {
		return nil, err
	}
	return &Bucket{name: bucketName, client: c}, nil
}

// Bucket represents a GCS bucket, which handles read, write and delete operations
// on objects within it.
type Bucket struct {
	name   string
	client *storage.Client
}

// BucketOptions provides information settings during bucket initialization.
type BucketOptions struct {
	TokenSource gcp.TokenSource
}

// NewRangeReader returns a Reader that reads part of an object, reading at most
// length bytes starting at the given offset. If length is 0, it will read only
// the metadata. If length is negative, it will read till the end of the object.
func (b *Bucket) NewRangeReader(ctx context.Context, key string, offset, length int64) (driver.Reader, error) {
	bkt := b.client.Bucket(b.name)
	obj := bkt.Object(key)
	return obj.NewRangeReader(ctx, offset, length)
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
func (b *Bucket) NewWriter(ctx context.Context, key string, opts *driver.WriterOptions) (driver.Writer, error) {
	if err := validateObjectChar(key); err != nil {
		return nil, err
	}
	bkt := b.client.Bucket(b.name)
	obj := bkt.Object(key)
	w := obj.NewWriter(ctx)
	if opts != nil {
		w.ChunkSize = bufferSize(opts.BufferSize)
	}
	return w, nil
}

// Delete deletes the object associated with key. It is a no-op if that object
// does not exist.
func (b *Bucket) Delete(ctx context.Context, key string) error {
	bkt := b.client.Bucket(b.name)
	obj := bkt.Object(key)
	return obj.Delete(ctx)
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
