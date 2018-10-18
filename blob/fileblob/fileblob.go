// Copyright 2018 The Go Cloud Authors
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
// Blob names are escaped using url.PathEscape before writing them
// to disk, and unescaped using url.PathUnescape during List.
// Exception: "/" is not escaped, so that it can be used as the real file
// separate on the filesystem.
// Filenames on disk that return an error for url.PathUnescape are not visible
// using fileblob.
//
// It does not support any types for As.
package fileblob

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/driver"
)

const defaultPageSize = 1000

type bucket struct {
	dir string
}

// NewBucket creates a new bucket that reads and writes to dir.
// dir must exist.
func NewBucket(dir string) (*blob.Bucket, error) {
	dir = filepath.Clean(dir)
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("open file bucket: %v", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("open file bucket: %s is not a directory", dir)
	}
	return blob.NewBucket(&bucket{dir}), nil
}

func escape(s string) string {
	s = url.PathEscape(s)
	// Unescape "/".
	return strings.Replace(s, "%2F", "/", -1)
}

func unescape(s string) (string, error) {
	return url.PathUnescape(s)
}

func (b *bucket) forKey(key string) (string, os.FileInfo, *xattrs, error) {
	relpath, err := unescape(key)
	if err != nil {
		return "", nil, nil, fmt.Errorf("open file blob %s: %v", key, err)
	}
	path := filepath.Join(b.dir, relpath)
	if strings.HasSuffix(path, attrsExt) {
		return "", nil, nil, fmt.Errorf("open file blob %s: extension %q cannot be directly read", key, attrsExt)
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, nil, fileError{relpath: relpath, msg: err.Error(), kind: driver.NotFound}
		}
		return "", nil, nil, fmt.Errorf("open file blob %s: %v", key, err)
	}
	xa, err := getAttrs(path)
	if err != nil {
		return "", nil, nil, fmt.Errorf("open file attributes %s: %v", key, err)
	}
	return path, info, &xa, nil
}

// ListPaged implements driver.ListPaged.
func (b *bucket) ListPaged(ctx context.Context, opt *driver.ListOptions) (*driver.ListPage, error) {

	// First, do a full recursive scan of the root directory.
	var result driver.ListPage
	err := filepath.Walk(b.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Couldn't read this file/directory for some reason; just skip it.
			return nil
		}
		// Skip the self-generated attribute files.
		if strings.HasSuffix(path, attrsExt) {
			return nil
		}
		// os.Walk returns the root directory; skip it.
		if path == b.dir {
			return nil
		}
		// Strip the <b.dir> prefix from path; +1 is to include the separator.
		path = path[len(b.dir)+1:]
		// Unescape the path to get the key; if this fails, skip.
		key, err := unescape(path)
		if err != nil {
			return nil
		}
		// Skip all directories. If opt.Delimiter is set, we'll create
		// pseudo-directories later.
		// Note that returning nil means that we'll still recurse into it;
		// we're just not adding a result for the directory itself.
		// We can avoid recursing into subdirectories if the directory name
		// already doesn't match the prefix; any files in it are guaranteed not
		// to match.
		if info.IsDir() {
			key += "/"
			if len(key) > len(opt.Prefix) && !strings.HasPrefix(key, opt.Prefix) {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip files/directories that don't match the Prefix.
		if !strings.HasPrefix(key, opt.Prefix) {
			return nil
		}
		// Add a ListObject for this file.
		result.Objects = append(result.Objects, &driver.ListObject{
			Key:     key,
			ModTime: info.ModTime(),
			Size:    info.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	// TODO(rvangent): It is likely possible (but not trivial) to optimize by
	// doing the collapsing and pagination inline to os.Walk, instead of as
	// post-processing.

	// If using Delimiter, collapse "directories".
	if len(result.Objects) > 0 && opt.Delimiter != "" {
		var collapsedObjects []*driver.ListObject
		var lastPrefix string
		for _, obj := range result.Objects {
			// Strip the prefix, which may contain Delimiter.
			keyWithoutPrefix := obj.Key[len(opt.Prefix):]
			// See if the key still contains Delimiter.
			// If no, it's a file and we just include it.
			// If yes, it's a file in a "sub-directory" and we want to collapse
			// all files in that "sub-directory" into a single "directory" result.
			if idx := strings.Index(keyWithoutPrefix, opt.Delimiter); idx == -1 {
				collapsedObjects = append(collapsedObjects, obj)
			} else {
				prefix := opt.Prefix + keyWithoutPrefix[0:idx+len(opt.Delimiter)]
				if prefix != lastPrefix {
					// First time seeing this "subdirectory"; add it.
					collapsedObjects = append(collapsedObjects, &driver.ListObject{
						Prefix: prefix,
					})
					lastPrefix = prefix
				}
			}
		}
		result.Objects = collapsedObjects
	}

	// If there's a pageToken, skip ahead.
	if len(opt.PageToken) > 0 {
		pageToken := string(opt.PageToken)
		for len(result.Objects) > 0 && result.Objects[0].Name() < pageToken {
			result.Objects = result.Objects[1:]
		}
	}

	// If we've got more than a full page of results, truncate and set
	// NextPageToken.
	pageSize := opt.PageSize
	if pageSize == 0 {
		pageSize = defaultPageSize
	}
	if len(result.Objects) > pageSize {
		result.NextPageToken = []byte(result.Objects[pageSize].Name())
		result.Objects = result.Objects[:pageSize]
	}
	return &result, nil
}

// As implements driver.As.
func (b *bucket) As(i interface{}) bool { return false }

// Attributes implements driver.Attributes.
func (b *bucket) Attributes(ctx context.Context, key string) (driver.Attributes, error) {
	_, info, xa, err := b.forKey(key)
	if err != nil {
		return driver.Attributes{}, err
	}
	return driver.Attributes{
		ContentType: xa.ContentType,
		Metadata:    xa.Metadata,
		ModTime:     info.ModTime(),
		Size:        info.Size(),
	}, nil
}

// NewRangeReader implements driver.NewRangeReader.
func (b *bucket) NewRangeReader(ctx context.Context, key string, offset, length int64) (driver.Reader, error) {
	path, info, xa, err := b.forKey(key)
	if err != nil {
		return nil, err
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
		r: r,
		c: f,
		attrs: driver.ReaderAttributes{
			ContentType: xa.ContentType,
			ModTime:     info.ModTime(),
			Size:        info.Size(),
		},
	}, nil
}

type reader struct {
	r     io.Reader
	c     io.Closer
	attrs driver.ReaderAttributes
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

func (r reader) Attributes() driver.ReaderAttributes {
	return r.attrs
}

func (r reader) As(i interface{}) bool { return false }

// NewTypedWriter implements driver.NewTypedWriter.
func (b *bucket) NewTypedWriter(ctx context.Context, key string, contentType string, opt *driver.WriterOptions) (driver.Writer, error) {
	path := filepath.Join(b.dir, escape(key))
	if strings.HasSuffix(path, attrsExt) {
		return nil, fmt.Errorf("open file blob %s: extension %q is reserved and cannot be used", key, attrsExt)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
		return nil, fmt.Errorf("open file blob %s: %v", key, err)
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("open file blob %s: %v", key, err)
	}
	if opt != nil && opt.BeforeWrite != nil {
		if err := opt.BeforeWrite(func(interface{}) bool { return false }); err != nil {
			return nil, err
		}
	}
	var metadata map[string]string
	if opt != nil && len(opt.Metadata) > 0 {
		metadata = opt.Metadata
	}
	attrs := xattrs{
		ContentType: contentType,
		Metadata:    metadata,
	}
	return &writer{
		ctx:   ctx,
		w:     f,
		path:  path,
		attrs: attrs,
	}, nil
}

type writer struct {
	ctx   context.Context
	w     io.WriteCloser
	path  string
	attrs xattrs
}

func (w writer) Write(p []byte) (n int, err error) {
	return w.w.Write(p)
}

func (w writer) Close() error {
	// If the write was cancelled, delete the file.
	if err := w.ctx.Err(); err != nil {
		_ = os.Remove(w.path)
		return err
	}
	if err := setAttrs(w.path, w.attrs); err != nil {
		return fmt.Errorf("write blob attributes: %v", err)
	}
	return w.w.Close()
}

// Delete implements driver.Delete.
func (b *bucket) Delete(ctx context.Context, key string) error {
	path := filepath.Join(b.dir, escape(key))
	if strings.HasSuffix(path, attrsExt) {
		return fmt.Errorf("delete file blob %s: extension %q cannot be directly deleted", key, attrsExt)
	}
	err := os.Remove(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileError{relpath: key, msg: err.Error(), kind: driver.NotFound}
		}
		return fmt.Errorf("delete file blob %s: %v", key, err)
	}
	if err = os.Remove(path + attrsExt); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete file blob %s: %v", key, err)
	}
	return nil
}

func (b *bucket) SignedURL(ctx context.Context, key string, opts *driver.SignedURLOptions) (string, error) {
	// TODO(Issue #546): Implemented SignedURL for fileblob.
	return "", fileError{msg: "SignedURL not supported (see issue #546)", kind: driver.NotImplemented}
}

type fileError struct {
	relpath, msg string
	kind         driver.ErrorKind
}

func (e fileError) Error() string {
	if e.relpath == "" {
		return fmt.Sprintf("fileblob: %s", e.msg)
	}
	return fmt.Sprintf("fileblob: object %s: %s", e.relpath, e.msg)
}

func (e fileError) Kind() driver.ErrorKind {
	return e.kind
}
