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

// Package blob provides an easy and portable way to interact with blobs
// within a storage location, hereafter called a "bucket".
//
// It supports operations like reading and writing blobs (using standard
// interfaces from the io package), deleting blobs, and listing blobs in a
// bucket.
//
// Subpackages contain distinct implementations of blob for various providers,
// including Cloud and on-prem solutions. For example, "fileblob" supports
// blobs backed by a filesystem. Your application should import one of these
// provider-specific subpackages and use its exported function(s) to create a
// *Bucket; do not use the NewBucket function in this package. For example:
//
//  bucket, err := fileblob.OpenBucket("path/to/dir", nil)
//  if err != nil {
//      return fmt.Errorf("could not open bucket: %v", err)
//  }
//  buf, err := bucket.ReadAll(ctx.Background(), "myfile.txt")
//  ...
//
// Then, write your application code using the *Bucket type, and you can easily
// reconfigure your initialization code to choose a different provider.
// You can develop your application locally using fileblob, or deploy it to
// multiple Cloud providers. You may find http://github.com/google/wire useful
// for managing your initialization code.
//
// Alternatively, you can construct a *Bucket using blob.Open by providing
// a URL that's supported by a blob subpackage that you have linked
// in to your application.
package blob // import "gocloud.dev/blob"

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"gocloud.dev/blob/driver"
)

// Reader reads bytes from a blob.
// It implements io.ReadCloser, and must be closed after
// reads are finished.
type Reader struct {
	b driver.Bucket
	r driver.Reader
}

// Read implements io.Reader (https://golang.org/pkg/io/#Reader).
func (r *Reader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	return n, wrapError(r.b, err)
}

// Close implements io.Closer (https://golang.org/pkg/io/#Closer).
func (r *Reader) Close() error {
	return wrapError(r.b, r.r.Close())
}

// ContentType returns the MIME type of the blob.
func (r *Reader) ContentType() string {
	return r.r.Attributes().ContentType
}

// ModTime returns the time the blob was last modified.
func (r *Reader) ModTime() time.Time {
	return r.r.Attributes().ModTime
}

// Size returns the size of the blob content in bytes.
func (r *Reader) Size() int64 {
	return r.r.Attributes().Size
}

// As converts i to provider-specific types.
// See Bucket.As for more details.
func (r *Reader) As(i interface{}) bool {
	return r.r.As(i)
}

// Attributes contains attributes about a blob.
type Attributes struct {
	// ContentType is the MIME type of the blob. It will not be empty.
	ContentType string
	// Metadata holds key/value pairs associated with the blob.
	// Keys are guaranteed to be in lowercase, even if the backend provider
	// has case-sensitive keys (although note that Metadata written via
	// this package will always be lowercased). If there are duplicate
	// case-insensitive keys (e.g., "foo" and "FOO"), only one value
	// will be kept, and it is undefined which one.
	Metadata map[string]string
	// ModTime is the time the blob was last modified.
	ModTime time.Time
	// Size is the size of the blob's content in bytes.
	Size int64

	asFunc func(interface{}) bool
}

// As converts i to provider-specific types.
// See Bucket.As for more details.
func (a *Attributes) As(i interface{}) bool {
	if a.asFunc == nil {
		return false
	}
	return a.asFunc(i)
}

// Writer writes bytes to a blob.
//
// It implements io.WriteCloser (https://golang.org/pkg/io/#Closer), and must be
// closed after all writes are done.
type Writer struct {
	b driver.Bucket
	w driver.Writer

	// These fields exist only when w is not created in the first place when
	// NewWriter is called.
	//
	// A ctx is stored in the Writer since we need to pass it into NewTypedWriter
	// when we finish detecting the content type of the blob and create the
	// underlying driver.Writer. This step happens inside Write or Close and
	// neither of them take a context.Context as an argument. The ctx must be set
	// to nil after we have passed it.
	ctx  context.Context
	key  string
	opts *driver.WriterOptions
	buf  *bytes.Buffer
}

// sniffLen is the byte size of Writer.buf used to detect content-type.
const sniffLen = 512

// Write implements the io.Writer interface (https://golang.org/pkg/io/#Writer).
//
// Writes may happen asynchronously, so the returned error can be nil
// even if the actual write eventually fails. The write is only guaranteed to
// have succeeded if Close returns no error.
func (w *Writer) Write(p []byte) (n int, err error) {
	if w.w != nil {
		n, err := w.w.Write(p)
		return n, wrapError(w.b, err)
	}

	// If w is not yet created due to no content-type being passed in, try to sniff
	// the MIME type based on at most 512 bytes of the blob content of p.

	// Detect the content-type directly if the first chunk is at least 512 bytes.
	if w.buf.Len() == 0 && len(p) >= sniffLen {
		return w.open(p)
	}

	// Store p in w.buf and detect the content-type when the size of content in
	// w.buf is at least 512 bytes.
	w.buf.Write(p)
	if w.buf.Len() >= sniffLen {
		return w.open(w.buf.Bytes())
	}
	return len(p), nil
}

// Close closes the blob writer. The write operation is not guaranteed to have succeeded until
// Close returns with no error.
// Close may return an error if the context provided to create the Writer is
// canceled or reaches its deadline.
func (w *Writer) Close() error {
	if w.w != nil {
		return wrapError(w.b, w.w.Close())
	}
	if _, err := w.open(w.buf.Bytes()); err != nil {
		return err
	}
	return wrapError(w.b, w.w.Close())
}

// open tries to detect the MIME type of p and write it to the blob.
// The error it returns is wrapped.
func (w *Writer) open(p []byte) (int, error) {
	ct := http.DetectContentType(p)
	var err error
	if w.w, err = w.b.NewTypedWriter(w.ctx, w.key, ct, w.opts); err != nil {
		return 0, wrapError(w.b, err)
	}
	w.buf = nil
	w.ctx = nil
	w.key = ""
	w.opts = nil
	n, err := w.w.Write(p)
	return n, wrapError(w.b, err)
}

// ListOptions sets options for listing blobs via Bucket.List.
type ListOptions struct {
	// Prefix indicates that only blobs with a key starting with this prefix
	// should be returned.
	Prefix string
	// Delimiter sets the delimiter used to define a hierarchical namespace,
	// like a filesystem with "directories".
	//
	// An empty delimiter means that the bucket is treated as a single flat
	// namespace.
	//
	// A non-empty delimiter means that any result with the delimiter in its key
	// after Prefix is stripped will be returned with ListObject.IsDir = true,
	// ListObject.Key truncated after the delimiter, and zero values for other
	// ListObject fields. These results represent "directories". Multiple results
	// in a "directory" are returned as a single result.
	Delimiter string

	// BeforeList is a callback that will be called before each call to the
	// the underlying provider's list functionality.
	// asFunc converts its argument to provider-specific types.
	// See Bucket.As for more details.
	BeforeList func(asFunc func(interface{}) bool) error
}

// ListIterator iterates over List results.
type ListIterator struct {
	b       driver.Bucket
	opts    *driver.ListOptions
	page    *driver.ListPage
	nextIdx int
}

// Next returns a *ListObject for the next blob. It returns (nil, io.EOF) if
// there are no more.
func (i *ListIterator) Next(ctx context.Context) (*ListObject, error) {
	if i.page != nil {
		// We've already got a page of results.
		if i.nextIdx < len(i.page.Objects) {
			// Next object is in the page; return it.
			dobj := i.page.Objects[i.nextIdx]
			i.nextIdx++
			return &ListObject{
				Key:     dobj.Key,
				ModTime: dobj.ModTime,
				Size:    dobj.Size,
				IsDir:   dobj.IsDir,
				asFunc:  dobj.AsFunc,
			}, nil
		}
		if len(i.page.NextPageToken) == 0 {
			// Done with current page, and there are no more; return io.EOF.
			return nil, io.EOF
		}
		// We need to load the next page.
		i.opts.PageToken = i.page.NextPageToken
	}
	// Loading a new page.
	p, err := i.b.ListPaged(ctx, i.opts)
	if err != nil {
		return nil, wrapError(i.b, err)
	}
	i.page = p
	i.nextIdx = 0
	return i.Next(ctx)
}

// ListObject represents a single blob returned from List.
type ListObject struct {
	// Key is the key for this blob.
	Key string
	// ModTime is the time the blob was last modified.
	ModTime time.Time
	// Size is the size of the blob's content in bytes.
	Size int64
	// IsDir indicates that this result represents a "directory" in the
	// hierarchical namespace, ending in ListOptions.Delimiter. Key can be
	// passed as ListOptions.Prefix to list items in the "directory".
	// Fields other than Key and IsDir will not be set if IsDir is true.
	IsDir bool

	asFunc func(interface{}) bool
}

// As converts i to provider-specific types.
// See Bucket.As for more details.
func (o *ListObject) As(i interface{}) bool {
	if o.asFunc == nil {
		return false
	}
	return o.asFunc(i)
}

// Bucket provides an easy and portable way to interact with blobs
// within a "bucket", including read, write, and list operations.
// To create a Bucket, use constructors found in provider-specific
// subpackages.
type Bucket struct {
	b driver.Bucket
}

// NewBucket creates a new *Bucket based on a specific driver implementation.
// Most end users should use subpackages to construct a *Bucket instead of this
// function; see the package documentation for details.
// It is intended for use by provider implementations.
func NewBucket(b driver.Bucket) *Bucket {
	return &Bucket{b: b}
}

// As converts i to provider-specific types.
//
// This function (and the other As functions in this package) are inherently
// provider-specific, and using them will make that part of your application
// non-portable, so use with care.
//
// See the documentation for the subpackage used to instantiate Bucket to see
// which type(s) are supported.
//
// Usage:
//
// 1. Declare a variable of the provider-specific type you want to access.
//
// 2. Pass a pointer to it to As.
//
// 3. If the type is supported, As will return true and copy the
// provider-specific type into your variable. Otherwise, it will return false.
//
// Provider-specific types that are intended to be mutable will be exposed
// as a pointer to the underlying type.
//
// See
// https://github.com/google/go-cloud/blob/master/internal/docs/design.md#as
// for more background.
func (b *Bucket) As(i interface{}) bool {
	if i == nil {
		return false
	}
	return b.b.As(i)
}

// ReadAll is a shortcut for creating a Reader via NewReader with nil
// ReaderOptions, and reading the entire blob.
func (b *Bucket) ReadAll(ctx context.Context, key string) ([]byte, error) {
	r, err := b.NewReader(ctx, key, nil)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return ioutil.ReadAll(r)
}

// List returns a ListIterator that can be used to iterate over blobs in a
// bucket, in lexicographical order of UTF-8 encoded keys. The underlying
// implementation fetches results in pages.
// Use ListOptions to control the page size and filtering.
//
// A nil ListOptions is treated the same as the zero value.
//
// List is not guaranteed to include all recently-written blobs;
// some providers are only eventually consistent.
func (b *Bucket) List(opts *ListOptions) *ListIterator {
	if opts == nil {
		opts = &ListOptions{}
	}
	dopts := &driver.ListOptions{
		Prefix:     opts.Prefix,
		Delimiter:  opts.Delimiter,
		BeforeList: opts.BeforeList,
	}
	return &ListIterator{b: b.b, opts: dopts}
}

// Attributes returns attributes for the blob stored at key.
//
// If the blob does not exist, Attributes returns an error for which
// IsNotExist will return true.
func (b *Bucket) Attributes(ctx context.Context, key string) (Attributes, error) {
	a, err := b.b.Attributes(ctx, key)
	if err != nil {
		return Attributes{}, wrapError(b.b, err)
	}
	var md map[string]string
	if len(a.Metadata) > 0 {
		// Providers are inconsistent, but at least some treat keys
		// as case-insensitive. To make the behavior consistent, we
		// force-lowercase them when writing and reading.
		md = make(map[string]string, len(a.Metadata))
		for k, v := range a.Metadata {
			md[strings.ToLower(k)] = v
		}
	}
	return Attributes{
		ContentType: a.ContentType,
		Metadata:    md,
		ModTime:     a.ModTime,
		Size:        a.Size,
		asFunc:      a.AsFunc,
	}, nil
}

// NewReader is a shortcut for NewRangedReader with offset=0 and length=-1.
func (b *Bucket) NewReader(ctx context.Context, key string, opts *ReaderOptions) (*Reader, error) {
	return b.NewRangeReader(ctx, key, 0, -1, opts)
}

// NewRangeReader returns a Reader to read content from the blob stored at key.
// It reads at most length (!= 0) bytes starting at offset (>= 0).
// If length is negative, it will read till the end of the blob.
//
// If the blob does not exist, NewRangeReader returns an error for which
// IsNotExist will return true. Attributes is a lighter-weight way
// to check for existence.
//
// A nil ReaderOptions is treated the same as the zero value.
//
// The caller must call Close on the returned Reader when done reading.
func (b *Bucket) NewRangeReader(ctx context.Context, key string, offset, length int64, opts *ReaderOptions) (*Reader, error) {
	if offset < 0 {
		return nil, errors.New("blob.NewRangeReader: offset must be non-negative")
	}
	if length == 0 {
		return nil, errors.New("blob.NewRangeReader: length cannot be 0")
	}
	if opts == nil {
		opts = &ReaderOptions{}
	}
	dopts := &driver.ReaderOptions{}
	r, err := b.b.NewRangeReader(ctx, key, offset, length, dopts)
	if err != nil {
		return nil, wrapError(b.b, err)
	}
	return &Reader{b: b.b, r: r}, nil
}

// WriteAll is a shortcut for creating a Writer via NewWriter and writing p.
func (b *Bucket) WriteAll(ctx context.Context, key string, p []byte, opts *WriterOptions) error {
	w, err := b.NewWriter(ctx, key, opts)
	if err != nil {
		return err
	}
	if _, err := w.Write(p); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

// NewWriter returns a Writer that writes to the blob stored at key.
// A nil WriterOptions is treated the same as the zero value.
//
// If a blob with this key already exists, it will be replaced.
// The blob being written is not guaranteed to be readable until Close
// has been called; until then, any previous blob will still be readable.
// Even after Close is called, newly written blobs are not guaranteed to be
// returned from List; some providers are only eventually consistent.
//
// The returned Writer will store ctx for later use in Write and/or Close.
// To abort a write, cancel ctx; otherwise, it must remain open until
// Close is called.
//
// The caller must call Close on the returned Writer, even if the write is
// aborted.
func (b *Bucket) NewWriter(ctx context.Context, key string, opts *WriterOptions) (*Writer, error) {
	var dopts *driver.WriterOptions
	var w driver.Writer
	if opts == nil {
		opts = &WriterOptions{}
	}
	dopts = &driver.WriterOptions{
		ContentMD5:  opts.ContentMD5,
		BufferSize:  opts.BufferSize,
		BeforeWrite: opts.BeforeWrite,
	}
	if len(opts.Metadata) > 0 {
		// Providers are inconsistent, but at least some treat keys
		// as case-insensitive. To make the behavior consistent, we
		// force-lowercase them when writing and reading.
		md := make(map[string]string, len(opts.Metadata))
		for k, v := range opts.Metadata {
			if k == "" {
				return nil, errors.New("blob.NewWriter: WriterOptions.Metadata keys may not be empty strings")
			}
			lowerK := strings.ToLower(k)
			if _, found := md[lowerK]; found {
				return nil, fmt.Errorf("blob.NewWriter: duplicate case-insensitive metadata key %q", lowerK)
			}
			md[lowerK] = v
		}
		dopts.Metadata = md
	}
	if opts.ContentType != "" {
		t, p, err := mime.ParseMediaType(opts.ContentType)
		if err != nil {
			return nil, err
		}
		ct := mime.FormatMediaType(t, p)
		w, err = b.b.NewTypedWriter(ctx, key, ct, dopts)
		if err != nil {
			return nil, wrapError(b.b, err)
		}
		return &Writer{b: b.b, w: w}, nil
	}
	return &Writer{
		ctx:  ctx,
		b:    b.b,
		key:  key,
		opts: dopts,
		buf:  bytes.NewBuffer([]byte{}),
	}, nil
}

// Delete deletes the blob stored at key.
//
// If the blob does not exist, Delete returns an error for which
// IsNotExist will return true.
func (b *Bucket) Delete(ctx context.Context, key string) error {
	return wrapError(b.b, b.b.Delete(ctx, key))
}

// SignedURL returns a URL that can be used to GET the blob for the duration
// specified in opts.Expiry.
//
// A nil SignedURLOptions is treated the same as the zero value.
//
// If the provider implementation does not support this functionality, SignedURL
// will return an error for which IsNotImplemented will return true.
func (b *Bucket) SignedURL(ctx context.Context, key string, opts *SignedURLOptions) (string, error) {
	if opts == nil {
		opts = &SignedURLOptions{}
	}
	if opts.Expiry < 0 {
		return "", errors.New("blob.SignedURL: SignedURLOptions.Expiry must be >= 0")
	}
	if opts.Expiry == 0 {
		opts.Expiry = DefaultSignedURLExpiry
	}
	dopts := driver.SignedURLOptions{
		Expiry: opts.Expiry,
	}
	url, err := b.b.SignedURL(ctx, key, &dopts)
	return url, wrapError(b.b, err)
}

// DefaultSignedURLExpiry is the default duration for SignedURLOptions.Expiry.
const DefaultSignedURLExpiry = 1 * time.Hour

// SignedURLOptions sets options for SignedURL.
type SignedURLOptions struct {
	// Expiry sets how long the returned URL is valid for.
	// Defaults to DefaultSignedURLExpiry.
	Expiry time.Duration
}

// ReaderOptions sets options for NewReader and NewRangedReader.
// It is provided for future extensibility.
type ReaderOptions struct{}

// WriterOptions sets options for NewWriter.
type WriterOptions struct {
	// BufferSize changes the default size in bytes of the chunks that
	// Writer will upload in a single request; larger blobs will be split into
	// multiple requests.
	//
	// This option may be ignored by some provider implementations.
	//
	// If 0, the provider implementation will choose a reasonable default.
	//
	// If the Writer is used to do many small writes concurrently, using a
	// smaller BufferSize may reduce memory usage.
	BufferSize int

	// ContentType specifies the MIME type of the blob being written. If not set,
	// it will be inferred from the content using the algorithm described at
	// http://mimesniff.spec.whatwg.org/.
	ContentType string

	// ContentMD5 may be used as a message integrity check (MIC).
	// https://tools.ietf.org/html/rfc1864
	ContentMD5 []byte

	// Metadata holds key/value strings to be associated with the blob, or nil.
	// Keys may not be empty, and are lowercased before being written.
	// Duplicate case-insensitive keys (e.g., "foo" and "FOO") will result in
	// an error.
	Metadata map[string]string

	// BeforeWrite is a callback that will be called exactly once, before
	// any data is written (unless NewWriter returns an error, in which case
	// it will not be called at all). Note that this is not necessarily during
	// or after the first Write call, as providers may buffer bytes before
	// sending an upload request.
	//
	// asFunc converts its argument to provider-specific types.
	// See Bucket.As for more details.
	BeforeWrite func(asFunc func(interface{}) bool) error
}

// FromURLFunc is intended for use by provider implementations.
// It allows providers to convert a parsed URL from Open to a driver.Bucket.
type FromURLFunc func(context.Context, *url.URL) (driver.Bucket, error)

var (
	// registry maps scheme strings to provider-specific instantiation functions.
	registry = map[string]FromURLFunc{}
	// registryMu protected registry.
	registryMu sync.Mutex
)

// Register is for use by provider implementations. It allows providers to
// register an instantiation function for URLs with the given scheme. It is
// expected to be called from the provider implementation's package init
// function.
//
// fn will be called from Open, with a bucket name and options parsed from
// the URL. All option keys will be lowercased.
//
// Register panics if a provider has already registered for scheme.
func Register(scheme string, fn FromURLFunc) {
	registryMu.Lock()
	defer registryMu.Unlock()

	if _, found := registry[scheme]; found {
		log.Fatalf("a provider has already registered for scheme %q", scheme)
	}
	registry[scheme] = fn
}

// fromRegistry looks up the registered function for scheme.
// It returns nil if scheme has not been registered for.
func fromRegistry(scheme string) FromURLFunc {
	registryMu.Lock()
	defer registryMu.Unlock()

	return registry[scheme]
}

// Open creates a *Bucket from a URL.
// See the package documentation in provider-specific subpackages for more
// details on supported scheme(s) and URL parameter(s).
func Open(ctx context.Context, urlstr string) (*Bucket, error) {
	u, err := url.Parse(urlstr)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" {
		return nil, fmt.Errorf("invalid URL %q, missing scheme", urlstr)
	}
	fn := fromRegistry(u.Scheme)
	if fn == nil {
		return nil, fmt.Errorf("no provider registered for scheme %q", u.Scheme)
	}
	drv, err := fn(ctx, u)
	if err != nil {
		return nil, err
	}
	return NewBucket(drv), nil
}

// wrappedError is used to wrap all errors returned by drivers so that users
// are not given access to provider-specific errors.
type wrappedError struct {
	err error
	b   driver.Bucket
}

func wrapError(b driver.Bucket, err error) error {
	if err == nil {
		return nil
	}
	if err == io.EOF {
		return err
	}
	return &wrappedError{b: b, err: err}
}

func (w *wrappedError) Error() string {
	return "blob: " + w.err.Error()
}

// IsNotExist returns true iff err indicates that the referenced blob does not exist.
func IsNotExist(err error) bool {
	if e, ok := err.(*wrappedError); ok {
		return e.b.IsNotExist(e.err)
	}
	return false
}

// IsNotImplemented returns true iff err indicates that the provider does not
// support the given operation.
func IsNotImplemented(err error) bool {
	if e, ok := err.(*wrappedError); ok {
		return e.b.IsNotImplemented(e.err)
	}
	return false
}

// ErrorAs converts i to provider-specific types.
// See Bucket.As for more details.
func ErrorAs(err error, i interface{}) bool {
	if err == nil || i == nil {
		return false
	}
	if e, ok := err.(*wrappedError); ok {
		return e.b.ErrorAs(e.err, i)
	}
	return false
}
