// Copyright 2026 The Go Cloud Development Kit Authors
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

package memblob

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"hash/crc32"
	"io"
	"maps"
	"sort"
	"strconv"
	"sync"
	"time"

	"gocloud.dev/blob/driver"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/gcerr"
)

// uploadSession holds the parts of one in-progress multipart upload.
type uploadSession struct {
	key  string
	opts driver.MultipartUploaderOptions

	mu    sync.Mutex
	parts map[int64][]byte
}

// NewMultipartUploader implements driver.MultipartUploaderBucket.
func (b *bucket) NewMultipartUploader(ctx context.Context, key string, opts *driver.MultipartUploaderOptions) (driver.MultipartUploader, error) {
	if key == "" {
		return nil, errInvalidKey
	}
	if opts == nil {
		opts = &driver.MultipartUploaderOptions{}
	}
	if opts.BeforeUpload != nil {
		if err := opts.BeforeUpload(func(any) bool { return false }); err != nil {
			return nil, err
		}
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextUploadID++
	id := strconv.FormatInt(b.nextUploadID, 10)
	sess := &uploadSession{key: key, opts: *opts, parts: map[int64][]byte{}}
	sess.opts.Metadata = maps.Clone(opts.Metadata)
	if b.uploads == nil {
		b.uploads = map[string]*uploadSession{}
	}
	b.uploads[id] = sess
	return &multipartUploader{b: b, uploadID: id}, nil
}

// OpenMultipartUploader implements driver.MultipartUploaderBucket.
func (b *bucket) OpenMultipartUploader(ctx context.Context, key string, uploadID string) (driver.MultipartUploader, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	sess, ok := b.uploads[uploadID]
	if !ok {
		return nil, gcerr.Newf(gcerrors.NotFound, nil, "memblob: no multipart upload with ID %q", uploadID)
	}
	if sess.key != key {
		return nil, gcerr.Newf(gcerrors.InvalidArgument, nil, "memblob: multipart upload %q is for key %q, not %q", uploadID, sess.key, key)
	}
	return &multipartUploader{b: b, uploadID: uploadID}, nil
}

type multipartUploader struct {
	b        *bucket
	uploadID string
}

func (u *multipartUploader) UploadID() string { return u.uploadID }

// session looks the upload up every time rather than holding a reference, so
// that using an uploader after Abort or Commit reports a missing upload
// instead of quietly writing into a discarded one.
func (u *multipartUploader) session() (*uploadSession, error) {
	u.b.mu.Lock()
	defer u.b.mu.Unlock()
	sess, ok := u.b.uploads[u.uploadID]
	if !ok {
		return nil, gcerr.Newf(gcerrors.NotFound, nil, "memblob: multipart upload %q is no longer open", u.uploadID)
	}
	return sess, nil
}

func (u *multipartUploader) UploadPart(ctx context.Context, part driver.UploaderPart, r io.Reader) (driver.UploaderPart, error) {
	sess, err := u.session()
	if err != nil {
		return driver.UploaderPart{}, err
	}
	if part.Number < 1 {
		return driver.UploaderPart{}, gcerr.Newf(gcerrors.InvalidArgument, nil, "memblob: part numbers start at 1, got %d", part.Number)
	}
	content, err := io.ReadAll(r)
	if err != nil {
		return driver.UploaderPart{}, err
	}
	if len(part.MD5) > 0 {
		if sum := md5.Sum(content); !bytes.Equal(part.MD5, sum[:]) {
			return driver.UploaderPart{}, gcerr.Newf(gcerrors.FailedPrecondition, nil, "memblob: MD5 mismatch for part %d", part.Number)
		}
	}
	if part.CRC32C != nil {
		if got := crc32.Checksum(content, crc32.MakeTable(crc32.Castagnoli)); got != *part.CRC32C {
			return driver.UploaderPart{}, gcerr.Newf(gcerrors.FailedPrecondition, nil, "memblob: CRC32C mismatch for part %d", part.Number)
		}
	}

	sess.mu.Lock()
	sess.parts[part.Number] = content
	sess.mu.Unlock()

	part.ID = strconv.FormatInt(part.Number, 10)
	part.Size = int64(len(content))
	return part, nil
}

func (u *multipartUploader) Commit(ctx context.Context, parts []driver.UploaderPart) error {
	sess, err := u.session()
	if err != nil {
		return err
	}

	// Parts may be presented in any order; the object is assembled in
	// ascending part number.
	ordered := make([]driver.UploaderPart, len(parts))
	copy(ordered, parts)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Number < ordered[j].Number })

	var content []byte
	sess.mu.Lock()
	for _, p := range ordered {
		body, ok := sess.parts[p.Number]
		if !ok {
			sess.mu.Unlock()
			return gcerr.Newf(gcerrors.FailedPrecondition, nil, "memblob: part %d was never uploaded", p.Number)
		}
		content = append(content, body...)
	}
	sess.mu.Unlock()

	var md5sum []byte
	if !u.b.options.NoMD5 {
		sum := md5.Sum(content)
		md5sum = sum[:]
	}
	now := time.Now()
	entry := &blobEntry{
		Content: content,
		Attributes: &driver.Attributes{
			CacheControl:       sess.opts.CacheControl,
			ContentDisposition: sess.opts.ContentDisposition,
			ContentEncoding:    sess.opts.ContentEncoding,
			ContentLanguage:    sess.opts.ContentLanguage,
			ContentType:        sess.opts.ContentType,
			Metadata:           sess.opts.Metadata,
			Size:               int64(len(content)),
			CreateTime:         now,
			ModTime:            now,
			MD5:                md5sum,
			ETag:               fmt.Sprintf("%q", fmt.Sprintf("%x-%x", now.UnixNano(), len(content))),
		},
	}

	u.b.mu.Lock()
	defer u.b.mu.Unlock()
	if prev := u.b.blobs[sess.key]; prev != nil {
		entry.Attributes.CreateTime = prev.Attributes.CreateTime
	}
	u.b.blobs[sess.key] = entry
	delete(u.b.uploads, u.uploadID)
	return nil
}

func (u *multipartUploader) Abort(ctx context.Context) error {
	u.b.mu.Lock()
	defer u.b.mu.Unlock()
	if _, ok := u.b.uploads[u.uploadID]; !ok {
		return gcerr.Newf(gcerrors.NotFound, nil, "memblob: multipart upload %q is no longer open", u.uploadID)
	}
	delete(u.b.uploads, u.uploadID)
	return nil
}
