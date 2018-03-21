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

package io

import (
	"context"
	baseio "io"
	"os"

	cloud "github.com/vsekhar/go-cloud"
)

const localServiceName = "local"

func init() {
	RegisterBlobProvider(localServiceName, openFileBlob)
}

// fileBlob makes a *os.File suitable for returning as a Blob.
type fileBlob struct {
	path string
}

func openFileBlob(_ context.Context, path string) (Blob, error) {
	service, path, err := cloud.Service(path)
	if err != nil {
		return nil, err
	}
	if service != localServiceName {
		panic("bad handler routing: " + localServiceName)
	}
	return &fileBlob{path: path}, nil
}

func (fb *fileBlob) NewReader(ctx context.Context) (BlobReader, error) {
	return fb.NewRangeReader(ctx, 0, -1)
}

func (fb *fileBlob) NewRangeReader(_ context.Context, offset, length int64) (BlobReader, error) {
	f, err := os.Open(fb.path)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if _, err := f.Seek(offset, os.SEEK_SET); err != nil {
		return nil, err
	}
	fbr := new(fileBlobReader)
	fbr.file = f
	fbr.r = f
	if length >= 0 {
		fbr.r = baseio.LimitReader(f, length)
	}
	fbr.size = fi.Size()
	if length >= 0 && length < fbr.size {
		fbr.size = length
	}
	return fbr, nil
}

func (fb *fileBlob) NewWriter(_ context.Context) (BlobWriter, error) {
	f, err := os.OpenFile(fb.path, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	if err := f.Truncate(0); err != nil {
		return nil, err
	}
	if _, err := f.Seek(0, os.SEEK_SET); err != nil {
		return nil, err
	}
	return f, nil
}

type fileBlobReader struct {
	file *os.File
	r    baseio.Reader // limited
	size int64
}

func (fbr *fileBlobReader) Read(d []byte) (int, error) {
	return fbr.r.Read(d)
}

func (fbr *fileBlobReader) Close() error {
	return fbr.file.Close()
}

func (fbr *fileBlobReader) Size() int64 {
	return fbr.size
}
