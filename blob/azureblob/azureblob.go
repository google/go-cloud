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

// Package azureblob provides a blob implementation that uses Azure Storage’s
// BlockBlob. Use OpenBucket to construct a *blob.Bucket.
//
// Open URLs
//
// For blob.Open URLs, azureblob registers for the scheme "azblob"; URLs start
// with "azblob://".
//
// The URL's Host is used as the bucket name.
//
// By default, credentials are retrieved from the environment variables
// AZURE_STORAGE_ACCOUNT, AZURE_STORAGE_KEY, and AZURE_STORAGE_SAS_TOKEN.
// AZURE_STORAGE_ACCOUNT is required, along with one of the other two. See
// https://docs.microsoft.com/en-us/azure/storage/common/storage-dotnet-shared-access-signature-part-1#what-is-a-shared-access-signature
// for more on SAS tokens. Alternatively, credentials can be loaded from a file;
// see the cred_path query parameter below.
//
// The following query options are supported:
//  - cred_path: Sets path to a credentials file in JSON format. The
//    AccountName field must be specified, and either AccountKey or SASToken.
// Example credentials file using AccountKey:
//     {
//       "AccountName": "STORAGE ACCOUNT NAME",
//       "AccountKey": "PRIMARY OR SECONDARY ACCOUNT KEY"
//     }
// Example credentials file using SASToken:
//     {
//       "AccountName": "STORAGE ACCOUNT NAME",
//       "SASToken": "ENTER YOUR AZURE STORAGE SAS TOKEN"
//     }
// Example URL:
//  azblob://mybucket?cred_path=pathToCredentials
//
// As
//
// azureblob exposes the following types for As:
//  - Bucket: *azblob.ContainerURL
//  - Error: azblob.StorageError
//  - ListObject: azblob.BlobItem for objects, azblob.BlobPrefix for "directories"
//  - ListOptions.BeforeList: *azblob.ListBlobsSegmentOptions
//  - Reader: azblob.DownloadResponse
//  - Attributes: azblob.BlobGetPropertiesResponse
//  - WriterOptions.BeforeWrite: *azblob.UploadStreamToBlockBlobOptions
package azureblob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-pipeline-go/pipeline"
	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/google/uuid"
	"github.com/google/wire"
	"gocloud.dev/blob"
	"gocloud.dev/blob/driver"

	"gocloud.dev/internal/useragent"
)

// Options sets options for constructing a *blob.Bucket backed by Azure Block Blob.
type Options struct {
	// Credential represents the authorizer for SignedURL.
	// Required to use SignedURL.
	Credential *azblob.SharedKeyCredential

	// SASToken can be provided along with anonymous credentials to use
	// delegated privileges.
	// See https://docs.microsoft.com/en-us/azure/storage/common/storage-dotnet-shared-access-signature-part-1#shared-access-signature-parameters.
	SASToken SASToken
}

// Azure does not handle backslashes in the blob key well. As a workaround, all
// backslashes are converted to forward slashes during bucket operations.
// This is needed to ensure directories from Windows file systems are
// represented correctly in Azure Storage.
//
// For example, the Windows path C:\Users\UserName\Test.json is converted
// to C:/Users/UserName/Test.json. This retains the original directory structure
// in Azure Storage as C:/Users/UserName/Test.json, where forwardslash
// represents the virtual directory
//
// For more naming rules and limitations see
// https://docs.microsoft.com/en-us/rest/api/storageservices/naming-and-referencing-containers--blobs--and-metadata
const (
	// blobPathSeparator is the replacement for backslashPathSeparator.
	blobPathSeparator = "/"
	// backslashPathSeparator are converted to blobPathSeparator.
	backslashPathSeparator = "\\"
)

const (
	defaultMaxDownloadRetryRequests = 3               // download retry policy (Azure default is zero)
	defaultPageSize                 = 1000            // default page size for ListPaged (Azure default is 5000)
	defaultUploadBuffers            = 5               // configure the number of rotating buffers that are used when uploading (for degree of parallelism)
	defaultUploadBlockSize          = 8 * 1024 * 1024 // configure the upload buffer size
)

func init() {
	blob.Register("azblob", openURL)
}

func openURL(ctx context.Context, u *url.URL) (driver.Bucket, error) {
	type AzureCreds struct {
		AccountName AccountName
		AccountKey  AccountKey
		SASToken    SASToken
	}
	ac := AzureCreds{}
	if credPath := u.Query()["cred_path"]; len(credPath) > 0 {
		f, err := ioutil.ReadFile(credPath[0])
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(f, &ac)
		if err != nil {
			return nil, err
		}
	} else {
		// Use default credential info from the environment.
		// Ignore errors, as we'll get errors from OpenBucket later.
		ac.AccountName, _ = DefaultAccountName()
		ac.AccountKey, _ = DefaultAccountKey()
		ac.SASToken, _ = DefaultSASToken()
	}

	// azblob.Credential is an interface; we will use either a SharedKeyCredential
	// or anonymous credentials. If the former, we will also fill in
	// Options.Credential so that SignedURL will work.
	var credential azblob.Credential
	var sharedKeyCred *azblob.SharedKeyCredential
	if ac.AccountKey != "" {
		var err error
		sharedKeyCred, err = NewCredential(ac.AccountName, ac.AccountKey)
		if err != nil {
			return nil, err
		}
		credential = sharedKeyCred
	} else {
		credential = azblob.NewAnonymousCredential()
	}
	pipeline := NewPipeline(credential, azblob.PipelineOptions{})
	return openBucket(ctx, pipeline, ac.AccountName, u.Host, &Options{
		Credential: sharedKeyCred,
		SASToken:   ac.SASToken,
	})
}

// DefaultIdentity is a Wire provider set that provides an Azure storage
// account name, key, and SharedKeyCredential from environment variables.
var DefaultIdentity = wire.NewSet(
	DefaultAccountName,
	DefaultAccountKey,
	NewCredential,
	wire.Bind(new(azblob.Credential), new(azblob.SharedKeyCredential)),
	wire.Value(azblob.PipelineOptions{}),
)

// SASTokenIdentity is a Wire provider set that provides an Azure storage
// account name, SASToken, and anonymous credential from environment variables.
var SASTokenIdentity = wire.NewSet(
	DefaultAccountName,
	DefaultSASToken,
	azblob.NewAnonymousCredential,
	wire.Value(azblob.PipelineOptions{}),
)

// AccountName is an Azure storage account name.
type AccountName string

// AccountKey is an Azure storage account key (primary or secondary).
type AccountKey string

// SASToken is an Azure shared access signature.
// https://docs.microsoft.com/en-us/azure/storage/common/storage-dotnet-shared-access-signature-part-1
type SASToken string

// DefaultAccountName loads the Azure storage account name from the
// AZURE_STORAGE_ACCOUNT environment variable.
func DefaultAccountName() (AccountName, error) {
	s := os.Getenv("AZURE_STORAGE_ACCOUNT")
	if s == "" {
		return "", errors.New("azureblob: environment variable AZURE_STORAGE_ACCOUNT not set")
	}
	return AccountName(s), nil
}

// DefaultAccountKey loads the Azure storage account key (primary or secondary)
// from the AZURE_STORAGE_KEY environment variable.
func DefaultAccountKey() (AccountKey, error) {
	s := os.Getenv("AZURE_STORAGE_KEY")
	if s == "" {
		return "", errors.New("azureblob: environment variable AZURE_STORAGE_KEY not set")
	}
	return AccountKey(s), nil
}

// DefaultSASToken loads a Azure SAS token from the AZURE_STORAGE_SAS_TOKEN
// environment variable.
func DefaultSASToken() (SASToken, error) {
	s := os.Getenv("AZURE_STORAGE_SAS_TOKEN")
	if s == "" {
		return "", errors.New("azureblob: environment variable AZURE_STORAGE_SAS_TOKEN not set")
	}
	return SASToken(s), nil
}

// NewCredential creates a SharedKeyCredential.
func NewCredential(accountName AccountName, accountKey AccountKey) (*azblob.SharedKeyCredential, error) {
	return azblob.NewSharedKeyCredential(string(accountName), string(accountKey))
}

// NewPipeline creates a Pipeline for making HTTP requests to Azure.
func NewPipeline(credential azblob.Credential, opts azblob.PipelineOptions) pipeline.Pipeline {
	opts.Telemetry.Value = useragent.AzureUserAgentPrefix("blob") + opts.Telemetry.Value
	return azblob.NewPipeline(credential, opts)
}

// bucket represents a Azure Storage Account Container, which handles read,
// write and delete operations on objects within it.
// See https://docs.microsoft.com/en-us/azure/storage/blobs/storage-blobs-introduction.
type bucket struct {
	name         string
	pageMarkers  map[string]azblob.Marker
	serviceURL   *azblob.ServiceURL
	containerURL azblob.ContainerURL
	opts         *Options
}

// OpenBucket returns a *blob.Bucket backed by Azure Storage Account. See the package
// documentation for an example and
// https://godoc.org/github.com/Azure/azure-storage-blob-go/azblob
// for more details.
func OpenBucket(ctx context.Context, pipeline pipeline.Pipeline, accountName AccountName, containerName string, opts *Options) (*blob.Bucket, error) {
	b, err := openBucket(ctx, pipeline, accountName, containerName, opts)
	if err != nil {
		return nil, err
	}
	return blob.NewBucket(b), nil
}

func openBucket(ctx context.Context, pipeline pipeline.Pipeline, accountName AccountName, containerName string, opts *Options) (*bucket, error) {
	if pipeline == nil {
		return nil, errors.New("azureblob.OpenBucket: pipeline is required")
	}
	if accountName == "" {
		return nil, errors.New("azureblob.OpenBucket: accountName is required")
	}
	if containerName == "" {
		return nil, errors.New("azureblob.OpenBucket: containerName is required")
	}
	if opts == nil {
		opts = &Options{}
	}
	blobURL, err := url.Parse(fmt.Sprintf("https://%s.blob.core.windows.net", accountName))
	if err != nil {
		return nil, err
	}
	serviceURL := azblob.NewServiceURL(*blobURL, pipeline)
	if opts.SASToken != "" {
		blobURL.RawQuery = string(opts.SASToken)
	}
	return &bucket{
		name:         containerName,
		pageMarkers:  map[string]azblob.Marker{},
		serviceURL:   &serviceURL,
		containerURL: serviceURL.NewContainerURL(containerName),
		opts:         opts,
	}, nil
}

// blockBlobURL replaces backslashes in key and returns an azblob.BlockBlobURL
// for it.
func (b *bucket) blockBlobURL(key string) azblob.BlockBlobURL {
	key = strings.Replace(key, backslashPathSeparator, blobPathSeparator, -1)
	return b.containerURL.NewBlockBlobURL(key)
}

// Delete implements driver.Delete.
func (b *bucket) Delete(ctx context.Context, key string) error {
	blockBlobURL := b.blockBlobURL(key)
	_, err := blockBlobURL.Delete(ctx, azblob.DeleteSnapshotsOptionInclude, azblob.BlobAccessConditions{})
	return err
}

// reader reads an azblob. It implements io.ReadCloser.
type reader struct {
	body  io.ReadCloser
	attrs driver.ReaderAttributes
	raw   *azblob.DownloadResponse
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
	p, ok := i.(*azblob.DownloadResponse)
	if !ok {
		return false
	}
	*p = *r.raw
	return true
}

// NewRangeReader implements driver.NewRangeReader.
func (b *bucket) NewRangeReader(ctx context.Context, key string, offset, length int64, opts *driver.ReaderOptions) (driver.Reader, error) {
	blockBlobURL := b.blockBlobURL(key)

	end := length
	if end < 0 {
		end = azblob.CountToEnd
	}

	blobDownloadResponse, err := blockBlobURL.Download(ctx, offset, end, azblob.BlobAccessConditions{}, false)
	if err != nil {
		return nil, err
	}
	attrs := driver.ReaderAttributes{
		ContentType: blobDownloadResponse.ContentType(),
		Size:        getSize(blobDownloadResponse.ContentLength(), blobDownloadResponse.ContentRange()),
		ModTime:     blobDownloadResponse.LastModified(),
	}
	var body io.ReadCloser
	if length == 0 {
		body = http.NoBody
	} else {
		body = blobDownloadResponse.Body(azblob.RetryReaderOptions{MaxRetryRequests: defaultMaxDownloadRetryRequests})
	}
	return &reader{
		body:  body,
		attrs: attrs,
		raw:   blobDownloadResponse,
	}, nil
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

// As implements driver.As.
func (b *bucket) As(i interface{}) bool {
	p, ok := i.(**azblob.ContainerURL)
	if !ok {
		return false
	}
	*p = &b.containerURL
	return true
}

// As implements driver.ErrorAs.
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

// IsNotExist implements driver.IsNotExist.
func (b *bucket) IsNotExist(err error) bool {
	if serr, ok := err.(azblob.StorageError); ok {
		// Check and fail both the SDK ServiceCode and the Http Response Code for NotFound
		if serr.ServiceCode() == azblob.ServiceCodeBlobNotFound || serr.Response().StatusCode == 404 {
			return true
		}
	}
	return false
}

// IsNotImplemented implements driver.IsNotImplemented.
func (b *bucket) IsNotImplemented(err error) bool {
	return false
}

// Attributes implements driver.Attributes.
func (b *bucket) Attributes(ctx context.Context, key string) (driver.Attributes, error) {
	blockBlobURL := b.blockBlobURL(key)
	blobPropertiesResponse, err := blockBlobURL.GetProperties(ctx, azblob.BlobAccessConditions{})
	if err != nil {
		return driver.Attributes{}, err
	}
	return driver.Attributes{
		CacheControl:       blobPropertiesResponse.CacheControl(),
		ContentDisposition: blobPropertiesResponse.ContentDisposition(),
		ContentEncoding:    blobPropertiesResponse.ContentEncoding(),
		ContentLanguage:    blobPropertiesResponse.ContentLanguage(),
		ContentType:        blobPropertiesResponse.ContentType(),
		Size:               blobPropertiesResponse.ContentLength(),
		MD5:                blobPropertiesResponse.ContentMD5(),
		ModTime:            blobPropertiesResponse.LastModified(),
		Metadata:           blobPropertiesResponse.NewMetadata(),
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

// ListPaged implements driver.ListPaged.
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

	opts.Prefix = strings.Replace(opts.Prefix, backslashPathSeparator, blobPathSeparator, -1)
	azOpts := azblob.ListBlobsSegmentOptions{
		MaxResults: int32(pageSize),
		Prefix:     opts.Prefix,
	}
	if opts.BeforeList != nil {
		asFunc := func(i interface{}) bool {
			p, ok := i.(**azblob.ListBlobsSegmentOptions)
			if !ok {
				return false
			}
			*p = &azOpts
			return true
		}
		if err := opts.BeforeList(asFunc); err != nil {
			return nil, err
		}
	}
	listBlob, err := b.containerURL.ListBlobsHierarchySegment(ctx, marker, opts.Delimiter, azOpts)
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
			MD5:     blobInfo.Properties.ContentMD5,
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

// SignedURL implements driver.SignedURL.
func (b *bucket) SignedURL(ctx context.Context, key string, opts *driver.SignedURLOptions) (string, error) {
	if b.opts.Credential == nil {
		return "", errors.New("to use SignedURL, you must call OpenBucket with a non-nil Options.Credential")
	}
	blockBlobURL := b.blockBlobURL(key)
	srcBlobParts := azblob.NewBlobURLParts(blockBlobURL.URL())

	var err error
	srcBlobParts.SAS, err = azblob.BlobSASSignatureValues{
		Protocol:      azblob.SASProtocolHTTPS,
		ExpiryTime:    time.Now().UTC().Add(opts.Expiry),
		ContainerName: b.name,
		BlobName:      srcBlobParts.BlobName,
		Permissions:   azblob.BlobSASPermissions{Read: true}.String(),
	}.NewSASQueryParameters(b.opts.Credential)
	if err != nil {
		return "", err
	}
	srcBlobURLWithSAS := srcBlobParts.URL()
	return srcBlobURLWithSAS.String(), nil
}

type writer struct {
	ctx          context.Context
	blockBlobURL *azblob.BlockBlobURL
	uploadOpts   *azblob.UploadStreamToBlockBlobOptions

	w     *io.PipeWriter
	donec chan struct{}
	err   error
}

// NewTypedWriter implements driver.NewTypedWriter.
func (b *bucket) NewTypedWriter(ctx context.Context, key string, contentType string, opts *driver.WriterOptions) (driver.Writer, error) {
	blockBlobURL := b.blockBlobURL(key)
	if opts.Metadata == nil {
		opts.Metadata = map[string]string{}
	}
	if opts.BufferSize == 0 {
		opts.BufferSize = defaultUploadBlockSize
	}
	uploadOpts := &azblob.UploadStreamToBlockBlobOptions{
		BufferSize: opts.BufferSize,
		MaxBuffers: defaultUploadBuffers,
		Metadata:   opts.Metadata,
		BlobHTTPHeaders: azblob.BlobHTTPHeaders{
			CacheControl:       opts.CacheControl,
			ContentDisposition: opts.ContentDisposition,
			ContentEncoding:    opts.ContentEncoding,
			ContentLanguage:    opts.ContentLanguage,
			ContentMD5:         opts.ContentMD5,
			ContentType:        contentType,
		},
	}
	if opts.BeforeWrite != nil {
		asFunc := func(i interface{}) bool {
			p, ok := i.(**azblob.UploadStreamToBlockBlobOptions)
			if !ok {
				return false
			}
			*p = uploadOpts
			return true
		}
		if err := opts.BeforeWrite(asFunc); err != nil {
			return nil, err
		}
	}
	return &writer{
		ctx:          ctx,
		blockBlobURL: &blockBlobURL,
		uploadOpts:   uploadOpts,
		donec:        make(chan struct{}),
	}, nil
}

// Write appends p to w. User must call Close to close the w after done writing.
func (w *writer) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if w.w == nil {
		pr, pw := io.Pipe()
		w.w = pw
		if err := w.open(pr); err != nil {
			return 0, err
		}
	}
	return w.w.Write(p)
}

func (w *writer) open(pr *io.PipeReader) error {
	go func() {
		defer close(w.donec)

		var body io.Reader
		if pr == nil {
			body = http.NoBody
		} else {
			body = pr
		}
		_, w.err = azblob.UploadStreamToBlockBlob(w.ctx, body, *w.blockBlobURL, *w.uploadOpts)
		if w.err != nil {
			if pr != nil {
				pr.CloseWithError(w.err)
			}
			return
		}
	}()
	return nil
}

// Close completes the writer and close it. Any error occuring during write will
// be returned. If a writer is closed before any Write is called, Close will
// create an empty file at the given key.
func (w *writer) Close() error {
	if w.w == nil {
		w.open(nil)
	} else if err := w.w.Close(); err != nil {
		return err
	}
	<-w.donec
	return w.err
}
