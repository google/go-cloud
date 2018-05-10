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

package s3blob_test

import (
	"bytes"
	"context"
	"io"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/s3blob"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/google/go-cmp/cmp"
)

const (
	testBucket       = "oh.test.psp"
	testBucketRegion = "us-east-2"
)

var (
	s3Bucket *blob.Bucket
	s3Client *s3.S3
	uploader *s3manager.Uploader
)

func TestMain(m *testing.M) {
	if testing.Short() {
		return
	}
	ctx := context.Background()
	var err error
	ecfg := &aws.Config{Region: aws.String(testBucketRegion)}
	sess := session.Must(session.NewSession(ecfg))
	if s3Bucket, err = s3blob.NewBucket(ctx, sess, testBucket); err != nil {
		log.Fatalf("error initializing S3 bucket: %v", err)
	}

	// Setup for using AWS SDK directly for verification.
	s3Client = s3.New(sess)
	uploader = s3manager.NewUploader(sess)
	os.Exit(m.Run())
}

func TestNewBucketNaming(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping tests requiring network")
	}
	t.Parallel()
	tests := []struct {
		name  string
		valid bool
	}{
		{testBucket, true},
		{"8ucket-nam3", true},
		{"bn", false},
		{"_bucketname_", false},
		{"bucketnameUpper", false},
		{"bucketname?invalidchar", false},
		{strings.Repeat("a", 64), false},
	}

	ctx := context.Background()
	sess := session.Must(session.NewSession(nil))
	for i, test := range tests {
		_, err := s3blob.NewBucket(ctx, sess, test.name)
		if test.valid && err != nil {
			t.Errorf("%d) got %v, want nil", i, err)
		} else if !test.valid && err == nil {
			t.Errorf("%d) got nil, want invalid error", i)
		}
	}
}

func TestNewWriterObjectNaming(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping tests requiring network")
	}
	t.Parallel()
	tests := []struct {
		name  string
		valid bool
	}{
		{"object-name", true},
		{"文件名", true},
		{"ファイル名", true},
		{"", false},
		{"\xF4\x90\x80\x80", false},
		{strings.Repeat("a", 1024), true},
		{strings.Repeat("a", 1025), false},
		{strings.Repeat("☺", 342), false},
	}

	ctx := context.Background()
	for i, test := range tests {
		_, err := s3Bucket.NewWriter(ctx, test.name, nil)
		if test.valid && err != nil {
			t.Errorf("%d) got %v, want nil", i, err)
		} else if !test.valid && err == nil {
			t.Errorf("%d) got nil, want invalid error", i)
		}
	}
}

func TestRead(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping tests requiring network")
	}
	t.Parallel()
	object := "test_read"
	content := []byte("something worth reading")
	fullen := int64(len(content))
	if _, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(testBucket),
		Key:    aws.String(object),
		Body:   bytes.NewReader(content),
	}); err != nil {
		t.Fatalf("error uploading test object: %v", err)
	}

	tests := []struct {
		name           string
		offset, length int64
		want           []byte
		got            []byte
		wantSize       int64
		wantError      bool
	}{
		{
			name:      "negative offset",
			offset:    -1,
			wantError: true,
		},
		{
			name:     "read metadata",
			length:   0,
			want:     make([]byte, 0),
			got:      make([]byte, 0),
			wantSize: fullen,
		},
		{
			name:     "read from positive offset to end",
			offset:   10,
			length:   -1,
			want:     content[10:],
			got:      make([]byte, fullen-10),
			wantSize: fullen - 10,
		},
		{
			name:     "read a part in middle",
			offset:   10,
			length:   5,
			want:     content[10:15],
			got:      make([]byte, 5),
			wantSize: 5,
		},
		{
			name:     "read in full",
			offset:   0,
			length:   -1,
			want:     content,
			got:      make([]byte, fullen),
			wantSize: fullen,
		},
	}

	for i, test := range tests {
		r, err := s3Bucket.NewRangeReader(context.Background(), object, test.offset, test.length)
		if test.wantError {
			if err == nil {
				t.Errorf("%d) want error got nil", i)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%d) cannot create new reader: %v", i, err)

		}
		if _, err := r.Read(test.got); err != nil && err != io.EOF {
			t.Fatalf("%d) error during read: %v", i, err)
		}
		if !cmp.Equal(test.got, test.want) || r.Size() != test.wantSize {
			t.Errorf("%d) got %s of size %d, want %s of size %d", i, test.got, r.Size(), test.want, test.wantSize)
		}
		r.Close()
	}
}

func TestWrite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping tests requiring network")
	}
	t.Parallel()
	ctx := context.Background()
	object := "test_write"

	tests := []struct {
		name       string
		parts      [][]byte
		want       []byte
		got        []byte
		wantSize   int
		closeLater bool
	}{
		{
			name:       "write in multiple parts",
			parts:      [][]byte{[]byte("HELLO!"), []byte("hello!")},
			want:       []byte("HELLO!hello!"),
			got:        make([]byte, 12),
			wantSize:   12,
			closeLater: false,
		},
		{
			name:       "read before writer closes",
			parts:      [][]byte{[]byte("!")},
			want:       []byte("HELLO!hello!"),
			got:        make([]byte, 12),
			wantSize:   12,
			closeLater: true,
		},
	}

	for i, test := range tests {
		w, err := s3Bucket.NewWriter(ctx, object, nil)
		if err != nil {
			t.Errorf("%d) error creating writer: %v", i, err)
		}
		var written int64 = 0
		for _, p := range test.parts {
			n, err := w.Write(p)
			if n != len(p) || err != nil {
				t.Errorf("%d) writing object: %d written, got error %v", i, n, err)
			}
			written += int64(n)
		}
		if !test.closeLater {
			if err := w.Close(); err != nil {
				t.Errorf("%d) error closing writer: %v", i, err)
			}
		}
		req, resp := s3Client.GetObjectRequest(&s3.GetObjectInput{
			Bucket: aws.String(testBucket),
			Key:    aws.String(object),
		})
		if err := req.Send(); err != nil {
			t.Fatalf("%d) error getting object: %v", i, err)
		}
		body := resp.Body
		n, err := body.Read(test.got)
		if err != nil && err != io.EOF {
			t.Errorf("%d) reading object: %d read, got error %v", i, n, err)
		}
		body.Close()
		if test.closeLater {
			if err := w.Close(); err != nil {
				t.Fatalf("%d) error closing writer: %v", i, err)
			}
		}
		if !cmp.Equal(test.got, test.want) || n != test.wantSize {
			t.Errorf("%d) got %s, size %d, want %s, size %d", i, test.got, n, test.want, test.wantSize)
		}
	}

	if err := s3Bucket.Delete(ctx, object); err != nil {
		t.Errorf("error deleting object: %v", err)
	}
}

func TestCloseWithoutWrite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping tests requiring network")
	}
	t.Parallel()
	ctx := context.Background()
	object := "test_close_without_write"

	w, err := s3Bucket.NewWriter(ctx, object, nil)
	if err != nil {
		t.Errorf("error creating new writer: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("error closing writer without writing: %v", err)
	}

	req, resp := s3Client.HeadObjectRequest(&s3.HeadObjectInput{
		Bucket: aws.String(testBucket),
		Key:    aws.String(object),
	})
	err = req.Send()
	size := aws.Int64Value(resp.ContentLength)
	if err != nil || size != 0 {
		t.Errorf("want 0 bytes written, got %d bytes written, error %v", size, err)
	}

	if err := s3Bucket.Delete(ctx, object); err != nil {
		t.Errorf("error deleting object: %v", err)
	}
}

func TestDelete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping tests requiring network")
	}
	t.Parallel()
	ctx := context.Background()
	object := "test_delete"
	content := []byte("something obsolete")
	if _, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(testBucket),
		Key:    aws.String(object),
		Body:   bytes.NewReader(content),
	}); err != nil {
		t.Fatalf("error uploading test object: %v", err)
	}

	if err := s3Bucket.Delete(ctx, object); err != nil {
		t.Errorf("error occurs when deleting a non-existing object: %v", err)
	}
	req, _ := s3Client.HeadObjectRequest(&s3.HeadObjectInput{
		Bucket: aws.String(testBucket),
		Key:    aws.String(object),
	})
	if err := req.Send(); err == nil {
		t.Errorf("object deleted, got err %v, want NotFound error", err)
	}

	// Delete non-existing object, no-op
	if err := s3Bucket.Delete(ctx, object); err != nil {
		t.Errorf("error occurs when deleting a non-existing object: %v", err)
	}

}
