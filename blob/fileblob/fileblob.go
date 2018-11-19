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
// Blob keys are escaped before being used as filenames, and filenames are
// unescaped when they are passed back as blob keys during List. The escape
// algorithm is:
// -- Alphanumeric characters (A-Z a-z 0-9) are not escaped.
// -- Space (' '), dash ('-'), underscore ('_'), and period ('.') are not escaped.
// -- Slash ('/') is always escaped to the OS-specific path separator character
//    (os.PathSeparator).
// -- All other characters are escaped similar to url.PathEscape:
//    "%<hex UTF-8 byte>", with capital letters ABCDEF in the hex code.
//
// Filenames that can't be unescaped due to invalid escape sequences
// (e.g., "%%"), or whose unescaped key doesn't escape back to the filename
// (e.g., "~", which unescapes to "~", which escapes back to "%7E" != "~"),
// aren't visible using fileblob.
//
// For blob.Open URLs, fileblob registers for the "file" scheme.
// The URL's Path is used as the root directory; the URL's Host is ignored.
// If os.PathSeparator != "/", any leading "/" from the Path is dropped.
// No query options are supported. Examples:
// -- file:///a/directory passes "/a/directory" to OpenBucket.
// -- file://localhost/a/directory also passes "/a/directory".
// -- file:///c:/foo/bar passes "c:/foo/bar".
//
// fileblob does not support any types for As.
package fileblob

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/driver"
)

const defaultPageSize = 1000

func init() {
	blob.Register("file", func(_ context.Context, u *url.URL) (driver.Bucket, error) {
		path := u.Path
		if os.PathSeparator != '/' && strings.HasPrefix(path, "/") {
			path = path[1:]
		}
		return openBucket(path, nil)
	})
}

// Options sets options for constructing a *blob.Bucket backed by fileblob.
type Options struct{}

type bucket struct {
	dir string
}

// openBucket creates a driver.Bucket that reads and writes to dir.
// dir must exist.
func openBucket(dir string, _ *Options) (driver.Bucket, error) {
	dir = filepath.Clean(dir)
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}
	return &bucket{dir}, nil
}

// OpenBucket creates a *blob.Bucket that reads and writes to dir.
// dir must exist.
func OpenBucket(dir string, opts *Options) (*blob.Bucket, error) {
	drv, err := openBucket(dir, opts)
	if err != nil {
		return nil, err
	}
	return blob.NewBucket(drv), nil
}

// shouldEscape returns true if c should be escaped.
func shouldEscape(c byte) bool {
	switch {
	case 'A' <= c && c <= 'Z':
		return false
	case 'a' <= c && c <= 'z':
		return false
	case '0' <= c && c <= '9':
		return false
	case c == ' ' || c == '-' || c == '_' || c == '.':
		return false
	case c == '/':
		return false
	}
	return true
}

// escape returns s escaped per the rules described in the package docstring.
// The code is modified from https://golang.org/src/net/url/url.go.
func escape(s string) string {
	hexCount := 0
	replaceSlash := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if shouldEscape(c) {
			hexCount++
		} else if c == '/' && os.PathSeparator != '/' {
			replaceSlash = true
		}
	}
	if hexCount == 0 && !replaceSlash {
		return s
	}
	t := make([]byte, len(s)+2*hexCount)
	j := 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '/':
			t[j] = os.PathSeparator
			j++
		case shouldEscape(c):
			t[j] = '%'
			t[j+1] = "0123456789ABCDEF"[c>>4]
			t[j+2] = "0123456789ABCDEF"[c&15]
			j += 3
		default:
			t[j] = s[i]
			j++
		}
	}
	return string(t)
}

// ishex returns true if c is a valid part of a hexadecimal number.
func ishex(c byte) bool {
	switch {
	case '0' <= c && c <= '9':
		return true
	case 'a' <= c && c <= 'f':
		return true
	case 'A' <= c && c <= 'F':
		return true
	}
	return false
}

// unhex returns the hexadecimal value of the hexadecimal character c.
// For example, unhex('A') returns 10.
func unhex(c byte) byte {
	switch {
	case '0' <= c && c <= '9':
		return c - '0'
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

// unescape unescapes s per the rules described in the package docstring.
// It returns an error if s has invalid escape sequences, or if
// escape(unescape(s)) != s.
// The code is modified from https://golang.org/src/net/url/url.go.
func unescape(s string) (string, error) {
	// Count %, check that they're well-formed.
	n := 0
	replacePathSeparator := false
	for i := 0; i < len(s); {
		switch s[i] {
		case '%':
			n++
			if i+2 >= len(s) || !ishex(s[i+1]) || !ishex(s[i+2]) {
				bad := s[i:]
				if len(bad) > 3 {
					bad = bad[:3]
				}
				return "", fmt.Errorf("couldn't unescape %q near %q", s, bad)
			}
			i += 3
		case os.PathSeparator:
			replacePathSeparator = os.PathSeparator != '/'
			i++
		default:
			i++
		}
	}
	unescaped := s
	if n > 0 || replacePathSeparator {
		t := make([]byte, len(s)-2*n)
		j := 0
		for i := 0; i < len(s); {
			switch s[i] {
			case '%':
				t[j] = unhex(s[i+1])<<4 | unhex(s[i+2])
				j++
				i += 3
			case os.PathSeparator:
				t[j] = '/'
				j++
				i++
			default:
				t[j] = s[i]
				j++
				i++
			}
		}
		unescaped = string(t)
	}
	escaped := escape(unescaped)
	if escaped != s {
		return "", fmt.Errorf("%q unescaped to %q but escaped back to %q instead of itself", s, unescaped, escaped)
	}
	return unescaped, nil
}

// IsNotExist implements driver.IsNotExist.
func (b *bucket) IsNotExist(err error) bool {
	return os.IsNotExist(err)
}

var errNotImplemented = errors.New("not implemented")

// IsNotImplemented implements driver.IsNotImplemented.
func (b *bucket) IsNotImplemented(err error) bool {
	return err == errNotImplemented
}

// forKey returns the full path, os.FileInfo, and attributes for key.
func (b *bucket) forKey(key string) (string, os.FileInfo, *xattrs, error) {
	relpath := escape(key)
	path := filepath.Join(b.dir, relpath)
	if strings.HasSuffix(path, attrsExt) {
		return "", nil, nil, errAttrsExt
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", nil, nil, err
	}
	xa, err := getAttrs(path)
	if err != nil {
		return "", nil, nil, err
	}
	return path, info, &xa, nil
}

// ListPaged implements driver.ListPaged.
func (b *bucket) ListPaged(ctx context.Context, opts *driver.ListOptions) (*driver.ListPage, error) {

	var pageToken string
	if len(opts.PageToken) > 0 {
		pageToken = string(opts.PageToken)
	}
	pageSize := opts.PageSize
	if pageSize == 0 {
		pageSize = defaultPageSize
	}
	// If opts.Delimiter != "", lastPrefix contains the last "directory" key we
	// added. It is used to avoid adding it again; all files in this "directory"
	// are collapsed to the single directory entry.
	var lastPrefix string

	// Do a full recursive scan of the root directory.
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
		// Skip all directories. If opts.Delimiter is set, we'll create
		// pseudo-directories later.
		// Note that returning nil means that we'll still recurse into it;
		// we're just not adding a result for the directory itself.
		if info.IsDir() {
			key += "/"
			// Avoid recursing into subdirectories if the directory name already
			// doesn't match the prefix; any files in it are guaranteed not to match.
			if len(key) > len(opts.Prefix) && !strings.HasPrefix(key, opts.Prefix) {
				return filepath.SkipDir
			}
			// Similarly, avoid recursing into subdirectories if we're making
			// "directories" and all of the files in this subdirectory are guaranteed
			// to collapse to a "directory" that we've already added.
			if lastPrefix != "" && strings.HasPrefix(key, lastPrefix) {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip files/directories that don't match the Prefix.
		if !strings.HasPrefix(key, opts.Prefix) {
			return nil
		}
		obj := &driver.ListObject{
			Key:     key,
			ModTime: info.ModTime(),
			Size:    info.Size(),
		}
		// If using Delimiter, collapse "directories".
		if opts.Delimiter != "" {
			// Strip the prefix, which may contain Delimiter.
			keyWithoutPrefix := key[len(opts.Prefix):]
			// See if the key still contains Delimiter.
			// If no, it's a file and we just include it.
			// If yes, it's a file in a "sub-directory" and we want to collapse
			// all files in that "sub-directory" into a single "directory" result.
			if idx := strings.Index(keyWithoutPrefix, opts.Delimiter); idx != -1 {
				prefix := opts.Prefix + keyWithoutPrefix[0:idx+len(opts.Delimiter)]
				// We've already included this "directory"; don't add it.
				if prefix == lastPrefix {
					return nil
				}
				// Update the object to be a "directory".
				obj = &driver.ListObject{
					Key:   prefix,
					IsDir: true,
				}
				lastPrefix = prefix
			}
		}
		// If there's a pageToken, skip anything before it.
		if pageToken != "" && obj.Key <= pageToken {
			return nil
		}
		// If we've already got a full page of results, set NextPageToken and stop.
		if len(result.Objects) == pageSize {
			result.NextPageToken = []byte(result.Objects[pageSize-1].Key)
			return io.EOF
		}
		result.Objects = append(result.Objects, obj)
		return nil
	})
	if err != nil && err != io.EOF {
		return nil, err
	}
	return &result, nil
}

// As implements driver.As.
func (b *bucket) As(i interface{}) bool { return false }

// As implements driver.ErrorAs.
func (b *bucket) ErrorAs(err error, i interface{}) bool { return false }

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
		return nil, err
	}
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return nil, err
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
func (b *bucket) NewTypedWriter(ctx context.Context, key string, contentType string, opts *driver.WriterOptions) (driver.Writer, error) {
	path := filepath.Join(b.dir, escape(key))
	if strings.HasSuffix(path, attrsExt) {
		return nil, errAttrsExt
	}
	if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
		return nil, err
	}
	f, err := ioutil.TempFile("", "fileblob")
	if err != nil {
		return nil, err
	}
	if opts.BeforeWrite != nil {
		if err := opts.BeforeWrite(func(interface{}) bool { return false }); err != nil {
			return nil, err
		}
	}
	var metadata map[string]string
	if len(opts.Metadata) > 0 {
		metadata = opts.Metadata
	}
	attrs := xattrs{
		ContentType: contentType,
		Metadata:    metadata,
	}
	w := &writer{
		ctx:   ctx,
		f:     f,
		path:  path,
		attrs: attrs,
	}
	if len(opts.ContentMD5) > 0 {
		w.contentMD5 = opts.ContentMD5
		w.md5hash = md5.New()
	}
	return w, nil
}

type writer struct {
	ctx        context.Context
	f          *os.File
	path       string
	attrs      xattrs
	contentMD5 []byte
	md5hash    hash.Hash
}

func (w writer) Write(p []byte) (n int, err error) {
	if w.md5hash != nil {
		if _, err := w.md5hash.Write(p); err != nil {
			return 0, err
		}
	}
	return w.f.Write(p)
}

func (w writer) Close() error {
	err := w.f.Close()
	if err != nil {
		return err
	}
	// Always delete the temp file. On success, it will have been renamed so
	// the Remove will fail.
	defer func() {
		_ = os.Remove(w.f.Name())
	}()

	// Check if the write was cancelled.
	if err := w.ctx.Err(); err != nil {
		return err
	}

	// Check MD5 hash if necessary.
	if w.md5hash != nil {
		md5sum := w.md5hash.Sum(nil)
		if !bytes.Equal(md5sum, w.contentMD5) {
			return fmt.Errorf(
				"the ContentMD5 you specified did not match what we received (%s != %s)",
				base64.StdEncoding.EncodeToString(md5sum),
				base64.StdEncoding.EncodeToString(w.contentMD5),
			)
		}
	}
	// Write the attributes file.
	if err := setAttrs(w.path, w.attrs); err != nil {
		return err
	}
	// Rename the temp file to path.
	if err := os.Rename(w.f.Name(), w.path); err != nil {
		_ = os.Remove(w.path + attrsExt)
		return err
	}
	return nil
}

// Delete implements driver.Delete.
func (b *bucket) Delete(ctx context.Context, key string) error {
	path := filepath.Join(b.dir, escape(key))
	if strings.HasSuffix(path, attrsExt) {
		return errAttrsExt
	}
	err := os.Remove(path)
	if err != nil {
		return err
	}
	if err = os.Remove(path + attrsExt); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (b *bucket) SignedURL(ctx context.Context, key string, opts *driver.SignedURLOptions) (string, error) {
	// TODO(Issue #546): Implemented SignedURL for fileblob.
	return "", errNotImplemented
}
