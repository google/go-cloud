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

// Package azureblob provides a blob implementation that uses Azure Storage’s BlockBlob. Use OpenBucket
// to construct a blob.Bucket.
//
// Open URLs
//
// For blob.Open URLs, azureblob registers for the protocol "azblob".
// The URL's Host is used as the bucket name.
//
// Authentication Options:
//
// Option 1: Use the Storage Account Name and Account Key (Primary or Secondary)
// This option requires the following environment variables to be set: AZURE_STORAGE_ACCOUNT_NAME, AZURE_STORAGE_ACCOUNT_KEY
//
// Option 2: Use a Shared Access Token (SASToken) for the Storage Account or Container
// This option requires the following environment variables to be set: AZURE_STORAGE_ACCOUNT_NAME, AZURE_STORAGE_SASTOKEN
// See documentation on Shared Access Signature: https://docs.microsoft.com/en-us/azure/storage/common/storage-dotnet-shared-access-signature-part-1#what-is-a-shared-access-signature
//
// The following query options are supported:
//  - backslashEscapeStr: Optional, defaults to forwardslash. Used to set the escape character for backslashes.
//
// Example URL:
//  azblob://mybucket?backslashEscapeStr=%2F
//
// As
//
// azureblob exposes the following types for As:
//  - Bucket: *azblob.BlockBlobURL
//  - Error: *azblob.StorageError
//  - ListObject: *azblob.BlobItem for objects, *azblob.BlobPrefix for "directories".
//  - ListOptions.BeforeList: Not Implemented
//  - Reader: *azblob.ContainerURL
//  - Attributes: *azblob.BlobGetPropertiesResponse
//  - WriterOptions.BeforeWrite: Not Implemented
package azureblob

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/driver"

	"github.com/Azure/azure-pipeline-go/pipeline"
	"github.com/Azure/azure-storage-blob-go/azblob"
)

type bucket struct {
	name             string
	credential       *azblob.SharedKeyCredential // used for SignedURL
	urls             serviceUrls
	pageMarkers      map[string]azblob.Marker // temporary page marker map, until azblob.Marker is an exportable type
	defaultDelimiter string                   // for escaping backslashes
}

type serviceUrls struct {
	serviceURL   *azblob.ServiceURL   // represents the Azure Storage Account
	containerURL *azblob.ContainerURL // represents the Azure Storage Container
	blockBlobURL *azblob.BlockBlobURL // represents the Azure Block Blob
}

// Settings to establish connection to Azure
type Settings struct {
	AccountName      string
	AccountKey       string
	DefaultDelimiter string
	SASToken         string
	Pipeline         pipeline.Pipeline
}

const (
	// BlobPathSeparator is used to escape backslashes
	BlobPathSeparator = "/"
	// OSPathSeparator or backslashes must be converted to forwardslashes
	OSPathSeparator = string(os.PathSeparator)
)

var (
	maxDownloadRetryRequests = 3               // download retry policy
	defaultPageSize          = 1000            // default page size for ListPaged
	defaultUploadBuffers     = 5               // configure the number of rotating buffers that are used when uploading
	defaultUploadBlockSize   = 8 * 1024 * 1024 //configure the upload buffer size
)

func init() {
	blob.Register("azblob", openURL)
}

func openURL(ctx context.Context, u *url.URL) (driver.Bucket, error) {
	q := u.Query()
	s := Settings{
		AccountName:      os.Getenv("AZURE_STORAGE_ACCOUNT_NAME"),
		AccountKey:       os.Getenv("AZURE_STORAGE_ACCOUNT_KEY"),
		SASToken:         os.Getenv("AZURE_STORAGE_SASTOKEN"),
		DefaultDelimiter: BlobPathSeparator,
	}

	if backslashEscapeStr := q["backslashEscapeStr"]; len(backslashEscapeStr) > 0 {
		s.DefaultDelimiter = backslashEscapeStr[0]
	}

	if s.SASToken != "" {
		return openBucketWithSASToken(ctx, &s, u.Host)
	} else {
		return openBucketWithAccountKey(ctx, &s, u.Host)
	}
}

// OpenBucket returns an Azure BlockBlob Bucket
func OpenBucket(ctx context.Context, settings *Settings, containerName string) (*blob.Bucket, error) {

	if settings.DefaultDelimiter == "" {
		settings.DefaultDelimiter = BlobPathSeparator
	}

	if settings.SASToken != "" {
		b, e := openBucketWithSASToken(ctx, settings, containerName)
		if e != nil {
			return nil, e
		}
		return blob.NewBucket(b), nil
	} else {
		b, e := openBucketWithAccountKey(ctx, settings, containerName)
		if e != nil {
			return nil, e
		}
		return blob.NewBucket(b), nil
	}
}

func openBucketWithSASToken(ctx context.Context, settings *Settings, containerName string) (driver.Bucket, error) {
	if settings.AccountName == "" {
		return nil, fmt.Errorf("azureblob: fail, settings.AccountName is not set")
	}
	if settings.SASToken == "" {
		return nil, fmt.Errorf("azureblob: fail, settings.SASToken is not set")
	}

	credential := azblob.NewAnonymousCredential()
	pipeline := settings.Pipeline
	if pipeline == nil {
		pipeline = azblob.NewPipeline(credential, azblob.PipelineOptions{})
	}

	blobURL := makeBlobStorageURL(settings.AccountName)
	blobURL.RawQuery = settings.SASToken

	serviceURL := azblob.NewServiceURL(*blobURL, pipeline)
	containerURL := serviceURL.NewContainerURL(containerName)

	return &bucket{
		name: containerName,
		urls: serviceUrls{
			serviceURL:   &serviceURL,
			containerURL: &containerURL,
		},
		pageMarkers:      map[string]azblob.Marker{},
		defaultDelimiter: settings.DefaultDelimiter,
	}, nil
}

func openBucketWithAccountKey(ctx context.Context, settings *Settings, containerName string) (driver.Bucket, error) {
	if settings.AccountName == "" {
		return nil, fmt.Errorf("azureblob: fail, settings.AccountName is not set")
	}

	if settings.AccountKey == "" {
		return nil, fmt.Errorf("azureblob: fail, settings.AccountKey is not set")
	}

	credential, _ := azblob.NewSharedKeyCredential(settings.AccountName, settings.AccountKey)
	pipeline := settings.Pipeline
	if pipeline == nil {
		pipeline = azblob.NewPipeline(credential, azblob.PipelineOptions{})
	}

	blobURL := makeBlobStorageURL(settings.AccountName)
	serviceURL := azblob.NewServiceURL(*blobURL, pipeline)
	containerURL := serviceURL.NewContainerURL(containerName)

	return &bucket{
		name:       containerName,
		credential: credential,
		urls: serviceUrls{
			serviceURL:   &serviceURL,
			containerURL: &containerURL,
		},
		pageMarkers:      map[string]azblob.Marker{},
		defaultDelimiter: settings.DefaultDelimiter,
	}, nil
}

func makeBlobStorageURL(accountName string) *url.URL {
	endpoint := fmt.Sprintf("https://%s.blob.core.windows.net", accountName)
	u, _ := url.Parse(endpoint)
	return u
}

func (b *bucket) Delete(ctx context.Context, key string) error {
	key = strings.Replace(key, OSPathSeparator, b.defaultDelimiter, -1)
	blobURL := b.urls.containerURL.NewBlockBlobURL(key)
	_, err := blobURL.Delete(ctx, azblob.DeleteSnapshotsOptionInclude, azblob.BlobAccessConditions{})

	if err != nil {
		return err
	}
	return nil
}

// reader reads an azblob. It implements io.ReadCloser.
type reader struct {
	body  io.ReadCloser
	attrs driver.ReaderAttributes
	raw   *azblob.BlockBlobURL
}

func (r *reader) Read(p []byte) (int, error) {
	return r.body.Read(p)
}
func (r *reader) Close() error {
	return r.body.Close()
}
func (r *reader) Attributes() driver.ReaderAttributes {
	return r.attrs
}
func (r *reader) As(i interface{}) bool {
	p, ok := i.(*azblob.BlockBlobURL)
	if !ok {
		return false
	}
	*p = *r.raw
	return true
}

// NewRangeReader implements driver.NewRangeReader.
func (b *bucket) NewRangeReader(ctx context.Context, key string, offset, length int64, opts *driver.ReaderOptions) (driver.Reader, error) {
	key = strings.Replace(key, OSPathSeparator, b.defaultDelimiter, -1)
	blockBlobURL := b.urls.containerURL.NewBlockBlobURL(key)

	end := length
	if end < 0 {
		end = azblob.CountToEnd
	}

	blobDownloadResponse, err := blockBlobURL.Download(ctx, offset, end, azblob.BlobAccessConditions{}, false)
	if err != nil {
		return nil, err
	}

	blobPropertiesResponse, _ := blockBlobURL.GetProperties(ctx, azblob.BlobAccessConditions{})

	return &reader{
		body: blobDownloadResponse.Body(azblob.RetryReaderOptions{MaxRetryRequests: maxDownloadRetryRequests}),
		attrs: driver.ReaderAttributes{
			ContentType: blobPropertiesResponse.ContentType(),
			Size:        blobPropertiesResponse.ContentLength(),
			ModTime:     blobPropertiesResponse.LastModified(),
		},
		raw: &blockBlobURL}, nil
}

func (b *bucket) As(i interface{}) bool {
	p, ok := i.(*azblob.ContainerURL)
	if !ok {
		return false
	}
	*p = *b.urls.containerURL
	return true
}

func (b *bucket) ErrorAs(err error, i interface{}) bool {
	switch v := err.(type) {
	case azblob.StorageError:
		if p, ok := i.(*azblob.StorageError); ok {
			*p = v
			return true
		}
	}
	return false
}

func (b *bucket) IsNotExist(err error) bool {
	if serr, ok := err.(azblob.StorageError); ok {
		// Check and fail both the SDK ServiceCode and the Http Response Code for NotFound
		if serr.ServiceCode() == azblob.ServiceCodeBlobNotFound || serr.Response().StatusCode == 404 {
			return true
		}
	}
	return false
}

func (b *bucket) IsNotImplemented(err error) bool {
	return false
}

func (b *bucket) Attributes(ctx context.Context, key string) (driver.Attributes, error) {

	key = strings.Replace(key, OSPathSeparator, b.defaultDelimiter, -1)
	blockBlobURL := b.urls.containerURL.NewBlockBlobURL(key)
	blobPropertiesResponse, err := blockBlobURL.GetProperties(ctx, azblob.BlobAccessConditions{})

	if err != nil {
		return driver.Attributes{}, err
	}

	return driver.Attributes{
		ContentType: blobPropertiesResponse.ContentType(),
		Size:        blobPropertiesResponse.ContentLength(),
		ModTime:     blobPropertiesResponse.LastModified(),
		Metadata:    blobPropertiesResponse.NewMetadata(),
		AsFunc: func(i interface{}) bool {
			p, ok := i.(*azblob.BlobGetPropertiesResponse)
			if !ok {
				return false
			}
			*p = *blobPropertiesResponse
			return true
		},
	}, nil
}

func (b *bucket) ListPaged(ctx context.Context, opts *driver.ListOptions) (*driver.ListPage, error) {
	pageSize := opts.PageSize
	if pageSize == 0 {
		pageSize = defaultPageSize
	}

	marker := azblob.Marker{}
	if len(opts.PageToken) > 0 {
		if m, ok := b.pageMarkers[string(opts.PageToken)]; ok {
			marker = m
		}
	}

	opts.Prefix = strings.Replace(opts.Prefix, OSPathSeparator, b.defaultDelimiter, -1)

	listBlob, err := b.urls.containerURL.ListBlobsHierarchySegment(ctx, marker, opts.Delimiter, azblob.ListBlobsSegmentOptions{
		MaxResults: int32(pageSize),
		Prefix:     opts.Prefix,
	})

	if err != nil {
		return nil, err
	}

	page := &driver.ListPage{}
	page.Objects = []*driver.ListObject{}
	for _, blobPrefix := range listBlob.Segment.BlobPrefixes {
		page.Objects = append(page.Objects, &driver.ListObject{
			Key:   blobPrefix.Name,
			Size:  0,
			IsDir: true,
			AsFunc: func(i interface{}) bool {
				p, ok := i.(*azblob.BlobPrefix)
				if !ok {
					return false
				}
				*p = blobPrefix
				return true
			}})
	}

	for _, blobInfo := range listBlob.Segment.BlobItems {
		page.Objects = append(page.Objects, &driver.ListObject{
			Key:     blobInfo.Name,
			ModTime: blobInfo.Properties.LastModified,
			Size:    *blobInfo.Properties.ContentLength,
			IsDir:   false,
			AsFunc: func(i interface{}) bool {
				p, ok := i.(*azblob.BlobItem)
				if !ok {
					return false
				}
				*p = blobInfo
				return true
			},
		})
	}

	if listBlob.NextMarker.NotDone() {
		token := uuid.New().String()
		b.pageMarkers[token] = listBlob.NextMarker
		page.NextPageToken = []byte(token)
	}

	if len(listBlob.Segment.BlobPrefixes) > 0 && len(listBlob.Segment.BlobItems) > 0 {
		sort.Slice(page.Objects, func(i, j int) bool {
			return page.Objects[i].Key < page.Objects[j].Key
		})
	}

	return page, nil
}

func (b *bucket) SignedURL(ctx context.Context, key string, opts *driver.SignedURLOptions) (string, error) {

	key = strings.Replace(key, OSPathSeparator, b.defaultDelimiter, -1)

	blockBlobURL := b.urls.containerURL.NewBlobURL(key)
	srcBlobParts := azblob.NewBlobURLParts(blockBlobURL.URL())
	var err error
	srcBlobParts.SAS, err = azblob.BlobSASSignatureValues{
		Protocol:      azblob.SASProtocolHTTPS,
		ExpiryTime:    time.Now().UTC().Add(opts.Expiry),
		ContainerName: b.name,
		BlobName:      key,
		Permissions:   azblob.BlobSASPermissions{Add: true, Create: true, Delete: true, Read: true, Write: true}.String(),
	}.NewSASQueryParameters(b.credential)

	if err != nil {
		return "", err
	}

	srcBlobURLWithSAS := srcBlobParts.URL()
	return srcBlobURLWithSAS.String(), nil
}

type writer struct {
	ctx         context.Context
	urls        *serviceUrls
	key         string
	contentType string
	opts        *driver.WriterOptions

	w     *io.PipeWriter
	donec chan struct{}
	err   error
}

// NewTypedWriter implements driver.NewTypedWriter.
func (b *bucket) NewTypedWriter(ctx context.Context, key string, contentType string, opts *driver.WriterOptions) (driver.Writer, error) {
	containerURL := b.urls.serviceURL.NewContainerURL(b.name)
	key = strings.Replace(key, OSPathSeparator, b.defaultDelimiter, -1)
	blockBlobURL := containerURL.NewBlockBlobURL(key)

	if opts.Metadata == nil {
		opts.Metadata = map[string]string{}
	}
	if opts.BufferSize == 0 {
		opts.BufferSize = defaultUploadBlockSize
	}

	w := &writer{
		ctx:         ctx,
		key:         key,
		contentType: contentType,
		opts:        opts,
		urls: &serviceUrls{
			serviceURL:   b.urls.serviceURL,
			containerURL: &containerURL,
			blockBlobURL: &blockBlobURL,
		},
		donec: make(chan struct{}),
	}

	return w, nil
}

// Write appends p to w. User must call Close to close the w after done writing.
func (w *writer) Write(p []byte) (int, error) {
	if w.w == nil {
		if err := w.open(); err != nil {
			return 0, err
		}
	}

	select {
	case <-w.donec:
		return 0, w.err
	default:
	}

	return w.w.Write(p)
}

func (w *writer) open() error {
	pr, pw := io.Pipe()
	w.w = pw

	go func() {
		defer close(w.donec)

		var blobHTTPHeaders = azblob.BlobHTTPHeaders{}
		if w.contentType != "" {
			blobHTTPHeaders.ContentType = w.contentType
		}
		if len(w.opts.ContentMD5) > 0 {
			blobHTTPHeaders.ContentMD5 = w.opts.ContentMD5
		}

		buf, err := ioutil.ReadAll(pr)
		if err != nil {
			w.err = err
			pr.CloseWithError(err)
			return
		}

		uploadOpts := azblob.UploadStreamToBlockBlobOptions{BufferSize: w.opts.BufferSize, MaxBuffers: defaultUploadBuffers, Metadata: w.opts.Metadata, BlobHTTPHeaders: blobHTTPHeaders}
		_, err = azblob.UploadStreamToBlockBlob(w.ctx, bytes.NewReader(buf), *w.urls.blockBlobURL, uploadOpts)
		w.err = err
		
		if err != nil {
			w.err = err
			pr.CloseWithError(err)
			return
		}
	}()

	return nil
}

// Close completes the writer and close it. Any error occuring during write will
// be returned. If a writer is closed before any Write is called, Close will
// create an empty file at the given key.
func (w *writer) Close() error {

	select {
	case <-w.ctx.Done():
		return w.ctx.Err()

	default:
		if w.w == nil {
			w.touch()
		} else if err := w.w.Close(); err != nil {
			return err
		}

		<-w.donec

		return w.err
	}
}

// touch creates an empty object in the bucket. It is called if user creates a
// new writer but never calls write before closing it.
func (w *writer) touch() {
	if w.w != nil {
		return
	}
	defer close(w.donec)

	blobHTTPHeaders := azblob.BlobHTTPHeaders{}
	uploadOpts := azblob.UploadStreamToBlockBlobOptions{BufferSize: w.opts.BufferSize, MaxBuffers: defaultUploadBuffers, Metadata: w.opts.Metadata, BlobHTTPHeaders: blobHTTPHeaders}
	_, err := azblob.UploadStreamToBlockBlob(w.ctx, ioutil.NopCloser(strings.NewReader("")), *w.urls.blockBlobURL, uploadOpts)

	w.err = err
}
