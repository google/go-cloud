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

package gcsblob

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/dnaeon/go-vcr/recorder"
	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/internal/testing/replay"
	"github.com/google/go-cloud/internal/testing/setup"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

const bucketPrefix = "go-cloud"

var projectID = flag.String("project", "", "GCP project ID (string, not project number) to run tests against")

func TestNewBucketNaming(t *testing.T) {
	tests := []struct {
		name, bucketName string
		wantErr          bool
	}{
		{
			name:       "A good bucket name should pass",
			bucketName: "bucket-name",
		},
		{
			name:       "A name with leading digits should pass",
			bucketName: "8ucket_nam3",
		},
		{
			name:       "A name with a leading underscore should fail",
			bucketName: "_bucketname_",
			wantErr:    true,
		},
		{
			name:       "A name with an uppercase character should fail",
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

	ctx := context.Background()
	gcsC, done, err := newGCSClient(ctx, t.Logf, "test-naming")
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	c, err := storage.NewClient(ctx, option.WithHTTPClient(&gcsC.Client))
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := c.Bucket(fmt.Sprintf("%s-%s", bucketPrefix, tc.bucketName))
			err = b.Create(ctx, *projectID, nil)

			switch {
			case err != nil && !tc.wantErr:
				t.Errorf("got %q; want nil", err)
			case err == nil && tc.wantErr:
				t.Errorf("got nil error; want error")
			case !tc.wantErr:
				_ = b.Delete(ctx)
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

	ctx := context.Background()
	gcsC, done, err := newGCSClient(ctx, t.Logf, "test-obj-naming")
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	c, err := storage.NewClient(ctx, option.WithHTTPClient(&gcsC.Client))
	if err != nil {
		t.Fatal(err)
	}
	bkt := fmt.Sprintf("%s-%s", bucketPrefix, "test-obj-naming")
	b := c.Bucket(bkt)
	defer func() { _ = b.Delete(ctx) }()
	_ = b.Create(ctx, *projectID, nil)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := OpenBucket(ctx, bkt, gcsC)
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
				t.Errorf("got nil error; want error")
			}
		})
	}
}

func TestBufferSize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		size int
		want int
	}{
		{
			size: 5 * 1024 * 1024,
			want: 5 * 1024 * 1024,
		},
		{
			size: 0,
			want: googleapi.DefaultUploadChunkSize,
		},
		{
			size: -1024,
			want: 0,
		},
	}
	for i, test := range tests {
		got := bufferSize(test.size)
		if got != test.want {
			t.Errorf("%d) got buffer size %d, want %d", i, got, test.want)
		}
	}
}

type transportSpy struct {
	called bool
}

func (ts *transportSpy) RoundTrip(*http.Request) (*http.Response, error) {
	ts.called = true
	return nil, fmt.Errorf("this is a spy")
}

func TestHTTPClientOpt(t *testing.T) {
	ctx := context.Background()

	ts := &transportSpy{}
	b, err := OpenBucket(ctx, "black-bucket", &gcp.HTTPClient{Client: http.Client{Transport: ts}})
	if err != nil {
		t.Fatal(err)
	}

	w, err := b.NewWriter(ctx, "green-blob", nil)
	if err != nil {
		t.Fatal(err)
	}
	w.Close()

	if !ts.called {
		t.Errorf("got %v; want %v", ts.called, "true")
	}
}

func newGCSClient(ctx context.Context, logf func(string, ...interface{}), filepath string) (*gcp.HTTPClient, func(), error) {

	mode := recorder.ModeRecording
	if !*setup.Record {
		mode = recorder.ModeReplaying
	}
	r, done, err := replay.NewGCSRecorder(logf, mode, filepath)
	if err != nil {
		return nil, nil, err
	}

	c := &gcp.HTTPClient{Client: http.Client{Transport: r}}
	if mode == recorder.ModeRecording {
		creds, err := gcp.DefaultCredentials(ctx)
		if err != nil {
			return nil, nil, err
		}
		c, err = gcp.NewHTTPClient(r, gcp.CredentialsTokenSource(creds))
		if err != nil {
			return nil, nil, err
		}
	}

	return c, done, err
}
