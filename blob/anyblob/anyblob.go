// Copyright 2019 The Go Cloud Authors
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

// Package anyblob provides an Open function that opens buckets backed
// by GCS, S3, Azure Storage, filesystem, and memory.
package anyblob

import (
	"context"

	"gocloud.dev/blob"
	"gocloud.dev/blob/azureblob"
	"gocloud.dev/blob/fileblob"
	"gocloud.dev/blob/gcsblob"
	"gocloud.dev/blob/memblob"
	"gocloud.dev/blob/s3blob"
)

// Open opens a bucket URL, allowing URL overrides for all openers that
// support it. See the URLOpener types in various provider packages for
// more details.
func Open(ctx context.Context, urlstr string) (*blob.Bucket, error) {
	return mux.Open(ctx, urlstr)
}

var mux = blob.NewURLMux(map[string]blob.BucketURLOpener{
	azureblob.Scheme: &azureblob.URLOpener{AllowURLOverrides: true},
	fileblob.Scheme:  &fileblob.URLOpener{},
	gcsblob.Scheme:   &gcsblob.URLOpener{AllowURLOverrides: true},
	memblob.Scheme:   &memblob.URLOpener{},
	s3blob.Scheme:    &s3blob.URLOpener{AllowURLOverrides: true},
})
