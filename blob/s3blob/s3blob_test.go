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
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/google/go-cloud/blob/s3blob"
	"github.com/google/go-cloud/testing/setup"
	"github.com/google/go-cmp/cmp"
)

const region = "us-east-2"

//func TestMain(m *testing.M) {
//sess, err := setup.NewAWSSession(t, region, "testmain")
//if s3Bucket, err = s3blob.NewBucket(ctx, sess, testBucket); err != nil {
//log.Fatalf("error initializing S3 bucket: %v", err)
//}

//// Setup for using AWS SDK directly for verification.
//s3Client = s3.New(sess)
//uploader = s3manager.NewUploader(sess)
//code := m.Run()
//recDone()
//os.Exit(code)
//}

// TestNewBucketNaming tests if buckets can be created with incorrect characters.
// Note that this function doesn't hit AWS, so does not require the recorder.
func TestNewBucketNaming(t *testing.T) {
	tests := []struct {
		name, bucketName string
		wantErr          bool
	}{
		{
			name:       "A good bucket name should pass",
			bucketName: "good-bucket",
		},
		{
			name:       "A name with leading digits should pass",
			bucketName: "8ucket-nam3",
		},
		{
			name:       "A name with leading underscores should fail",
			bucketName: "_bucketname_",
			wantErr:    true,
		},
		{
			name:       "A name with upper case letters should fail",
			bucketName: "bucketnameUpper",
			wantErr:    true,
		},
		{
			name:       "A name with an invalid character should fail",
			bucketName: "bucketname?invalidchar",
			wantErr:    true,
		},
		{
			name:       "A name that's too long should fail",
			bucketName: strings.Repeat("a", 64),
			wantErr:    true,
		},
	}

	sess, done := setup.NewAWSSession(t, region, "test-naming")
	defer done()
	svc := s3.New(sess)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bkt := fmt.Sprintf("go-x-cloud.%s", tc.bucketName)
			_ = forceDeleteBucket(svc, bkt)
			_, err := svc.CreateBucket(&s3.CreateBucketInput{
				Bucket: &bkt,
				CreateBucketConfiguration: &s3.CreateBucketConfiguration{LocationConstraint: aws.String(region)},
			})

			switch {
			case err != nil && !tc.wantErr:
				t.Errorf("got %q; want nil", err)
			case err == nil && tc.wantErr:
				t.Errorf("got nil error; want error")
			}
		})
	}
}

func TestNewWriterObjectNaming(t *testing.T) {
	tests := []struct {
		name, objName string
		wantErr       bool
	}{
		{
			name:    "An ASCII name should pass",
			objName: "object-name",
		},
		{
			name:    "A Unicode name should pass",
			objName: "文件名",
		},

		{
			name:    "An empty name should fail",
			wantErr: true,
		},
		{
			name:    "A name of escaped chars should fail",
			objName: "\xF4\x90\x80\x80",
			wantErr: true,
		},
		{
			name:    "A name of 1024 chars should succeed",
			objName: strings.Repeat("a", 1024),
		},
		{
			name:    "A name of 1025 chars should fail",
			objName: strings.Repeat("a", 1025),
			wantErr: true,
		},
		{
			name:    "A long name of Unicode chars should fail",
			objName: strings.Repeat("☺", 342),
			wantErr: true,
		},
	}

	sess, done := setup.NewAWSSession(t, region, "test-obj-naming")
	defer done()
	svc := s3.New(sess)

	bkt := fmt.Sprintf("go-x-cloud.%s", "test-obj-naming")
	_ = forceDeleteBucket(svc, bkt)
	_, err := svc.CreateBucket(&s3.CreateBucketInput{
		Bucket: &bkt,
		CreateBucketConfiguration: &s3.CreateBucketConfiguration{LocationConstraint: aws.String(region)},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := s3blob.NewBucket(ctx, sess, bkt)
			if err != nil {
				t.Fatal(err)
			}

			w, err := b.NewWriter(ctx, tc.objName, nil)
			if err != nil {
				t.Fatal(err)
			}

			_, err = io.WriteString(w, "foo")
			if err != nil {
				t.Fatal(err)
			}
			err = w.Close()
			switch {
			case err != nil && !tc.wantErr:
				t.Errorf("got %q; want nil", err)
			case err == nil && tc.wantErr:
				t.Errorf("got nil; want error")
			}
		})
	}
}

func TestRead(t *testing.T) {
	content := []byte("something worth reading")
	contentSize := int64(len(content))

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
			wantSize: contentSize,
		},
		{
			name:     "read from positive offset to end",
			offset:   10,
			length:   -1,
			want:     content[10:],
			got:      make([]byte, contentSize-10),
			wantSize: contentSize - 10,
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
			got:      make([]byte, contentSize),
			wantSize: contentSize,
		},
	}

	sess, done := setup.NewAWSSession(t, region, "test-read")
	defer done()
	svc := s3.New(sess)

	bkt := fmt.Sprintf("go-x-cloud.%s", "test-read")
	_ = forceDeleteBucket(svc, bkt)
	_, err := svc.CreateBucket(&s3.CreateBucketInput{
		Bucket: &bkt,
		CreateBucketConfiguration: &s3.CreateBucketConfiguration{LocationConstraint: aws.String(region)},
	})
	if err != nil {
		t.Fatal(err)
	}

	obj := "test_read"
	uploader := s3manager.NewUploader(sess)
	if _, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bkt),
		Key:    aws.String(obj),
		Body:   bytes.NewReader(content),
	}); err != nil {
		t.Fatalf("error uploading test object: %v", err)
	}

	ctx := context.Background()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := s3blob.NewBucket(ctx, sess, bkt)
			if err != nil {
				t.Fatal(err)
			}
			r, err := b.NewRangeReader(context.Background(), obj, tc.offset, tc.length)
			switch {
			case err != nil && !tc.wantError:
				t.Fatalf("cannot create new reader: %v", err)
			case err == nil && tc.wantError:
				t.Fatal("got nil error; want error")
			case tc.wantError:
				return
			}

			if _, err := r.Read(tc.got); err != nil && err != io.EOF {
				t.Fatalf("error during read: %v", err)
			}
			if !cmp.Equal(tc.got, tc.want) || r.Size() != tc.wantSize {
				t.Errorf("got %s of size %d; want %s of size %d", tc.got, r.Size(), tc.want, tc.wantSize)
			}
			r.Close()
		})
	}
}

func TestWrite(t *testing.T) {
	sess, done := setup.NewAWSSession(t, region, "test-write")
	defer done()
	svc := s3.New(sess)

	bkt := fmt.Sprintf("go-x-cloud.%s", "test-write")
	_ = forceDeleteBucket(svc, bkt)
	_, err := svc.CreateBucket(&s3.CreateBucketInput{
		Bucket: &bkt,
		CreateBucketConfiguration: &s3.CreateBucketConfiguration{LocationConstraint: aws.String(region)},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	b, err := s3blob.NewBucket(ctx, sess, bkt)
	if err != nil {
		t.Fatal(err)
	}

	obj := "test_write"
	w, err := b.NewWriter(ctx, obj, nil)
	if err != nil {
		t.Errorf("error creating writer: %v", err)
	}

	var written int64 = 0
	for _, p := range [][]byte{[]byte("HELLO!"), []byte("hello!")} {
		n, err := w.Write(p)
		if n != len(p) || err != nil {
			t.Errorf("writing obj: %d written, got error %v", n, err)
		}
		written += int64(n)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("error closing writer: %v", err)
	}
	req, resp := svc.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(bkt),
		Key:    aws.String(obj),
	})
	if err := req.Send(); err != nil {
		t.Fatalf("error getting object: %v", err)
	}
	body := resp.Body
	got := make([]byte, 12)
	n, err := body.Read(got)
	if err != nil && err != io.EOF {
		t.Errorf("reading object: %d read, got error %v", n, err)
	}
	defer body.Close()
	want := []byte("HELLO!hello!")
	if !cmp.Equal(got, want) || n != 12 {
		t.Errorf("got %s, size %d; want %s, size %d", got, n, want, 12)
	}
}

//func TestCloseWithoutWrite(t *testing.T) {
//ctx := context.Background()
//object := "test_close_without_write"

//w, err := s3Bucket.NewWriter(ctx, object, nil)
//if err != nil {
//t.Errorf("error creating new writer: %v", err)
//}
//if err := w.Close(); err != nil {
//t.Errorf("error closing writer without writing: %v", err)
//}

//req, resp := s3Client.HeadObjectRequest(&s3.HeadObjectInput{
//Bucket: aws.String(testBucket),
//Key:    aws.String(object),
//})
//err = req.Send()
//size := aws.Int64Value(resp.ContentLength)
//if err != nil || size != 0 {
//t.Errorf("want 0 bytes written, got %d bytes written, error %v", size, err)
//}

//if err := s3Bucket.Delete(ctx, object); err != nil {
//t.Errorf("error deleting object: %v", err)
//}
//}

func TestDelete(t *testing.T) {
	sess, done := setup.NewAWSSession(t, region, "test-delete")
	defer done()
	svc := s3.New(sess)

	bkt := fmt.Sprintf("go-x-cloud.%s", "test-delete")
	_ = forceDeleteBucket(svc, bkt)
	_, err := svc.CreateBucket(&s3.CreateBucketInput{
		Bucket: &bkt,
		CreateBucketConfiguration: &s3.CreateBucketConfiguration{LocationConstraint: aws.String(region)},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	obj := "test_delete"
	uploader := s3manager.NewUploader(sess)
	if _, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bkt),
		Key:    aws.String(obj),
		Body:   bytes.NewReader([]byte("something obsolete")),
	}); err != nil {
		t.Fatalf("error uploading test object: %v", err)
	}

	b, err := s3blob.NewBucket(ctx, sess, bkt)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Delete(ctx, obj); err != nil {
		t.Errorf("error occurs when deleting a non-existing object: %v", err)
	}
	req, _ := svc.HeadObjectRequest(&s3.HeadObjectInput{
		Bucket: aws.String(bkt),
		Key:    aws.String(obj),
	})
	if err := req.Send(); err == nil {
		t.Errorf("object deleted, got err %v, want NotFound error", err)
	}

	// Delete non-existing object, no-op
	if err := b.Delete(ctx, obj); err != nil {
		t.Errorf("error occurs when deleting a non-existing object: %v", err)
	}
}

func forceDeleteBucket(svc *s3.S3, bucket string) error {
	resp, err := svc.ListObjects(&s3.ListObjectsInput{Bucket: &bucket})
	if err != nil {
		return err
	}

	var objs []*s3.ObjectIdentifier
	for _, o := range resp.Contents {
		objs = append(objs, &s3.ObjectIdentifier{Key: aws.String(*o.Key)})
	}

	var items s3.Delete
	items.SetObjects(objs)

	_, err = svc.DeleteObjects(&s3.DeleteObjectsInput{Bucket: &bucket, Delete: &items})
	if err != nil {
		return err
	}

	_, err = svc.DeleteBucket(&s3.DeleteBucketInput{Bucket: &bucket})
	if err != nil {
		return err
	}

	return nil
}
