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

// Package fileblob provides a bucket implementation that operates on the local
// filesystem. This should not be used for production: it is intended for local
// development.
//
// BUG: fileblob does not sanitize its inputs, and should not be used with untrusted sources.
package fileblob

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/driver"
)

type bucket struct {
	dir string
}

// NewBucket creates a new bucket that reads and writes to dir.
// dir must exist.
func NewBucket(dir string) (*blob.Bucket, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("open file bucket: %v", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("open file bucket: %s is not a directory", dir)
	}
	return blob.NewBucket(&bucket{dir}), nil
}

func (b *bucket) NewRangeReader(ctx context.Context, key string, offset, length int64) (driver.Reader, error) {
	path := filepath.Join(b.dir, key)
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("open file blob %s: %v", key, err)
	}
	if length == 0 {
		return reader{size: info.Size()}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file blob %s: %v", key, err)
	}
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return nil, fmt.Errorf("open file blob %s: %v", key, err)
		}
	}
	r := io.Reader(f)
	if length > 0 {
		r = io.LimitReader(r, length)
	}
	return reader{
		r:    r,
		c:    f,
		size: info.Size(),
	}, nil
}

type reader struct {
	r    io.Reader
	c    io.Closer
	size int64
}

func (r reader) Read(p []byte) (int, error) {
	if r.r == nil {
		return 0, io.EOF
	}
	return r.r.Read(p)
}

func (r reader) Close() error {
	if r.c == nil {
		return nil
	}
	return r.c.Close()
}

func (r reader) Size() int64 {
	return r.size
}

func (b *bucket) NewWriter(ctx context.Context, key string, opt *driver.WriterOptions) (driver.Writer, error) {
	if d := filepath.Dir(key); d != "." {
		if err := os.MkdirAll(filepath.Join(b.dir, d), 0777); err != nil {
			return nil, fmt.Errorf("open file blob %s: %v", key, err)
		}
	}
	f, err := os.Create(filepath.Join(b.dir, key))
	if err != nil {
		return nil, fmt.Errorf("open file blob %s: %v", key, err)
	}
	return f, nil
}

func (b *bucket) Delete(ctx context.Context, key string) error {
	err := os.Remove(filepath.Join(b.dir, key))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete file blob %s: %v", key, err)
	}
	return nil
}
