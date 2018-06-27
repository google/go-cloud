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
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-x-cloud/gcp"

	"google.golang.org/api/googleapi"
)

func TestValidateBucketChar(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		valid bool
	}{
		{"bucket-name", true},
		{"8ucket_nam3", true},
		{"bn", false},
		{"_bucketname_", false},
		{"bucketnameUpper", false},
		{"bucketname?invalidchar", false},
	}

	for i, test := range tests {
		err := validateBucketChar(test.name)
		if test.valid && err != nil {
			t.Errorf("%d) got %v, want nil", i, err)
		} else if !test.valid && err == nil {
			t.Errorf("%d) got nil, want invalid error", i)
		}
	}
}

func TestValidateObjectChar(t *testing.T) {
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

	for i, test := range tests {
		err := validateObjectChar(test.name)
		if test.valid && err != nil {
			t.Errorf("%d) got %v, want nil", i, err)
		} else if !test.valid && err == nil {
			t.Errorf("%d) got nil, want invalid error", i)
		}
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
	b, err := NewBucket(ctx, "black-bucket", &gcp.HTTPClient{Client: http.Client{Transport: ts}})
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
