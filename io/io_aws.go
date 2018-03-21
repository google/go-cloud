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

// +build aws

package io

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/s3manager"
	cloud "github.com/google/go-cloud"
	cloudaws "github.com/google/go-cloud/platforms/aws"
)

const s3ServiceName = "s3"

var s3Client *s3.S3
var s3ClientOnce sync.Once

func init() {
	RegisterBlobProvider(s3ServiceName, openS3Blob)
}

type s3Reader struct {
	o *s3.GetObjectOutput
}

func (r *s3Reader) Read(b []byte) (int, error) {
	return r.o.Body.Read(b)
}

func (r *s3Reader) Close() error {
	return r.o.Body.Close()
}

func (r *s3Reader) Size() int64 {
	return *r.o.ContentLength
}

type s3Writer struct {
	up    *s3manager.Uploader
	w     io.WriteCloser // pipe to writing goroutine
	errCh chan error     // error coming back from writing goroutine
}

func (w *s3Writer) Write(d []byte) (int, error) {
	return w.w.Write(d)
}

func (w *s3Writer) Close() error {
	if err := w.w.Close(); err != nil {
		return err
	}
	return <-w.errCh
}

type s3Blob struct {
	bucket, key string
}

func (b *s3Blob) NewReader(_ context.Context) (BlobReader, error) {
	return reader(&s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.key),
	})
}

func (b *s3Blob) NewRangeReader(_ context.Context, offset, length int64) (BlobReader, error) {
	return reader(&s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.key),
		Range:  aws.String(fmt.Sprintf("bytes=%d-%d", offset, offset+length)),
	})
}

func (b *s3Blob) NewWriter(ctx context.Context) (BlobWriter, error) {
	r, w := io.Pipe()
	ret := &s3Writer{
		up:    s3manager.NewUploader(cloudaws.Config()),
		w:     w,
		errCh: make(chan error, 1),
	}
	go func() {
		_, err := ret.up.UploadWithContext(ctx, &s3manager.UploadInput{
			Bucket: &b.bucket,
			Key:    &b.key,
			Body:   r,
		})
		ret.errCh <- err
	}()
	return ret, nil
}

func reader(input *s3.GetObjectInput) (*s3Reader, error) {
	req := s3Client.GetObjectRequest(input)
	result, err := req.Send()
	if err != nil {
		return nil, err
	}
	return &s3Reader{o: result}, nil
}

func openS3Blob(ctx context.Context, uri string) (Blob, error) {
	s3ClientOnce.Do(func() {
		s3Client = s3.New(cloudaws.Config())
	})

	service, path, err := cloud.Service(uri)
	if err != nil {
		return nil, err
	}
	if service != s3ServiceName {
		panic("bad handler routing: " + uri)
	}
	bucket, key := cloud.PopPath(path)
	return &s3Blob{bucket: bucket, key: key}, nil
}
