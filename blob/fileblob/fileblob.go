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
// Blob names must only contain alphanumeric characters, slashes, periods,
// spaces, underscores, and dashes. Repeated slashes, a leading "./" or "../",
// or the sequence "/./" is not permitted. This is to ensure that blob names map
// cleanly onto files underneath a directory.
package fileblob

import (
	"context"
	"fmt"
	"io"
	"os"
	slashpath "path"
	"path/filepath"
	"strings"

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

// resolvePath converts a key into a relative filesystem path. It guarantees
// that there will only be one valid key for a given path and that the resulting
// path will not reach outside the directory.
func resolvePath(key string) (string, error) {
	for _, c := range key {
		if !('A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '/' || c == '.' || c == ' ' || c == '_' || c == '-') {
			return "", fmt.Errorf("contains invalid character %q", c)
		}
	}
	if cleaned := slashpath.Clean(key); key != cleaned {
		return "", fmt.Errorf("not a clean slash-separated path")
	}
	if slashpath.IsAbs(key) {
		return "", fmt.Errorf("starts with a slash")
	}
	if key == "." {
		return "", fmt.Errorf("invalid path \".\"")
	}
	if strings.HasPrefix(key, "../") {
		return "", fmt.Errorf("starts with \"../\"")
	}
	return filepath.FromSlash(key), nil
}

func (b *bucket) NewRangeReader(ctx context.Context, key string, offset, length int64) (driver.Reader, error) {
	relpath, err := resolvePath(key)
	if err != nil {
		return nil, fmt.Errorf("open file blob %s: %v", key, err)
	}
	path := filepath.Join(b.dir, relpath)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errObjectNotExist{path: path, msg: err.Error()}
		}
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
	relpath, err := resolvePath(key)
	if err != nil {
		return nil, fmt.Errorf("open file blob %s: %v", key, err)
	}
	path := filepath.Join(b.dir, relpath)
	if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
		return nil, fmt.Errorf("open file blob %s: %v", key, err)
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("open file blob %s: %v", key, err)
	}
	return f, nil
}

func (b *bucket) Delete(ctx context.Context, key string) error {
	relpath, err := resolvePath(key)
	if err != nil {
		return fmt.Errorf("delete file blob %s: %v", key, err)
	}
	path := filepath.Join(b.dir, relpath)
	err = os.Remove(path)
	if err != nil {
		if os.IsNotExist(err) {
			return errObjectNotExist{path: path, msg: err.Error()}
		}
		return fmt.Errorf("delete file blob %s: %v", key, err)
	}
	return nil
}

type errObjectNotExist struct {
	path string
	msg  string
}

func (e errObjectNotExist) WrapNotExist() error {
	return fmt.Errorf("fileblob: object %s doesn't exist: %v", e.path, e.msg)
}

func (e errObjectNotExist) Error() string {
	return e.WrapNotExist().Error()
}
