
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
	"sync"
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
	SASToken         string
	Pipeline         pipeline.Pipeline
	DefaultDelimiter DefaultBlobDelimiter
}

// DefaultBlobDelimiter is used to escape backslashes
type DefaultBlobDelimiter struct {
	Value string
}

const (
	// BlobPathSeparator is used to escape backslashes
	BlobPathSeparator = "/"
	// OSPathSeparator or backslashes must be converted to forwardslashes
	OSPathSeparator = string(os.PathSeparator)
)

var (
	maxDownloadRetryRequests = 3    // download retry policy
	maxPageSize              = 5000 // default page size for ListPaged
)

// OpenBucket returns an Azure BlockBlob Bucket
func OpenBucket(ctx context.Context, settings *Settings, containerName string) (*blob.Bucket, error) {

	if settings.DefaultDelimiter.Value == "" {
		settings.DefaultDelimiter.Value = BlobPathSeparator
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
		return nil, fmt.Errorf("Settings.AccountName is not set")
	}
	if settings.SASToken == "" {
		return nil, fmt.Errorf("Settings.SASToken is not set")
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
		name:             containerName,		
		urls: serviceUrls{
			serviceURL:   &serviceURL,
			containerURL: &containerURL,
		},
		pageMarkers:      make(map[string]azblob.Marker),
		defaultDelimiter: settings.DefaultDelimiter.Value,
	}, nil
}

func openBucketWithAccountKey(ctx context.Context, settings *Settings, containerName string) (driver.Bucket, error) {
	if settings.AccountName == "" {
		return nil, fmt.Errorf("Settings.AccountName is not set")
	}

	if settings.AccountKey == "" {
		return nil, fmt.Errorf("Settings.AccountKey is not set")
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
		name:             containerName,
		credential:       credential,		
		urls: serviceUrls{
			serviceURL:   &serviceURL,
			containerURL: &containerURL,
		},
		pageMarkers:      make(map[string]azblob.Marker),
		defaultDelimiter: settings.DefaultDelimiter.Value,
	}, nil
}

func makeBlobStorageURL(accountName string) *url.URL {
	endpoint := fmt.Sprintf("https://%s.blob.core.windows.net", accountName)
	u, _ := url.Parse(endpoint)
	return u
}

func (b *bucket) Delete(ctx context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("Invalid/Empty Key")
	}

	key = strings.Replace(key, OSPathSeparator, b.defaultDelimiter, -1)

	blobURL := b.urls.containerURL.NewBlockBlobURL(key)
	_, err := blobURL.Delete(ctx, azblob.DeleteSnapshotsOptionInclude, azblob.BlobAccessConditions{})

	if err != nil {
		return makeAzureError(err, b.name, key)
	} else {
		return nil
	}
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

	if key == "" {
		return nil, fmt.Errorf("Invalid/Empty Key")
	}

	key = strings.Replace(key, OSPathSeparator, b.defaultDelimiter, -1)

	blockBlobURL := b.urls.containerURL.NewBlockBlobURL(key)
	blobPropertiesResponse, err := blockBlobURL.GetProperties(ctx, azblob.BlobAccessConditions{})

	if err != nil {
		return nil, makeAzureError(err, b.name, key)
	}

	// determine content end for reader
	end := length
	if end < 0 {
		end = blobPropertiesResponse.ContentLength()

		if offset > blobPropertiesResponse.ContentLength() {
			return nil, azureError{bucket: b.name, key: key, msg: "offset cannot be greater than length"}
		}
	}

	// return content reader
	if end > 0 {
		blobDownloadResponse, err := blockBlobURL.Download(ctx, offset, end, azblob.BlobAccessConditions{}, false)

		if err != nil {
			return nil, makeAzureError(err, b.name, key)
		}

		return &reader{
			body: blobDownloadResponse.Body(azblob.RetryReaderOptions{MaxRetryRequests: maxDownloadRetryRequests}),
			attrs: driver.ReaderAttributes{
				ContentType: blobPropertiesResponse.ContentType(),
				Size:        blobPropertiesResponse.ContentLength(),
				ModTime:     blobPropertiesResponse.LastModified(),
			},
			raw: &blockBlobURL}, nil
	}

	// return metadata reader
	emptyReader := ioutil.NopCloser(strings.NewReader(""))
	return &reader{
		body: emptyReader,
		attrs: driver.ReaderAttributes{
			ContentType: blobPropertiesResponse.ContentType(),
			Size:        blobPropertiesResponse.ContentLength(),
			ModTime:     blobPropertiesResponse.LastModified(),
		},
		raw: nil}, nil
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
	// check StorageError error code
	if serr, ok := err.(azblob.StorageError); ok {
		switch serr.ServiceCode() {
		case azblob.ServiceCodeBlobNotFound:
			return true
		default:
			// test the http status code for 404/notfound
			errorStatusCode := serr.Response().StatusCode
			if errorStatusCode == 404 {
				return true
			}
		}
	}

	// check azureError code code
	if aerr, ok := err.(azureError); ok {
		if aerr.errorCode == 404 {
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
		return driver.Attributes{}, makeAzureError(err, b.name, key)
	}

	metadata := blobPropertiesResponse.NewMetadata()

	return driver.Attributes{
		ContentType: blobPropertiesResponse.ContentType(),
		Size:        blobPropertiesResponse.ContentLength(),
		ModTime:     blobPropertiesResponse.LastModified(),
		Metadata:    metadata,
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
		pageSize = maxPageSize
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
		return nil, makeAzureError(err, b.name, "")
	}

	resultCount := len(listBlob.Segment.BlobPrefixes) + len(listBlob.Segment.BlobItems)

	page := &driver.ListPage{}
	page.Objects = make([]*driver.ListObject, resultCount)
	for idx, blobPrefix := range listBlob.Segment.BlobPrefixes {
		page.Objects[idx] = &driver.ListObject{
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
			},
		}
	}
	for idx, blobInfo := range listBlob.Segment.BlobItems {
		page.Objects[idx+len(listBlob.Segment.BlobPrefixes)] = &driver.ListObject{
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
		}
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
		return "", makeAzureError(err, b.name, key)
	}

	srcBlobURLWithSAS := srcBlobParts.URL()
	return srcBlobURLWithSAS.String(), nil
}

type writer struct {
	ctx         context.Context
	urls        *serviceUrls
	key         string
	contentType string
	blockIDs    []string
	mux         sync.Mutex
	writerOpts  *driver.WriterOptions
}

// NewTypedWriter implements driver.NewTypedWriter.
func (b *bucket) NewTypedWriter(ctx context.Context, key string, contentType string, opts *driver.WriterOptions) (driver.Writer, error) {

	if key == "" {
		return nil, fmt.Errorf("Invalid/Empty Key")
	}

	containerURL := b.urls.serviceURL.NewContainerURL(b.name)	
	key = strings.Replace(key, OSPathSeparator, b.defaultDelimiter, -1)
	blockBlobURL := containerURL.NewBlockBlobURL(key)

	var blockIDs []string
	w := &writer{
		ctx:         ctx,
		key:         key,
		contentType: contentType,
		writerOpts:  opts,
		urls: &serviceUrls{
			serviceURL:   b.urls.serviceURL,
			containerURL: &containerURL,
			blockBlobURL: &blockBlobURL,
		},
		blockIDs: blockIDs,
	}

	return w, nil
}

// Write creates a stated block for incoming buffer (p)
// Each call to Write will append to the blockId list for final commit in w.Close()
func (w *writer) Write(p []byte) (int, error) {
	chunks := split(p, azblob.BlockBlobMaxStageBlockBytes)
	var wg sync.WaitGroup
	wg.Add(len(chunks))

	for _, chunk := range chunks {
		var index = len(w.blockIDs) + 1
		blockID := BlockIDIntToBase64(index)
		w.blockIDs = append(w.blockIDs, blockID)

		go func(c []byte, bid string) {
			defer wg.Done()
			w.urls.blockBlobURL.StageBlock(w.ctx, bid, bytes.NewReader(c), azblob.LeaseAccessConditions{}, w.writerOpts.ContentMD5)
		}(chunk, blockID)
	}
	wg.Wait()

	return len(p), nil
}

// credits to xlab/bytes_split.go
func split(buf []byte, lim int) [][]byte {
	var chunk []byte
	chunks := make([][]byte, 0, len(buf)/lim+1)
	for len(buf) >= lim {
		chunk, buf = buf[:lim], buf[lim:]
		chunks = append(chunks, chunk)
	}
	if len(buf) > 0 {
		chunks = append(chunks, buf)
	}
	return chunks
}

// Close completes the writer and close it. Any error occurring during write will
// be returned. If a writer is closed before any Write is called, Close will
// create an empty file at the given key.
func (w *writer) Close() error {
	select {
	case <-w.ctx.Done():				
		return w.ctx.Err()
	default:
		metaData := azblob.Metadata{}
		if w.writerOpts != nil {
			if len(w.writerOpts.Metadata) > 0 {
				metaData = w.writerOpts.Metadata
			}
		}
		_, err := w.urls.blockBlobURL.CommitBlockList(w.ctx, w.blockIDs, azblob.BlobHTTPHeaders{}, metaData, azblob.BlobAccessConditions{})
		if err == nil && w.contentType != "" {
			var basicHeaders = azblob.BlobHTTPHeaders{
				ContentType: w.contentType,
			}
			w.urls.blockBlobURL.SetHTTPHeaders(w.ctx, basicHeaders, azblob.BlobAccessConditions{})
		}

		return err
	}
}

type azureError struct {
	bucket, key, msg string
	errorCode        int
}

func (e azureError) Error() string {
	return fmt.Sprintf("azure://%s/%s: %s", e.bucket, e.key, e.msg)
}
func makeAzureError(err error, bucketName string, key string) azureError {
	aerr := azureError{bucket: bucketName, key: key, msg: err.Error()}

	if serr, ok := err.(azblob.StorageError); ok {
		switch serr.ServiceCode() {
		case azblob.ServiceCodeBlobNotFound:
			aerr.errorCode = 404 // NotFound
		default:
			// use http response status code
			statusCode := serr.Response().StatusCode
			aerr.errorCode = statusCode
		}
	}

	return aerr
}
