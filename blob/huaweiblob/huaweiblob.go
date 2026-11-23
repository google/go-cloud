// Copyright 2018-2019 The Go Cloud Development Kit Authors
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

// Package huaweiblob provides a blob driver for Huawei Cloud OBS (Object
// Storage Service). It implements the Go CDK blob.Bucket abstraction on top
// of the OBS REST API using AWS Signature V4-compatible signing (OBS uses
// "obs" as the service name).
//
// This driver talks to OBS directly over HTTP; it does not use any external
// SDK. Requests are signed with the endpoint, access key ID and secret key
// supplied through Options.
//
// # URLs
//
// For blob.OpenBucket, huaweiblob registers the scheme "obs". You can use the
// prefixed host to specify the bucket name and query parameters for the rest
// of the configuration:
//
//	blob.OpenBucket(ctx, "obs://mybucket?endpoint=obs.cn-north-4.myhuaweicloud.com&access_key=AKIDEXAMPLE&secret_key=SECRET&region=cn-north-4")
//
// The "bucket" portion of the URL (u.Host) is required. The "endpoint",
// "access_key", "secret_key" and "region" query parameters are optional;
// "region" defaults to "cn-north-1" and the other fields to their zero
// values. If the endpoint does not start with "http", the requests use
// virtual-hosted style (https://<bucket>.<endpoint>/...) and HTTPS is
// assumed; otherwise path-style requests are sent to the given endpoint.
//
// # Escaping
//
// Go CDK blob keys are strings; OBS object names are strings. A small
// experimental set of characters is escaped to avoid incompatibilities with
// OBS. The same escaping applies to metadata keys and values. To summarize:
//   - ASCII characters 0-31 are escaped (e.g., a newline is "__0x0a__").
//   - Slashes in "../" are escaped (to avoid parent-directory traversal).
//   - Metadata keys have URL PathEscape applied and "@", ":" and "=" escaped.
//   - Metadata values have URL PathEscape applied.
//
// To read the escaped key back, use the `OBS` as the opaque API? No: decoders
// are internal and unescape keys automatically. Both keys and metadata keys
// are unescaped when read.
//
// # As
//
// huaweiblob exposes the following types for As:
//   - Bucket: supports *http.Client
//   - Error: supports *obsError (and any error via errors.As)
//   - ListObject: supports listContents (with Key, LastModified, ETag, Size)
//     and listCommonPrefix (with Prefix); see ListObjectCheck.
//   - Reader: supports *http.Response
//   - Attributes: supports *http.Header
//   - BeforeList, BeforeRead, BeforeWrite, BeforeCopy, BeforeDelete,
//     BeforeSign callbacks: support **http.Request
package huaweiblob

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gocloud.dev/blob"
	"gocloud.dev/blob/driver"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/escape"
)

const (
	// Scheme is the URL scheme huaweiblob registers its URLOpener under on
	// blob.DefaultMux.
	Scheme = "obs"

	// defaultPageSize is the number of objects requested per ListPaged page
	// when the caller does not specify a PageSize.
	defaultPageSize = 1000

	// defaultRegion is used when Options.Region is empty.
	defaultRegion = "cn-north-1"
)

func init() {
	blob.DefaultURLMux().RegisterBucket(Scheme, &URLOpener{})
}

// Options is the set of configuration options for opening a Huawei OBS bucket.
// Most users will not need these; the OpenBucket URL supports setting endpoint,
// access_key, secret_key and region via query parameters.
type Options struct {
	// Endpoint is the OBS service endpoint, e.g. "obs.cn-north-4.myhuaweicloud.com".
	// If it does not start with "http", requests use virtual-hosted style and
	// HTTPS; otherwise path-style requests are sent to the given endpoint.
	Endpoint string

	// BucketName is the name of the OBS bucket. Required.
	BucketName string

	// AccessKeyID is the OBS access key ID. Required.
	AccessKeyID string

	// SecretKey is the OBS secret key. Required.
	SecretKey string

	// Region is the OBS region, e.g. "cn-north-4". Optional; defaults to
	// "cn-north-1".
	Region string

	// HTTPClient is the client used for making requests. Optional; defaults to
	// http.DefaultClient with a 30s timeout.
	HTTPClient *http.Client
}

// URLOpener opens OBS bucket URLs like "obs://mybucket?...".
type URLOpener struct {
	// Options specifies the default options to use when opening a bucket; the
	// query parameters in the URL override these.
	Options *Options
}

// OpenBucketURL opens a blob.Bucket based on the URL.
func (o *URLOpener) OpenBucketURL(ctx context.Context, u *url.URL) (*blob.Bucket, error) {
	opts := Options{}
	if o.Options != nil {
		opts = *o.Options
	}
	for param, values := range u.Query() {
		if len(values) != 1 {
			return nil, fmt.Errorf("open bucket %v: query parameter %q must have exactly one value", u, param)
		}
		switch param {
		case "endpoint":
			opts.Endpoint = values[0]
		case "access_key":
			opts.AccessKeyID = values[0]
		case "secret_key":
			opts.SecretKey = values[0]
		case "region":
			opts.Region = values[0]
		default:
			return nil, fmt.Errorf("open bucket %v: invalid query parameter %q", u, param)
		}
	}
	opts.BucketName = u.Host
	return OpenBucket(ctx, &opts)
}

// OpenBucket creates a *blob.Bucket backed by a Huawei OBS bucket.
func OpenBucket(ctx context.Context, opts *Options) (*blob.Bucket, error) {
	drv, err := openBucket(ctx, opts)
	if err != nil {
		return nil, err
	}
	return blob.NewBucket(drv), nil
}

func openBucket(ctx context.Context, opts *Options) (driver.Bucket, error) {
	if opts.Endpoint == "" {
		return nil, errors.New("huaweiblob.OpenBucket: endpoint is required")
	}
	if opts.BucketName == "" {
		return nil, errors.New("huaweiblob.OpenBucket: bucketName is required")
	}
	if opts.AccessKeyID == "" {
		return nil, errors.New("huaweiblob.OpenBucket: accessKeyID is required")
	}
	if opts.SecretKey == "" {
		return nil, errors.New("huaweiblob.OpenBucket: secretKey is required")
	}
	region := opts.Region
	if region == "" {
		region = defaultRegion
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &bucket{
		endpoint:    opts.Endpoint,
		bucketName:  opts.BucketName,
		accessKeyID: opts.AccessKeyID,
		secretKey:   opts.SecretKey,
		region:      region,
		client:      client,
	}, nil
}

// bucket represents a Huawei OBS bucket.
type bucket struct {
	endpoint    string
	bucketName  string
	accessKeyID string
	secretKey   string
	region      string

	client *http.Client
}

var (
	_ driver.Bucket   = (*bucket)(nil)
	_ driver.Uploader = (*writer)(nil)
)

// obsError represents an OBS API error response.
type obsError struct {
	// StatusCode is the HTTP status code of the response.
	StatusCode int
	// Key is the blob key the failing request referred to, when known; it is
	// stamped on the error by withKey after the request fails, so the message
	// identifies missing keys even when OBS returns no error body (as for HEAD
	// failures). The zero value means "no key".
	Key string
	// Code is the OBS error code from the XML body (e.g. "NoSuchKey").
	Code string
	// Message is the OBS error message from the XML body.
	Message string
}

// Error formats the error, including the key when known (mirroring the
// portable layer's wrapError). OBS returns no body for HEAD failures (e.g.
// 404 on a missing blob), so the HTTP status text is used as a fallback
// message.
func (e *obsError) Error() string {
	var b strings.Builder
	b.WriteString("huaweiblob: OBS error")
	if e.Key != "" {
		fmt.Fprintf(&b, " (key %q)", e.Key)
	}
	fmt.Fprintf(&b, " (status %d)", e.StatusCode)
	switch {
	case e.Code != "" && e.Message != "":
		fmt.Fprintf(&b, ": %s: %s", e.Code, e.Message)
	case e.Code != "":
		fmt.Fprintf(&b, ": %s", e.Code)
	case e.Message != "":
		b.WriteString(": " + e.Message)
	default:
		b.WriteString(": " + http.StatusText(e.StatusCode))
	}
	return b.String()
}

// withKey stamps the blob key on an *obsError returned by a failed request so
// that the error message includes it, mirroring the portable layer's
// wrapError. It mutates the error in place, preserving the errors.As chain
// required by ErrorAs. Errors that are not *obsError (and empty keys) pass
// through unchanged.
func withKey(err error, key string) error {
	var oe *obsError
	if err != nil && key != "" && errors.As(err, &oe) {
		oe.Key = key
	}
	return err
}

// xmlErr mirrors the OBS error document:
//
//	<Error><Code>NoSuchKey</Code><Message>...</Message></Error>
type xmlErr struct {
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

// Close implements driver.Close.
func (b *bucket) Close() error {
	b.client.CloseIdleConnections()
	return nil
}

// ErrorCode implements driver.ErrorCode.
func (b *bucket) ErrorCode(err error) gcerrors.ErrorCode {
	var oe *obsError
	if errors.As(err, &oe) {
		switch oe.StatusCode {
		case http.StatusNotFound:
			return gcerrors.NotFound
		case http.StatusForbidden, http.StatusUnauthorized:
			return gcerrors.PermissionDenied
		case http.StatusPreconditionFailed:
			return gcerrors.FailedPrecondition
		}
		switch oe.Code {
		case "NoSuchBucket", "NoSuchKey", "NotFound":
			return gcerrors.NotFound
		case "PreconditionFailed":
			return gcerrors.FailedPrecondition
		case "AccessDenied", "Forbidden":
			return gcerrors.PermissionDenied
		}
		return gcerrors.Unknown
	}
	if errors.Is(err, errNotImplemented) {
		return gcerrors.Unimplemented
	}
	return gcerrors.Unknown
}

// errNotImplemented is returned by SignedURL; it maps to gcerrors.Unimplemented
// in ErrorCode so that drivertest skips the signed URL checks.
var errNotImplemented = errors.New("huaweiblob: not implemented")

// As implements driver.As.
func (b *bucket) As(i any) bool {
	p, ok := i.(**http.Client)
	if !ok {
		return false
	}
	*p = b.client
	return true
}

// ErrorAs implements driver.ErrorAs.
func (b *bucket) ErrorAs(err error, i any) bool {
	return errors.As(err, i)
}

// Attributes implements driver.Attributes.
func (b *bucket) Attributes(ctx context.Context, key string) (*driver.Attributes, error) {
	if key == "" {
		return nil, errors.New("invalid key (empty string)")
	}
	origKey := key
	key = escapeKey(key)
	req, err := b.newRequest(ctx, http.MethodHead, "/"+key, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.do(ctx, req)
	if err != nil {
		// OBS returns no body for a HEAD 404, so without the key the error
		// message would be empty; stamp the (unescaped) key on it.
		return nil, withKey(err, origKey)
	}
	defer resp.Body.Close()

	md := make(map[string]string)
	for k, v := range resp.Header {
		lower := strings.ToLower(k)
		if strings.HasPrefix(lower, "x-obs-meta-") {
			// Prefer the native OBS metadata prefix; x-amz-meta-* (set by
			// S3-compatible clients) only fills keys not already present.
			md[unescapeMetadataKey(strings.TrimPrefix(lower, "x-obs-meta-"))] = unescapeMetadataValue(v)
		} else if strings.HasPrefix(lower, "x-amz-meta-") {
			mk := unescapeMetadataKey(strings.TrimPrefix(lower, "x-amz-meta-"))
			if _, ok := md[mk]; !ok {
				md[mk] = unescapeMetadataValue(v)
			}
		}
	}

	etag := resp.Header.Get("ETag")
	var size int64
	if resp.ContentLength >= 0 {
		size = resp.ContentLength
	}
	return &driver.Attributes{
		CacheControl:       resp.Header.Get("Cache-Control"),
		ContentDisposition: resp.Header.Get("Content-Disposition"),
		ContentEncoding:    resp.Header.Get("Content-Encoding"),
		ContentLanguage:    resp.Header.Get("Content-Language"),
		ContentType:        resp.Header.Get("Content-Type"),
		Metadata:           md,
		ModTime:            parseHTTPTime(resp.Header.Get("Last-Modified")),
		Size:               size,
		MD5:                eTagToMD5(etag),
		ETag:               etag,
		AsFunc: func(i any) bool {
			p, ok := i.(*http.Header)
			if !ok {
				return false
			}
			*p = resp.Header
			return true
		},
	}, nil
}

func unescapeMetadataKey(k string) string {
	return escape.HexUnescape(escape.URLUnescape(k))
}

func unescapeMetadataValue(vs []string) string {
	v := ""
	if len(vs) > 0 {
		v = vs[0]
	}
	return escape.URLUnescape(v)
}

// ListPaged implements driver.ListPaged.
func (b *bucket) ListPaged(ctx context.Context, opts *driver.ListOptions) (*driver.ListPage, error) {
	pageSize := opts.PageSize
	if pageSize == 0 {
		pageSize = defaultPageSize
	}
	q := url.Values{}
	q.Set("list-type", "2")
	q.Set("max-keys", strconv.Itoa(pageSize))
	if len(opts.PageToken) > 0 {
		q.Set("continuation-token", string(opts.PageToken))
	}
	if opts.Prefix != "" {
		q.Set("prefix", escapeKey(opts.Prefix))
	}
	if opts.Delimiter != "" {
		q.Set("delimiter", escapeKey(opts.Delimiter))
	}
	req, err := b.newRequest(ctx, http.MethodGet, "/", q, nil, nil)
	if err != nil {
		return nil, err
	}
	if opts.BeforeList != nil {
		asFunc := func(i any) bool {
			p, ok := i.(**http.Request)
			if !ok {
				return false
			}
			*p = req
			return true
		}
		if err := opts.BeforeList(asFunc); err != nil {
			return nil, err
		}
	}
	resp, err := b.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result listBucketResult
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	page := driver.ListPage{}
	if result.NextContinuationToken != "" {
		page.NextPageToken = []byte(result.NextContinuationToken)
	}
	if n := len(result.Contents) + len(result.CommonPrefixes); n > 0 {
		page.Objects = make([]*driver.ListObject, n)
		for i, obj := range result.Contents {
			page.Objects[i] = &driver.ListObject{
				Key:     unescapeKey(obj.Key),
				ModTime: parseListTime(obj.LastModified),
				Size:    obj.Size,
				MD5:     eTagToMD5(obj.ETag),
				AsFunc: func(i any) bool {
					p, ok := i.(*listContents)
					if !ok {
						return false
					}
					*p = obj
					return true
				},
			}
		}
		for i, prefix := range result.CommonPrefixes {
			page.Objects[i+len(result.Contents)] = &driver.ListObject{
				Key:   unescapeKey(prefix.Prefix),
				IsDir: true,
				AsFunc: func(i any) bool {
					p, ok := i.(*listCommonPrefix)
					if !ok {
						return false
					}
					*p = prefix
					return true
				},
			}
		}
		if len(result.Contents) > 0 && len(result.CommonPrefixes) > 0 {
			// OBS gives us blobs and "directories" in separate lists; sort them.
			sort.Slice(page.Objects, func(i, j int) bool {
				return page.Objects[i].Key < page.Objects[j].Key
			})
		}
	}
	return &page, nil
}

// listBucketResult mirrors the S3-compatible ListObjects V2 response XML that
// OBS returns for GET /?list-type=2.
type listBucketResult struct {
	XMLName                xml.Name           `xml:"ListBucketResult"`
	IsTruncated            bool               `xml:"IsTruncated"`
	NextContinuationToken  string             `xml:"NextContinuationToken"`
	Contents               []listContents     `xml:"Contents"`
	CommonPrefixes         []listCommonPrefix `xml:"CommonPrefixes"`
}

// listContents is a single object listed by ListPaged; exposed via
// ListObject.As.
type listContents struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
}

// listCommonPrefix is a single "directory" listed by ListPaged when a
// delimiter is used; exposed via ListObject.As.
type listCommonPrefix struct {
	Prefix string `xml:"Prefix"`
}

// NewRangeReader implements driver.NewRangeReader.
func (b *bucket) NewRangeReader(ctx context.Context, key string, offset, length int64, opts *driver.ReaderOptions) (driver.Reader, error) {
	if key == "" {
		return nil, errors.New("invalid key (empty string)")
	}
	origKey := key
	key = escapeKey(key)
	headers := make(map[string]string)
	switch {
	case offset > 0 && length < 0:
		headers["Range"] = fmt.Sprintf("bytes=%d-", offset)
	case length == 0:
		// OBS doesn't support a zero-length read; we'll read 1 byte and then
		// ignore it in favor of http.NoBody below.
		headers["Range"] = fmt.Sprintf("bytes=%d-%d", offset, offset)
	case length >= 0:
		headers["Range"] = fmt.Sprintf("bytes=%d-%d", offset, offset+length-1)
	}
	req, err := b.newRequest(ctx, http.MethodGet, "/"+key, nil, headers, nil)
	if err != nil {
		return nil, err
	}
	if opts.BeforeRead != nil {
		asFunc := func(i any) bool {
			p, ok := i.(**http.Request)
			if !ok {
				return false
			}
			*p = req
			return true
		}
		if err := opts.BeforeRead(asFunc); err != nil {
			return nil, err
		}
	}
	resp, err := b.do(ctx, req)
	if err != nil {
		return nil, withKey(err, origKey)
	}
	body := resp.Body
	if length == 0 {
		// Close the real body; we won't read it.
		_ = resp.Body.Close()
		body = http.NoBody
	}
	return &reader{
		body: body,
		attrs: driver.ReaderAttributes{
			ContentType: resp.Header.Get("Content-Type"),
			ModTime:     parseHTTPTime(resp.Header.Get("Last-Modified")),
			Size:        getSize(resp.ContentLength, resp.Header.Get("Content-Range")),
		},
		raw: resp,
	}, nil
}

// reader implements driver.Reader.
type reader struct {
	body  io.ReadCloser
	attrs driver.ReaderAttributes
	raw   *http.Response // the underlying HTTP response
}

func (r *reader) Read(p []byte) (int, error) {
	return r.body.Read(p)
}

func (r *reader) Close() error {
	return r.body.Close()
}

func (r *reader) As(i any) bool {
	p, ok := i.(**http.Response)
	if !ok {
		return false
	}
	*p = r.raw
	return true
}

func (r *reader) Attributes() *driver.ReaderAttributes {
	return &r.attrs
}

// NewTypedWriter implements driver.NewTypedWriter.
func (b *bucket) NewTypedWriter(ctx context.Context, key, contentType string, opts *driver.WriterOptions) (driver.Writer, error) {
	if key == "" {
		return nil, errors.New("invalid key (empty string)")
	}
	return &writer{
		ctx:         ctx,
		b:           b,
		key:         key,
		contentType: contentType,
		opts:        opts,
	}, nil
}

// writer buffers the entire object in memory and uploads it with a single PUT
// on Close.
type writer struct {
	ctx         context.Context
	b           *bucket
	key         string
	contentType string
	opts        *driver.WriterOptions
	buf         bytes.Buffer
	closed      bool
}

func (w *writer) Write(p []byte) (int, error) {
	if w.closed {
		return 0, errors.New("huaweiblob: writer is already closed")
	}
	return w.buf.Write(p)
}

func (w *writer) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	// Check context to ensure the caller hasn't canceled.
	if err := w.ctx.Err(); err != nil {
		return err
	}
	return w.b.writeData(w.ctx, w.key, w.buf.Bytes(), w.contentType, w.opts)
}

// writeData uploads a single object to OBS with the given metadata.
func (b *bucket) writeData(ctx context.Context, key string, data []byte, contentType string, opts *driver.WriterOptions) error {
	origKey := key
	if contentType == "" && !opts.DisableContentTypeDetection {
		contentType = mime.TypeByExtension(filepath.Ext(key))
	}
	key = escapeKey(key)
	headers := make(map[string]string)
	if contentType != "" {
		headers["Content-Type"] = contentType
	}
	if opts.CacheControl != "" {
		headers["Cache-Control"] = opts.CacheControl
	}
	if opts.ContentDisposition != "" {
		headers["Content-Disposition"] = opts.ContentDisposition
	}
	if opts.ContentEncoding != "" {
		headers["Content-Encoding"] = opts.ContentEncoding
	}
	if opts.ContentLanguage != "" {
		headers["Content-Language"] = opts.ContentLanguage
	}
	for k, v := range opts.Metadata {
		// See the package comments for more details on escaping of metadata
		// keys & values.
		k = escape.HexEscape(url.PathEscape(k), func(runes []rune, i int) bool {
			return runes[i] == '@' || runes[i] == ':' || runes[i] == '='
		})
		headers["x-obs-meta-"+k] = url.PathEscape(v)
	}
	if opts.IfNotExist {
		headers["If-None-Match"] = "*"
	}
	if opts.BeforeWrite != nil {
		var req *http.Request
		asFunc := func(i any) bool {
			p, ok := i.(**http.Request)
			if !ok {
				return false
			}
			req, _ = b.newRequest(ctx, http.MethodPut, "/"+key, nil, headers, data)
			*p = req
			return true
		}
		if err := opts.BeforeWrite(asFunc); err != nil {
			return err
		}
		if req != nil {
			// The callback already created the request; reuse it.
			resp, err := b.do(ctx, req)
			if err != nil {
				return withKey(err, origKey)
			}
			defer resp.Body.Close()
			_, _ = io.Copy(io.Discard, resp.Body)
			return nil
		}
	}
	if err := b.putObject(ctx, key, data, headers); err != nil {
		return withKey(err, origKey)
	}
	return nil
}

func (b *bucket) putObject(ctx context.Context, key string, data []byte, headers map[string]string) error {
	req, err := b.newRequest(ctx, http.MethodPut, "/"+key, nil, headers, data)
	if err != nil {
		return err
	}
	resp, err := b.do(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// Upload implements driver.Uploader by buffering the reader in memory and
// uploading it with a single PUT on Close.
func (w *writer) Upload(r io.Reader) error {
	if w.closed {
		return errors.New("huaweiblob: writer is already closed")
	}
	_, err := io.Copy(&w.buf, r)
	return err
}

// Copy implements driver.Copy. OBS uses the S3-compatible x-copy-source
// header with the path-style source reference /<bucket>/<key>.
func (b *bucket) Copy(ctx context.Context, dstKey, srcKey string, opts *driver.CopyOptions) error {
	// A failed copy typically means the source is missing, so errors are
	// stamped with the source key.
	if srcKey == "" || dstKey == "" {
		return errors.New("invalid key (empty string)")
	}
	origSrcKey := srcKey
	dstKey = escapeKey(dstKey)
	srcKey = escapeKey(srcKey)
	headers := map[string]string{
		"x-copy-source": "/" + b.bucketName + "/" + srcKey,
	}
	var req *http.Request
	if opts.BeforeCopy != nil {
		asFunc := func(i any) bool {
			p, ok := i.(**http.Request)
			if !ok {
				return false
			}
			req, _ = b.newRequest(ctx, http.MethodPut, "/"+dstKey, nil, headers, nil)
			*p = req
			return true
		}
		if err := opts.BeforeCopy(asFunc); err != nil {
			return err
		}
		if req != nil {
			resp, err := b.do(ctx, req)
			if err != nil {
				return withKey(err, origSrcKey)
			}
			defer resp.Body.Close()
			_, _ = io.Copy(io.Discard, resp.Body)
			return nil
		}
	}
	req, err := b.newRequest(ctx, http.MethodPut, "/"+dstKey, nil, headers, nil)
	if err != nil {
		return err
	}
	resp, err := b.do(ctx, req)
	if err != nil {
		return withKey(err, origSrcKey)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// Delete implements driver.Delete.
func (b *bucket) Delete(ctx context.Context, key string, opts *driver.DeleteOptions) error {
	// OBS DeleteObject succeeds even if the key doesn't exist, so we check
	// existence first to return NotFound.
	if _, err := b.Attributes(ctx, key); err != nil {
		// The error is already stamped with the key by Attributes.
		return err
	}
	origKey := key
	key = escapeKey(key)
	var req *http.Request
	if opts.BeforeDelete != nil {
		asFunc := func(i any) bool {
			p, ok := i.(**http.Request)
			if !ok {
				return false
			}
			req, _ = b.newRequest(ctx, http.MethodDelete, "/"+key, nil, nil, nil)
			*p = req
			return true
		}
		if err := opts.BeforeDelete(asFunc); err != nil {
			return err
		}
		if req != nil {
			resp, err := b.do(ctx, req)
			if err != nil {
				return withKey(err, origKey)
			}
			defer resp.Body.Close()
			_, _ = io.Copy(io.Discard, resp.Body)
			return nil
		}
	}
	req, err := b.newRequest(ctx, http.MethodDelete, "/"+key, nil, nil, nil)
	if err != nil {
		return err
	}
	resp, err := b.do(ctx, req)
	if err != nil {
		return withKey(err, origKey)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// SignedURL implements driver.SignedURL. Not supported for OBS; it returns an
// error that maps to gcerrors.Unimplemented so that conformance tests skip
// signed URL checks.
func (b *bucket) SignedURL(ctx context.Context, key string, opts *driver.SignedURLOptions) (string, error) {
	return "", errNotImplemented
}

// do executes req and returns the response for 2xx statuses. For other
// statuses it reads the (bounded) body, parses the OBS error document and
// returns an *obsError.
func (b *bucket) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		var e xmlErr
		if err := xml.Unmarshal(body, &e); err != nil {
			e.Code = ""
			e.Message = strings.TrimSpace(string(body))
		}
		return nil, &obsError{StatusCode: resp.StatusCode, Code: e.Code, Message: e.Message}
	}
	return resp, nil
}

// newRequest builds a signed HTTP request for the OBS API. path is relative to
// the bucket root (e.g. "/key" or "/" for sub-resources); query (may be nil)
// is appended as the OBS sub-resource or list parameters.
func (b *bucket) newRequest(ctx context.Context, method, path string, query url.Values, headers map[string]string, body []byte) (*http.Request, error) {
	// Build the full URL via url.URL so the object key is percent-encoded
	// consistently: keys may contain characters that are invalid or ambiguous
	// in a URL ("%", "#", "?", spaces, non-ASCII bytes), which would otherwise
	// break URL parsing or corrupt the request target. http.NewRequest
	// re-parses the string, leaving req.URL.Path decoded and req.URL.RawPath
	// holding the encoded form that is sent on the wire.
	canonQuery := canonicalQuery(query)
	u := &url.URL{
		Scheme:   "https",
		Host:     b.bucketName + "." + b.endpoint,
		Path:     path,
		RawQuery: canonQuery,
	}
	if strings.HasPrefix(b.endpoint, "http") {
		// Path-style: the bucket is part of the URL path (e.g. for local or
		// on-premise endpoints).
		ep, err := url.Parse(b.endpoint)
		if err != nil {
			return nil, err
		}
		u.Scheme = ep.Scheme
		u.Host = ep.Host
		u.Path = "/" + b.bucketName + path
	}

	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), reqBody)
	if err != nil {
		return nil, err
	}

	// Add standard headers. The host that is actually sent on the wire is
	// req.Host (= req.URL.Host, set by http.NewRequest), so the signature must
	// cover that exact value regardless of URL style (virtual-hosted vs
	// path-style).
	req.Header.Set("Host", req.URL.Host)

	// Add date header.
	now := time.Now().UTC()
	req.Header.Set("X-Obs-Date", now.Format("20060102T150405Z"))

	// Default content type for raw bodies; caller headers below can override it
	// (e.g. application/xml for sub-resource configurations).
	if body != nil {
		req.Header.Set("Content-Type", "application/octet-stream")
	}

	// Add custom headers.
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Calculate payload hash.
	var payloadHash string
	if body != nil {
		hash := sha256.Sum256(body)
		payloadHash = hex.EncodeToString(hash[:])
	} else {
		emptyHash := sha256.Sum256([]byte{})
		payloadHash = hex.EncodeToString(emptyHash[:])
	}
	req.Header.Set("X-Obs-Content-Sha256", payloadHash)

	// Generate and add signature (Huawei OBS uses AWS SigV4-compatible signing).
	// Sign the path actually present on the wire: in path-style mode the bucket
	// prefix is part of the wire path and must be covered by the signature, in
	// virtual-hosted mode it lives in the host. Using req.URL.EscapedPath()
	// (the percent-encoded path as sent) makes the canonical request match the
	// wire bytes either way; canonicalURI passes %XX triples through untouched.
	signature := b.generateSignatureV4(method, req.URL.EscapedPath(), canonQuery, req.Header, payloadHash, now)
	req.Header.Set("Authorization", signature)

	return req, nil
}

// generateSignatureV4 generates the Huawei OBS Signature V4. canonicalQuery is
// the pre-sorted, RFC 3986-encoded query string (may be empty) that is
// included in the canonical request so sub-resources such as "versioning" or
// "lifecycle" are covered by the signature.
func (b *bucket) generateSignatureV4(method, path, canonicalQuery string, headers http.Header, payloadHash string, timestamp time.Time) string {
	// Step 1: Create canonical request.
	canonicalHeaders := []string{
		fmt.Sprintf("host:%s", headers.Get("Host")),
		fmt.Sprintf("x-obs-content-sha256:%s", payloadHash),
		fmt.Sprintf("x-obs-date:%s", headers.Get("X-Obs-Date")),
	}

	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s",
		method,
		canonicalURI(path),
		canonicalQuery,
		strings.Join(canonicalHeaders, "\n")+"\n",
		"host;x-obs-content-sha256;x-obs-date", // signed headers
	)

	// Step 2: Create string to sign.
	dateStamp := timestamp.Format("20060102")
	credentialScope := fmt.Sprintf("%s/%s/obs/aws4_request", dateStamp, b.region)

	canonicalRequestHash := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s",
		timestamp.Format("20060102T150405Z"),
		credentialScope,
		hex.EncodeToString(canonicalRequestHash[:]),
	)

	// Step 3: Calculate signature.
	dateKey := hmacSHA256([]byte("AWS4"+b.secretKey), dateStamp)
	regionKey := hmacSHA256(dateKey, b.region)
	serviceKey := hmacSHA256(regionKey, "obs")
	signingKey := hmacSHA256(serviceKey, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))

	// Step 4: Build authorization header.
	return fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=host;x-obs-content-sha256;x-obs-date, Signature=%s",
		b.accessKeyID, credentialScope, signature)
}

// hmacSHA256 computes HMAC-SHA256.
func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

// canonicalQuery serializes query parameters in the form required by Signature
// V4: sorted keys (and values), RFC 3986-encoded name=value pairs joined by
// "&". OBS sub-resources use an empty value (e.g. "versioning=").
func canonicalQuery(query url.Values) string {
	if len(query) == 0 {
		return ""
	}
	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		values := query[k]
		if len(values) == 0 {
			values = []string{""}
		}
		sort.Strings(values)
		for _, v := range values {
			parts = append(parts, rfc3986Encode(k)+"="+rfc3986Encode(v))
		}
	}
	return strings.Join(parts, "&")
}

// rfc3986Encode percent-encodes s, keeping only the unreserved characters that
// are allowed verbatim in an RFC 3986 / Signature V4 canonical string.
func rfc3986Encode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if c := s[i]; isUnreserved(c) {
			b.WriteByte(c)
		} else {
			writePercent(&b, c)
		}
	}
	return b.String()
}

// canonicalURI returns the encoded form of the request target used by
// Signature V4. "/" separators are preserved and existing percent-encoded
// sequences pass through untouched, so segment-escaped object keys stay valid.
func canonicalURI(path string) string {
	if path == "" {
		return "/"
	}
	var b strings.Builder
	for i := 0; i < len(path); i++ {
		if path[i] == '%' && i+2 < len(path) && isHex(path[i+1]) && isHex(path[i+2]) {
			b.WriteString(path[i : i+3])
			i += 2
			continue
		}
		if path[i] == '/' {
			b.WriteByte('/')
			continue
		}
		if isUnreserved(path[i]) {
			b.WriteByte(path[i])
			continue
		}
		writePercent(&b, path[i])
	}
	return b.String()
}

func writePercent(b *strings.Builder, c byte) {
	const hexDigits = "0123456789ABCDEF"
	b.WriteByte('%')
	b.WriteByte(hexDigits[c>>4])
	b.WriteByte(hexDigits[c&0x0f])
}

func isUnreserved(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
		c == '-' || c == '_' || c == '.' || c == '~'
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// escapeKey does all required escaping for UTF-8 strings to work with OBS.
func escapeKey(key string) string {
	return escape.HexEscape(key, func(r []rune, i int) bool {
		c := r[i]
		switch {
		// OBS doesn't handle these characters (determined via experimentation).
		case c < 32:
			return true
		// For "../", escape the trailing slash.
		case i > 1 && c == '/' && r[i-1] == '.' && r[i-2] == '.':
			return true
		}
		return false
	})
}

// unescapeKey reverses escapeKey.
func unescapeKey(key string) string {
	return escape.HexUnescape(key)
}

// eTagToMD5 processes an ETag header and returns an MD5 hash if possible.
// OBS's ETag header is sometimes a quoted hexstring of the MD5. Other times,
// notably when the object was uploaded in multiple parts, it is not.
// We do the best we can.
func eTagToMD5(etag string) []byte {
	if etag == "" {
		// No header at all.
		return nil
	}
	// Strip the expected leading and trailing quotes.
	quoted := etag
	if len(quoted) < 2 || quoted[0] != '"' || quoted[len(quoted)-1] != '"' {
		return nil
	}
	unquoted := quoted[1 : len(quoted)-1]
	// Un-hex; we return nil on error. In particular, we'll get an error here
	// for multi-part uploaded blobs, whose ETag will contain a "-" and so will
	// never be a legal hex encoding.
	md5, err := hex.DecodeString(unquoted)
	if err != nil {
		return nil
	}
	return md5
}

func getSize(contentLength int64, contentRange string) int64 {
	// Default size to ContentLength, but that's incorrect for partial-length reads,
	// where ContentLength refers to the size of the returned Body, not the entire
	// size of the blob. ContentRange has the full size.
	size := contentLength
	if contentRange != "" {
		// Sample: bytes 10-14/27 (where 27 is the full size).
		parts := strings.Split(contentRange, "/")
		if len(parts) == 2 {
			if i, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
				size = i
			}
		}
	}
	return size
}

// parseHTTPTime parses an HTTP date header (Last-Modified), returning the zero
// time on failure or absence.
func parseHTTPTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := http.ParseTime(s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// parseListTime parses the OBS LastModified date as returned in list results
// (RFC 3339, often with milliseconds). It returns the zero time on failure.
func parseListTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// Versioning statuses accepted by EnableBucketVersioning.
const (
	// VersioningStatusEnabled enables versioning for the bucket.
	VersioningStatusEnabled = "Enabled"
	// VersioningStatusSuspended suspends versioning, keeping existing
	// versions while no new versions are created.
	VersioningStatusSuspended = "Suspended"
)

// Lifecycle rule statuses accepted by PutLifecycleConfiguration.
const (
	// LifecycleRuleStatusEnabled marks a lifecycle rule as active.
	LifecycleRuleStatusEnabled = "Enabled"
	// LifecycleRuleStatusDisabled keeps the rule but does not enforce it.
	LifecycleRuleStatusDisabled = "Disabled"
)

// VersioningConfiguration mirrors the OBS versioning configuration document
// (XML body of GET/PUT /?versioning).
type VersioningConfiguration struct {
	XMLName xml.Name `xml:"VersioningConfiguration"`
	Status  string   `xml:"Status"`
}

// LifecycleConfiguration mirrors the OBS lifecycle configuration document
// (XML body of GET/PUT /?lifecycle).
type LifecycleConfiguration struct {
	XMLName xml.Name        `xml:"LifecycleConfiguration"`
	Rules   []LifecycleRule `xml:"Rule"`
}

// LifecycleRule is a single lifecycle rule applied to a bucket.
type LifecycleRule struct {
	ID                             string                                   `xml:"ID"`
	Status                         string                                   `xml:"Status"`
	Filter                         *LifecycleRuleFilter                     `xml:"Filter,omitempty"`
	Transition                     *LifecycleTransition                     `xml:"Transition,omitempty"`
	Expiration                     *LifecycleExpiration                     `xml:"Expiration,omitempty"`
	NoncurrentVersionExpiration    *LifecycleNoncurrentVersionExpiration    `xml:"NoncurrentVersionExpiration,omitempty"`
	AbortIncompleteMultipartUpload *LifecycleAbortIncompleteMultipartUpload `xml:"AbortIncompleteMultipartUpload,omitempty"`
}

// LifecycleRuleFilter restricts which objects a rule applies to.
type LifecycleRuleFilter struct {
	Prefix string `xml:"Prefix,omitempty"`
}

// LifecycleTransition moves object versions to another storage class.
type LifecycleTransition struct {
	Days         int32  `xml:"Days,omitempty"`
	StorageClass string `xml:"StorageClass,omitempty"`
}

// LifecycleExpiration expires objects after a number of days.
type LifecycleExpiration struct {
	Days int32 `xml:"Days,omitempty"`
}

// LifecycleNoncurrentVersionExpiration expires noncurrent object versions.
type LifecycleNoncurrentVersionExpiration struct {
	NoncurrentDays int32 `xml:"NoncurrentDays,omitempty"`
}

// LifecycleAbortIncompleteMultipartUpload aborts incomplete multipart uploads.
type LifecycleAbortIncompleteMultipartUpload struct {
	DaysAfterInitiation int32 `xml:"DaysAfterInitiation,omitempty"`
}

// xmlHeaders returns the headers used for sub-resource configuration bodies.
func xmlHeaders() map[string]string {
	return map[string]string{"Content-Type": "application/xml"}
}

// EnableBucketVersioning sets the bucket versioning status
// (VersioningStatusEnabled or VersioningStatusSuspended) via the ?versioning
// sub-resource. Not part of upstream go-cloud/blob; provided by this patch.
func (b *bucket) EnableBucketVersioning(ctx context.Context, status string) error {
	if status != VersioningStatusEnabled && status != VersioningStatusSuspended {
		return fmt.Errorf("invalid OBS versioning status %q (must be %q or %q)",
			status, VersioningStatusEnabled, VersioningStatusSuspended)
	}
	body, err := xml.Marshal(VersioningConfiguration{Status: status})
	if err != nil {
		return fmt.Errorf("encoding versioning configuration: %w", err)
	}
	body = append([]byte(xml.Header), body...)

	req, err := b.newRequest(ctx, http.MethodPut, "/", url.Values{"versioning": {""}}, xmlHeaders(), body)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OBS enable versioning failed with status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// GetBucketVersioning reads the bucket versioning configuration via the
// ?versioning sub-resource. An empty configuration (never configured) is
// returned with a blank Status.
func (b *bucket) GetBucketVersioning(ctx context.Context) (*VersioningConfiguration, error) {
	req, err := b.newRequest(ctx, http.MethodGet, "/", url.Values{"versioning": {""}}, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OBS get versioning failed with status %d: %s", resp.StatusCode, string(respBody))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading versioning response: %w", err)
	}
	cfg := &VersioningConfiguration{}
	if len(bytes.TrimSpace(body)) == 0 {
		return cfg, nil
	}
	if err := xml.Unmarshal(body, cfg); err != nil {
		return nil, fmt.Errorf("decoding versioning configuration: %w", err)
	}
	return cfg, nil
}

// PutLifecycleConfiguration installs a lifecycle configuration on the bucket
// via the ?lifecycle sub-resource. config must contain at least one rule.
func (b *bucket) PutLifecycleConfiguration(ctx context.Context, config *LifecycleConfiguration) error {
	if config == nil || len(config.Rules) == 0 {
		return fmt.Errorf("lifecycle configuration must contain at least one rule")
	}
	for i := range config.Rules {
		status := config.Rules[i].Status
		if status != LifecycleRuleStatusEnabled && status != LifecycleRuleStatusDisabled {
			return fmt.Errorf("lifecycle rule %q has invalid status %q (must be %q or %q)",
				config.Rules[i].ID, status, LifecycleRuleStatusEnabled, LifecycleRuleStatusDisabled)
		}
	}
	body, err := xml.Marshal(config)
	if err != nil {
		return fmt.Errorf("encoding lifecycle configuration: %w", err)
	}
	body = append([]byte(xml.Header), body...)

	req, err := b.newRequest(ctx, http.MethodPut, "/", url.Values{"lifecycle": {""}}, xmlHeaders(), body)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OBS put lifecycle failed with status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// GetLifecycleConfiguration reads the bucket lifecycle configuration via the
// ?lifecycle sub-resource. Returns (nil, nil) when the bucket has no lifecycle
// configuration (OBS 404).
func (b *bucket) GetLifecycleConfiguration(ctx context.Context) (*LifecycleConfiguration, error) {
	req, err := b.newRequest(ctx, http.MethodGet, "/", url.Values{"lifecycle": {""}}, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OBS get lifecycle failed with status %d: %s", resp.StatusCode, string(respBody))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading lifecycle response: %w", err)
	}
	cfg := &LifecycleConfiguration{}
	if len(bytes.TrimSpace(body)) == 0 {
		return cfg, nil
	}
	if err := xml.Unmarshal(body, cfg); err != nil {
		return nil, fmt.Errorf("decoding lifecycle configuration: %w", err)
	}
	return cfg, nil
}