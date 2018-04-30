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

// Package awsblob provides an implementation of using blob API on S3.
// It is built on top of AWS Go SDK v2.
package awsblob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/s3manager"
	"github.com/google/go-cloud/blob/driver"
)

var _ driver.Bucket = (*Bucket)(nil)

// New returns an S3 Bucket. It handles creation of a client used to communicate
// to S3. AWS config can be passed in through BucketOptions to change the default
// configuration of the S3Bucket.
func New(ctx context.Context, bucketName string, opts *BucketOptions) (*Bucket, error) {
	if err := validateBucketChar(bucketName); err != nil {
		return nil, err
	}
	if opts == nil {
		opts = new(BucketOptions)
	}
	if opts.AWSConfig == nil {
		cfg, err := external.LoadDefaultAWSConfig()
		if err != nil {
			return nil, err
		}
		opts.AWSConfig = &cfg
	}
	svc := s3.New(*opts.AWSConfig)
	return &Bucket{name: bucketName, client: svc, ecfg: opts.AWSConfig}, nil
}

var emptyBody = ioutil.NopCloser(strings.NewReader(""))

// Reader reads an S3 object. It implements io.ReadCloser.
type Reader struct {
	body io.ReadCloser
	size int64
}

func (r *Reader) Read(p []byte) (int, error) {
	return r.body.Read(p)
}

// Close closes the Reader itself. It must be called when done reading.
func (r *Reader) Close() error {
	return r.body.Close()
}

// Size returns the byte size of the object.
func (r *Reader) Size() int64 {
	return r.size
}

// Writer writes an S3 object, it implements io.WriteCloser.
type Writer struct {
	w *io.PipeWriter

	bucket     string
	key        string
	bufferSize int
	ctx        context.Context
	uploader   *s3manager.Uploader
	donec      chan struct{} // closed when done writing
	// The following fields will be written before donec closes:
	err error
}

// Write appends p to w. User must call Close to close the w after done writing.
func (w *Writer) Write(p []byte) (int, error) {
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

func (w *Writer) open() error {
	pr, pw := io.Pipe()
	w.w = pw

	go func() {
		defer close(w.donec)

		_, err := w.uploader.UploadWithContext(w.ctx, &s3manager.UploadInput{
			Bucket: aws.String(w.bucket),
			Key:    aws.String(w.key),
			Body:   pr,
		})
		if err != nil {
			w.err = err
			pr.CloseWithError(err)
			return
		}
	}()
	return nil
}

// Close completes the writer and close it.
// Any error occuring during write will be returned.
func (w *Writer) Close() error {
	if w.w == nil {
		w.touch()
	} else if err := w.w.Close(); err != nil {
		return err
	}
	<-w.donec
	return w.err
}

// touch creates an empty object in the bucket if there isn't one already.
// It is called if user creates a new writer but never calls write before
// closing it.
func (w *Writer) touch() {
	defer close(w.donec)
	_, w.err = w.uploader.UploadWithContext(w.ctx, &s3manager.UploadInput{
		Bucket: aws.String(w.bucket),
		Key:    aws.String(w.key),
		Body:   emptyBody,
	})
}

// Bucket represents an S3 bucket and handles read, write and delete operations.
type Bucket struct {
	name   string
	client *s3.S3
	ecfg   *aws.Config
}

// BucketOptions provides information settings during bucket initialization.
type BucketOptions struct {
	AWSConfig *aws.Config
}

// NewRangeReader returns a Reader that reads part of an object, reading at most
// length bytes starting at the given offset. If length is 0, it will read only
// the metadata. If length is negative, it will read till the end of the object.
func (b *Bucket) NewRangeReader(ctx context.Context, key string, offset, length int64) (driver.Reader, error) {
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
	req := b.client.GetObjectRequest(in)
	res, err := req.Send()
	if err != nil {
		return nil, err
	}
	return &Reader{
		body: res.Body,
		size: aws.Int64Value(res.ContentLength),
	}, nil
}

func (b *Bucket) newMetadataReader(ctx context.Context, key string) (driver.Reader, error) {
	in := &s3.HeadObjectInput{
		Bucket: aws.String(b.name),
		Key:    aws.String(key),
	}
	req := b.client.HeadObjectRequest(in)
	res, err := req.Send()
	if err != nil {
		return nil, err
	}
	return &Reader{
		body: emptyBody,
		size: aws.Int64Value(res.ContentLength),
	}, nil
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
	uploader := s3manager.NewUploader(*b.ecfg)
	w := &Writer{
		bucket:   b.name,
		ctx:      ctx,
		key:      key,
		uploader: uploader,
		donec:    make(chan struct{}),
	}
	if opts != nil {
		w.bufferSize = opts.BufferSize
	}
	return w, nil
}

// Delete deletes the object associated with key. It is a no-op if that object
// does not exist.
func (b *Bucket) Delete(ctx context.Context, key string) error {
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(b.name),
		Key:    aws.String(key),
	}

	req := b.client.DeleteObjectRequest(input)
	_, err := req.Send()
	return err
}

const (
	bucketNamingRuleURL = "https://docs.aws.amazon.com/AmazonS3/latest/dev/BucketRestrictions.html"
	objectNamingRuleURL = "https://docs.aws.amazon.com/AmazonS3/latest/dev/UsingMetadata.html"
)

// validateBucketChar checks whether character set and length meet the general
// requirement of bucket naming rule. See
// https://docs.aws.amazon.com/AmazonS3/latest/dev/BucketRestrictions.html
// for the full requirements and best practice.
func validateBucketChar(name string) error {
	v := regexp.MustCompile(`^[a-z0-9][a-z0-9-.]{1,61}[a-z0-9]$`)
	if !v.MatchString(name) {
		return fmt.Errorf("invalid bucket name, see %s for detailed requirements", bucketNamingRuleURL)
	}
	return nil
}

// validateObjectChar checks whether name is a valid UTF-8 encoded string, and its
// length is between 1-1024 bytes. See
// https://docs.aws.amazon.com/AmazonS3/latest/dev/UsingMetadata.html for the full
// requirements and best practice.
func validateObjectChar(name string) error {
	if name == "" {
		return errors.New("object name is empty")
	}
	if !utf8.ValidString(name) {
		return fmt.Errorf("object name is not vlid UTF-8, see %s for detailed requirements", objectNamingRuleURL)
	}
	if len(name) > 1024 {
		return fmt.Errorf("object name is longer than 1024 bytes, see %s for detailed requirements", objectNamingRuleURL)
	}
	return nil
}
