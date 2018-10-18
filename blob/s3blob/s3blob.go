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

// Package s3blob provides an implementation of blob using S3.
//
// It exposes the following types for As:
// Bucket: *s3.S3
// ListObject: s3.Object
// ListOptions.BeforeList: *s3.ListObjectsV2Input
// Reader: s3.GetObjectOutput
// Attributes: s3.HeadObjectOutput
// WriterOptions.BeforeWrite: *s3manager.UploadInput
package s3blob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
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
	return blob.NewBucket(&bucket{
		name:   bucketName,
		sess:   sess,
		client: s3.New(sess),
	}), nil
}

var emptyBody = ioutil.NopCloser(strings.NewReader(""))

// reader reads an S3 object. It implements io.ReadCloser.
type reader struct {
	body  io.ReadCloser
	attrs driver.ReaderAttributes
	raw   *s3.GetObjectOutput
}

func (r *reader) Read(p []byte) (int, error) {
	return r.body.Read(p)
}

// Close closes the reader itself. It must be called when done reading.
func (r *reader) Close() error {
	return r.body.Close()
}

func (r *reader) As(i interface{}) bool {
	p, ok := i.(*s3.GetObjectOutput)
	if !ok {
		return false
	}
	*p = *r.raw
	return true
}

func (r *reader) Attributes() driver.ReaderAttributes {
	return r.attrs
}

// writer writes an S3 object, it implements io.WriteCloser.
type writer struct {
	w *io.PipeWriter

	ctx      context.Context
	uploader *s3manager.Uploader
	req      *s3manager.UploadInput
	donec    chan struct{} // closed when done writing
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

		w.req.Body = pr
		_, err := w.uploader.UploadWithContext(w.ctx, w.req)
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
	w.req.Body = emptyBody
	_, w.err = w.uploader.UploadWithContext(w.ctx, w.req)
}

// bucket represents an S3 bucket and handles read, write and delete operations.
type bucket struct {
	name   string
	sess   client.ConfigProvider
	client *s3.S3
}

// ListPaged implements driver.ListPaged.
func (b *bucket) ListPaged(ctx context.Context, opt *driver.ListOptions) (*driver.ListPage, error) {
	in := &s3.ListObjectsV2Input{
		Bucket:  aws.String(b.name),
		MaxKeys: aws.Int64(int64(opt.PageSize)),
	}
	if len(opt.PageToken) > 0 {
		in.ContinuationToken = aws.String(string(opt.PageToken))
	}
	if opt.Prefix != "" {
		in.Prefix = aws.String(opt.Prefix)
	}
	if opt.BeforeList != nil {
		asFunc := func(i interface{}) bool {
			p, ok := i.(**s3.ListObjectsV2Input)
			if !ok {
				return false
			}
			*p = in
			return true
		}
		if err := opt.BeforeList(asFunc); err != nil {
			return nil, err
		}
	}
	req, resp := b.client.ListObjectsV2Request(in)
	if err := req.Send(); err != nil {
		return nil, err
	}
	page := driver.ListPage{}
	if resp.NextContinuationToken != nil {
		page.NextPageToken = []byte(*resp.NextContinuationToken)
	}
	if len(resp.Contents) > 0 {
		page.Objects = make([]*driver.ListObject, len(resp.Contents))
		for i, obj := range resp.Contents {
			page.Objects[i] = &driver.ListObject{
				Key:     *obj.Key,
				ModTime: *obj.LastModified,
				Size:    *obj.Size,
				AsFunc: func(i interface{}) bool {
					p, ok := i.(*s3.Object)
					if !ok {
						return false
					}
					*p = *obj
					return true
				},
			}
		}
	}
	return &page, nil
}

// As implements driver.As.
func (b *bucket) As(i interface{}) bool {
	p, ok := i.(**s3.S3)
	if !ok {
		return false
	}
	*p = b.client
	return true
}

// Attributes implements driver.Attributes.
func (b *bucket) Attributes(ctx context.Context, key string) (driver.Attributes, error) {
	in := &s3.HeadObjectInput{
		Bucket: aws.String(b.name),
		Key:    aws.String(key),
	}
	req, resp := b.client.HeadObjectRequest(in)
	if err := req.Send(); err != nil {
		if e := isErrNotExist(err); e != nil {
			return driver.Attributes{}, s3Error{bucket: b.name, key: key, msg: e.Message(), kind: driver.NotFound}
		}
		return driver.Attributes{}, err
	}
	var md map[string]string
	if len(resp.Metadata) > 0 {
		md = make(map[string]string, len(resp.Metadata))
		for k, v := range resp.Metadata {
			if v != nil {
				md[k] = aws.StringValue(v)
			}
		}
	}
	return driver.Attributes{
		ContentType: aws.StringValue(resp.ContentType),
		Metadata:    md,
		ModTime:     aws.TimeValue(resp.LastModified),
		Size:        aws.Int64Value(resp.ContentLength),
		AsFunc: func(i interface{}) bool {
			p, ok := i.(*s3.HeadObjectOutput)
			if !ok {
				return false
			}
			*p = *resp
			return true
		},
	}, nil
}

// NewRangeReader implements driver.NewRangeReader.
func (b *bucket) NewRangeReader(ctx context.Context, key string, offset, length int64) (driver.Reader, error) {
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
		body: resp.Body,
		attrs: driver.ReaderAttributes{
			ContentType: aws.StringValue(resp.ContentType),
			ModTime:     aws.TimeValue(resp.LastModified),
			Size:        getSize(resp),
		},
		raw: resp,
	}, nil
}

func getSize(resp *s3.GetObjectOutput) int64 {
	// Default size to ContentLength, but that's incorrect for partial-length reads,
	// where ContentLength refers to the size of the returned Body, not the entire
	// size of the blob. ContentRange has the full size.
	size := aws.Int64Value(resp.ContentLength)
	if cr := aws.StringValue(resp.ContentRange); cr != "" {
		// Sample: bytes 10-14/27 (where 27 is the full size).
		parts := strings.Split(cr, "/")
		if len(parts) == 2 {
			if i, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
				size = i
			}
		}
	}
	return size
}

// NewTypedWriter implements driver.NewTypedWriter.
func (b *bucket) NewTypedWriter(ctx context.Context, key string, contentType string, opts *driver.WriterOptions) (driver.Writer, error) {
	uploader := s3manager.NewUploader(b.sess, func(u *s3manager.Uploader) {
		if opts != nil {
			u.PartSize = int64(opts.BufferSize)
		}
	})
	var metadata map[string]*string
	if opts != nil && len(opts.Metadata) > 0 {
		metadata = make(map[string]*string, len(opts.Metadata))
		for k, v := range opts.Metadata {
			metadata[k] = aws.String(v)
		}
	}
	req := &s3manager.UploadInput{
		Bucket:      aws.String(b.name),
		ContentType: aws.String(contentType),
		Key:         aws.String(key),
		Metadata:    metadata,
	}
	if opts != nil && opts.BeforeWrite != nil {
		asFunc := func(i interface{}) bool {
			p, ok := i.(**s3manager.UploadInput)
			if !ok {
				return false
			}
			*p = req
			return true
		}
		if err := opts.BeforeWrite(asFunc); err != nil {
			return nil, err
		}
	}
	return &writer{
		ctx:      ctx,
		uploader: uploader,
		req:      req,
		donec:    make(chan struct{}),
	}, nil
}

// Delete implements driver.Delete.
func (b *bucket) Delete(ctx context.Context, key string) error {
	if _, err := b.Attributes(ctx, key); err != nil {
		return err
	}
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(b.name),
		Key:    aws.String(key),
	}
	req, _ := b.client.DeleteObjectRequest(input)
	return req.Send()
}

func (b *bucket) SignedURL(ctx context.Context, key string, opts *driver.SignedURLOptions) (string, error) {
	in := &s3.GetObjectInput{
		Bucket: aws.String(b.name),
		Key:    aws.String(key),
	}
	req, _ := b.client.GetObjectRequest(in)
	return req.Presign(opts.Expiry)
}

type s3Error struct {
	bucket, key, msg string
	kind             driver.ErrorKind
}

func (e s3Error) Kind() driver.ErrorKind {
	return e.kind
}

func (e s3Error) Error() string {
	return fmt.Sprintf("s3://%s/%s: %s", e.bucket, e.key, e.msg)
}

func isErrNotExist(err error) awserr.Error {
	if e, ok := err.(awserr.Error); ok && (e.Code() == "NoSuchKey" || e.Code() == "NotFound") {
		return e
	}
	return nil
}
