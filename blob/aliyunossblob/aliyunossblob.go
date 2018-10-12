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

// Package aliyunossblob provides an implementation of using blob API on aliyunoss.
package aliyunossblob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"bytes"
	"net/http"
	"strconv"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/driver"
	"github.com/spf13/cast"
)

const (
	bufferSize = 4 * 1024 * 1024
)

type bucket struct {
	name   string
	client *oss.Client
}

// OpenBucket returns an AliyunOSS Bucket.
func OpenBucket(ctx context.Context, client *oss.Client, bucketName string) (*blob.Bucket, error) {
	if client == nil {
		return nil, errors.New("client must be provided to get bucket")
	}

	return blob.NewBucket(&bucket{
		name:   bucketName,
		client: client,
	}), nil
}

var emptyBody = ioutil.NopCloser(strings.NewReader(""))

// reader reads an AliyunOSS object. It implements io.ReadCloser.
type reader struct {
	body  io.ReadCloser
	attrs driver.ReaderAttributes
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

// writer writes an AliyunOSS object, it implements io.WriteCloser.
type writer struct {
	w            *io.PipeWriter
	ctx          context.Context
	aliyunBucket *oss.Bucket
	key          string
	contentType  string
	metadata     map[string]string
	imur         oss.InitiateMultipartUploadResult
	parts        []oss.UploadPart
	donec        chan struct{} // closed when done writing
	err          error
}

// Write appends p to w. User must call Close to close the w after done writing.
func (w *writer) Write(p []byte) (n int, err error) {
	werr := w.err
	if werr != nil {
		return 0, werr
	}
	if w.w == nil {
		if err := w.open(); err != nil {
			return 0, err
		}
	}

	n, err = w.w.Write(p)
	if err != nil {
		werr := w.err
		// Preserve existing functionality that when context is canceled, Write will return
		// context.Canceled instead of "io: read/write on closed pipe". This hides the
		// pipe implementation detail from users and makes Write seem as though it's an RPC.
		if werr == context.Canceled || werr == context.DeadlineExceeded {
			return n, werr
		}
	}
	return n, err

}

func (w *writer) open() error {
	pr, pw := io.Pipe()
	w.w = pw

	go w.monitorCancel()

	options := []oss.Option{
		oss.ContentType(w.contentType),
	}
	for k, v := range w.metadata {
		options = append(options, oss.Meta(strings.ToLower(k), v))
	}

	go func() {
		defer close(w.donec)

		body, err := ioutil.ReadAll(pr)
		if err != nil {
			w.err = err
			pr.CloseWithError(err)
			return
		}
		bytesR := bytes.NewReader(body)

		chunks, err := splitByteByPartSize(bytesR, int64(bufferSize))
		if err != nil {
			w.err = err
			pr.CloseWithError(err)
			return
		}

		w.imur, err = w.aliyunBucket.InitiateMultipartUpload(w.key, options...)
		if err != nil {
			w.err = err
			pr.CloseWithError(err)
			return
		}

		for _, chunk := range chunks {
			bytesR.Seek(chunk.Offset, io.SeekStart)
			part, err := w.aliyunBucket.UploadPart(w.imur, bytesR, chunk.Size, chunk.Number)
			if err != nil {
				w.err = err
				pr.CloseWithError(err)
				return
			}
			w.parts = append(w.parts, part)
		}

		_, err = w.aliyunBucket.CompleteMultipartUpload(w.imur, w.parts)
		if err != nil {
			w.err = err
			pr.CloseWithError(err)
			return
		}

	}()
	return nil
}

func splitByteByPartSize(r *bytes.Reader, chunkSize int64) ([]oss.FileChunk, error) {
	var chunkN = r.Size() / chunkSize
	if chunkN >= 10000 {
		return nil, errors.New("Too many parts, please increase part size ")
	}

	var chunks []oss.FileChunk
	var chunk = oss.FileChunk{}
	for i := int64(0); i < chunkN; i++ {
		chunk.Number = int(i + 1)
		chunk.Offset = i * chunkSize
		chunk.Size = chunkSize
		chunks = append(chunks, chunk)
	}

	if r.Size()%chunkSize > 0 {
		chunk.Number = len(chunks) + 1
		chunk.Offset = int64(len(chunks)) * chunkSize
		chunk.Size = r.Size() % chunkSize
		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

// Close completes the writer and close it. Any error occuring during write will
// be returned. If a writer is closed before any Write is called, Close will
// create an empty file at the given key.
func (w *writer) Close() error {
	if w.ctx.Err() != nil {
		w.aliyunBucket.DeleteObject(w.key)
	}

	if w.w != nil {
		if err := w.w.Close(); err != nil {
			return err
		}
	} else if w.ctx.Err() == nil {
		w.touch()
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
	w.err = w.aliyunBucket.PutObject(w.key, emptyBody)
}

// monitorCancel is intended to be used as a background goroutine. It monitors the
// the context, and when it observes that the context has been canceled, it manually
// closes things that do not take a context.
func (w *writer) monitorCancel() {
	select {
	case <-w.ctx.Done():
		werr := w.ctx.Err()
		w.err = werr

		// Closing either the read or write causes the entire pipe to close.
		w.w.CloseWithError(werr)
	case <-w.donec:
	}
}

// Attributes implements driver.Attributes.
func (b *bucket) Attributes(ctx context.Context, key string) (driver.Attributes, error) {
	bkt, _ := b.client.Bucket(b.name)
	meta, err := bkt.GetObjectDetailedMeta(key)
	if err != nil {
		if isErrNotExist(err) {
			return driver.Attributes{}, aliOSSError{bucket: b.name, key: key, msg: err.Error(), kind: driver.NotFound}
		}

		return driver.Attributes{}, err
	}

	metadata := make(map[string]string)
	for k := range meta {
		if strings.HasPrefix(k, oss.HTTPHeaderOssMetaPrefix) {
			metadata[strings.TrimPrefix(k, oss.HTTPHeaderOssMetaPrefix)] = meta.Get(k)
		}
	}

	return driver.Attributes{
		ContentType: meta.Get("Content-Type"),
		Metadata:    metadata,
		ModTime:     cast.ToTime(meta.Get("Last-Modified")),
		Size:        cast.ToInt64(meta.Get("Content-Length")),
	}, nil
}

// NewRangeReader implements driver.NewRangeReader.
func (b *bucket) NewRangeReader(ctx context.Context, key string, offset, length int64) (driver.Reader, error) {
	bkt, _ := b.client.Bucket(b.name)

	option := make([]oss.Option, 0)
	if offset > 0 && length < 0 {
		option = append(option, oss.NormalizedRange(fmt.Sprintf("%d-", offset)))
	} else if length > 0 {
		option = append(option, oss.Range(offset, offset+length-1))
	}

	result, err := bkt.DoGetObject(&oss.GetObjectRequest{ObjectKey: key}, option)
	if err != nil {
		if isErrNotExist(err) {
			return nil, aliOSSError{bucket: b.name, key: key, msg: err.Error(), kind: driver.NotFound}
		}
		return nil, err
	}

	fmt.Println(result.Response.Headers)

	return &reader{
		body: result.Response.Body,
		attrs: driver.ReaderAttributes{
			ModTime:     cast.ToTime(result.Response.Headers.Get("Last-Modified")),
			ContentType: result.Response.Headers.Get("Content-Type"),
			Size:        getSize(&result.Response.Headers),
		},
	}, nil
}

func getSize(headers *http.Header) int64 {
	// Default size to ContentLength, but that's incorrect for partial-length reads,
	// where ContentLength refers to the size of the returned Body, not the entire
	// size of the blob. ContentRange has the full size.
	size := cast.ToInt64(headers.Get("Content-Length"))
	if cr := headers.Get("Content-Range"); cr != "" {
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
	bkt, _ := b.client.Bucket(b.name)

	metadata := map[string]string{}
	if opts != nil {
		metadata = opts.Metadata
	}

	w := &writer{
		ctx:          ctx,
		key:          key,
		aliyunBucket: bkt,
		contentType:  contentType,
		metadata:     metadata,
		donec:        make(chan struct{}),
	}
	return w, nil
}

// Delete implements driver.Delete.
func (b *bucket) Delete(ctx context.Context, key string) error {
	bkt, _ := b.client.Bucket(b.name)

	_, err := bkt.GetObjectMeta(key)
	if err != nil {
		if isErrNotExist(err) {
			return aliOSSError{bucket: b.name, key: key, msg: err.Error(), kind: driver.NotFound}
		}
		return err
	}
	return bkt.DeleteObject(key)
}

type aliOSSError struct {
	bucket, key, msg string
	kind             driver.ErrorKind
}

func (e aliOSSError) Kind() driver.ErrorKind {
	return e.kind
}

func (e aliOSSError) Error() string {
	return fmt.Sprintf("aliyunoss://%s/%s: %s", e.bucket, e.key, e.msg)
}

func isErrNotExist(err error) bool {
	if e, ok := err.(oss.ServiceError); ok {
		if e.StatusCode == 404 && e.Code == "NoSuchKey" {
			return true
		}
	}
	return false
}
