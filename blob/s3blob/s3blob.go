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

// Package s3blob provides an implementation of using blob API on S3.
package s3blob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/driver"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

// OpenBucket returns an S3 Bucket.
func OpenBucket(ctx context.Context, sess client.ConfigProvider, bucketName string) (*blob.Bucket, error) {
	if sess == nil {
		return nil, errors.New("sess must be provided to get bucket")
	}
	svc := s3.New(sess)
	uploader := s3manager.NewUploader(sess)
	return blob.NewBucket(&bucket{
		name:     bucketName,
		client:   svc,
		uploader: uploader,
	}), nil
}

var emptyBody = ioutil.NopCloser(strings.NewReader(""))

// reader reads an S3 object. It implements io.ReadCloser.
type reader struct {
	body        io.ReadCloser
	size        int64
	contentType string
}

func (r *reader) Read(p []byte) (int, error) {
	return r.body.Read(p)
}

// Close closes the reader itself. It must be called when done reading.
func (r *reader) Close() error {
	return r.body.Close()
}

func (r *reader) Attrs() *driver.ObjectAttrs {
	return &driver.ObjectAttrs{
		Size:        r.size,
		ContentType: r.contentType,
	}
}

// writer writes an S3 object, it implements io.WriteCloser.
type writer struct {
	w *io.PipeWriter

	bucket      string
	key         string
	bufferSize  int
	ctx         context.Context
	uploader    *s3manager.Uploader
	contentType string
	donec       chan struct{} // closed when done writing
	// The following fields will be written before donec closes:
	err error
}

// Write appends p to w. User must call Close to close the w after done writing.
func (w *writer) Write(p []byte) (int, error) {
	if w.w == nil {
		if err := w.open(); err != nil {
			return 0, err
		}
	}
	select {
	case <-w.donec:
		return 0, w.err
	default:
	}
	return w.w.Write(p)
}

func (w *writer) open() error {
	pr, pw := io.Pipe()
	w.w = pw

	go func() {
		defer close(w.donec)

		_, err := w.uploader.UploadWithContext(w.ctx, &s3manager.UploadInput{
			Bucket:      aws.String(w.bucket),
			ContentType: aws.String(w.contentType),
			Key:         aws.String(w.key),
			Body:        pr,
		})
		if err != nil {
			w.err = err
			pr.CloseWithError(err)
			return
		}
	}()
	return nil
}

// Close completes the writer and close it. Any error occuring during write will
// be returned. If a writer is closed before any Write is called, Close will
// create an empty file at the given key.
func (w *writer) Close() error {
	if w.w == nil {
		w.touch()
	} else if err := w.w.Close(); err != nil {
		return err
	}
	<-w.donec
	return w.err
}

// touch creates an empty object in the bucket. It is called if user creates a
// new writer but never calls write before closing it.
func (w *writer) touch() {
	if w.w != nil {
		return
	}
	defer close(w.donec)
	_, w.err = w.uploader.UploadWithContext(w.ctx, &s3manager.UploadInput{
		Bucket:      aws.String(w.bucket),
		ContentType: aws.String(w.contentType),
		Key:         aws.String(w.key),
		Body:        emptyBody,
	})
}

// bucket represents an S3 bucket and handles read, write and delete operations.
type bucket struct {
	name     string
	client   *s3.S3
	uploader *s3manager.Uploader
}

// NewRangeReader returns a reader that reads part of an object, reading at most
// length bytes starting at the given offset. If length is 0, it will read only
// the metadata. If length is negative, it will read till the end of the object.
func (b *bucket) NewRangeReader(ctx context.Context, key string, offset, length int64) (driver.Reader, error) {
	if offset < 0 {
		return nil, fmt.Errorf("negative offset %d", offset)
	}
	if length == 0 {
		return b.newMetadataReader(ctx, key)
	}
	in := &s3.GetObjectInput{
		Bucket: aws.String(b.name),
		Key:    aws.String(key),
	}
	if offset > 0 && length < 0 {
		in.Range = aws.String(fmt.Sprintf("bytes=%d-", offset))
	} else if length > 0 {
		in.Range = aws.String(fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))
	}
	req, resp := b.client.GetObjectRequest(in)
	if err := req.Send(); err != nil {
		if e := isErrNotExist(err); e != nil {
			return nil, s3Error{bucket: b.name, key: key, msg: e.Message(), kind: driver.NotFound}
		}
		return nil, err
	}
	return &reader{
		body:        resp.Body,
		contentType: aws.StringValue(resp.ContentType),
		size:        aws.Int64Value(resp.ContentLength),
	}, nil
}

func (b *bucket) newMetadataReader(ctx context.Context, key string) (driver.Reader, error) {
	in := &s3.HeadObjectInput{
		Bucket: aws.String(b.name),
		Key:    aws.String(key),
	}
	req, resp := b.client.HeadObjectRequest(in)
	if err := req.Send(); err != nil {
		if e := isErrNotExist(err); e != nil {
			return nil, s3Error{bucket: b.name, key: key, msg: e.Message(), kind: driver.NotFound}
		}
		return nil, err
	}
	return &reader{
		body:        emptyBody,
		contentType: aws.StringValue(resp.ContentType),
		size:        aws.Int64Value(resp.ContentLength),
	}, nil
}

// NewTypedWriter returns a writer that writes to an object associated with key.
//
// A new object will be created unless an object with this key already exists.
// Otherwise any previous object with the same name will be replaced.
// The object will not be available (and any previous object will remain)
// until Close has been called.
//
// A WriterOptions can be given to change the default behavior of the writer.
//
// The caller must call Close on the returned writer when done writing.
func (b *bucket) NewTypedWriter(ctx context.Context, key string, contentType string, opts *driver.WriterOptions) (driver.Writer, error) {
	w := &writer{
		bucket:      b.name,
		ctx:         ctx,
		key:         key,
		uploader:    b.uploader,
		contentType: contentType,
		donec:       make(chan struct{}),
	}
	if opts != nil {
		w.bufferSize = opts.BufferSize
	}
	return w, nil
}

// Delete deletes the object associated with key. It is a no-op if that object
// does not exist.
func (b *bucket) Delete(ctx context.Context, key string) error {
	if _, err := b.newMetadataReader(ctx, key); err != nil {
		return err
	}
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(b.name),
		Key:    aws.String(key),
	}

	req, _ := b.client.DeleteObjectRequest(input)
	return req.Send()
}

type s3Error struct {
	bucket, key, msg string
	kind             driver.ErrorKind
}

func (e s3Error) BlobError() driver.ErrorKind {
	return e.kind
}

func (e s3Error) Error() string {
	return fmt.Sprintf("s3://%s/%s: %s", e.bucket, e.key, e.msg)
}

func isErrNotExist(err error) awserr.Error {
	if e, ok := err.(awserr.Error); ok && e.Code() == "NotFound" {
		return e
	}
	return nil
}
