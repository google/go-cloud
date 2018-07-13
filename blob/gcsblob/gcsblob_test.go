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
	"net/http"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/dnaeon/go-vcr/recorder"
	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/internal/testing/replay"
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
			bkt := c.Bucket(fmt.Sprintf("%s-%s", bucketPrefix, tc.bucketName))
			err = bkt.Create(ctx, *projectID, nil)

			switch {
			case err != nil && !tc.wantErr:
				t.Errorf("got %q; want nil", err)
			case err == nil && tc.wantErr:
				t.Errorf("got nil error; want error")
			case !tc.wantErr:
				_ = bkt.Delete(ctx)
			}
		})
	}
}

//func TestValidateObjectChar(t *testing.T) {
//t.Parallel()
//tests := []struct {
//name  string
//valid bool
//}{
//{"object-name", true},
//{"文件名", true},
//{"ファイル名", true},
//{"", false},
//{"\xF4\x90\x80\x80", false},
//{strings.Repeat("a", 1024), true},
//{strings.Repeat("a", 1025), false},
//{strings.Repeat("☺", 342), false},
//}

//for i, test := range tests {
//err := validateObjectChar(test.name)
//if test.valid && err != nil {
//t.Errorf("%d) got %v, want nil", i, err)
//} else if !test.valid && err == nil {
//t.Errorf("%d) got nil, want invalid error", i)
//}
//}
//}

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
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		return nil, nil, err
	}

	mode := recorder.ModeRecording
	if testing.Short() {
		mode = recorder.ModeReplaying
	}
	r, done, err := replay.NewGCSRecorder(logf, mode, filepath)
	if err != nil {
		return nil, nil, err
	}
	c, err := gcp.NewHTTPClient(r, gcp.CredentialsTokenSource(creds))
	if err != nil {
		return nil, nil, err
	}

	return c, done, err
}
