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

// +build gcp

package io

import (
	"context"
	"sync"

	"cloud.google.com/go/storage"
	cloud "github.com/vsekhar/go-cloud"
)

const gcpServiceName = "gs"

func init() {
	RegisterBlobProvider(gcpServiceName, openGCSBlob)
}

var gcpStorageClient *storage.Client
var gcpStorageClientOnce sync.Once

func openGCSBlob(ctx context.Context, uri string) (Blob, error) {
	// Create client on first use so that we don't have dangling connections
	// if this provider is never used.
	gcpStorageClientOnce.Do(func() {
		var err error
		gcpStorageClient, err = storage.NewClient(ctx)
		if err != nil {
			panic(err)
		}
	})

	service, path, err := cloud.Service(uri)
	if err != nil {
		return nil, err
	}
	if service != gcpServiceName {
		panic("bad handler routing: " + uri)
	}
	bucket, path := cloud.PopPath(path)
	bkt := gcpStorageClient.Bucket(bucket)
	obj := bkt.Object(path)
	return &gcsBlob{obj: obj}, nil
}

// implements io.Blob
type gcsBlob struct {
	obj *storage.ObjectHandle
}

func (b *gcsBlob) NewReader(ctx context.Context) (BlobReader, error) {
	return b.obj.NewReader(ctx)
}

func (b *gcsBlob) NewRangeReader(ctx context.Context, offset, length int64) (BlobReader, error) {
	return b.obj.NewRangeReader(ctx, offset, length)
}

func (b *gcsBlob) NewWriter(ctx context.Context) (BlobWriter, error) {
	return b.obj.NewWriter(ctx), nil
}
