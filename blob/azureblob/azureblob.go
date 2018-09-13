package azureblob

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/driver"

	"github.com/Azure/azure-pipeline-go/pipeline"
	"github.com/Azure/azure-storage-blob-go/2018-03-28/azblob"
)

type bucket struct {
	name             string
	PublicAccessType azblob.PublicAccessType
	urls             serviceUrls
}
type serviceUrls struct {
	serviceURL   *azblob.ServiceURL
	containerURL *azblob.ContainerURL
	blockBlobURL *azblob.BlockBlobURL
}

//Settings for Storage Account connections
type Settings struct {
	AccountName      string
	AccountKey       string
	PublicAccessType azblob.PublicAccessType
	SASToken         string
	Pipeline         pipeline.Pipeline
}

// See https://msdn.microsoft.com/en-us/library/azure/dd179468.aspx and "x-ms-blob-public-access" header.
const (
	// PublicAccessBlob
	PublicAccessBlob azblob.PublicAccessType = azblob.PublicAccessBlob
	// PublicAccessContainer
	PublicAccessContainer azblob.PublicAccessType = azblob.PublicAccessContainer
	// PublicAccessNone represents an empty PublicAccessType.
	PublicAccessNone azblob.PublicAccessType = azblob.PublicAccessNone
)

//OpenBucket returns a new Azure Blob Bucket
func OpenBucket(ctx context.Context, settings *Settings, containerName string) (*blob.Bucket, error) {
	if settings.SASToken != "" {
		return initBucketWithSASToken(ctx, settings, containerName)
	}

	return initBucketWithAccountKey(ctx, settings, containerName)
}

func initBucketWithSASToken(ctx context.Context, settings *Settings, containerName string) (*blob.Bucket, error) {
	if settings.AccountName == "" {
		return nil, fmt.Errorf("Settings.AccountName is not set")
	}

	if settings.SASToken == "" {
		return nil, fmt.Errorf("Settings.SASToken is not set")
	}

	credentials := azblob.NewAnonymousCredential()
	p := settings.Pipeline
	if p == nil {
		p = azblob.NewPipeline(credentials, azblob.PipelineOptions{})
	}

	u := makeBlobStorageURL(settings.AccountName)
	u.RawQuery = settings.SASToken

	serviceURL := azblob.NewServiceURL(*u, p)
	containerURL := serviceURL.NewContainerURL(containerName)

	return blob.NewBucket(&bucket{
		name:             containerName,
		PublicAccessType: settings.PublicAccessType,
		urls: serviceUrls{
			serviceURL:   &serviceURL,
			containerURL: &containerURL,
		},
	}), nil
}

func initBucketWithAccountKey(ctx context.Context, settings *Settings, containerName string) (*blob.Bucket, error) {
	if settings.AccountName == "" {
		return nil, fmt.Errorf("Settings.AccountName is not set")
	}

	if settings.AccountKey == "" {
		return nil, fmt.Errorf("Settings.AccountKey is not set")
	}

	credentials := azblob.NewSharedKeyCredential(settings.AccountName, settings.AccountKey)
	p := settings.Pipeline
	if p == nil {
		p = azblob.NewPipeline(credentials, azblob.PipelineOptions{})
	}
	u := makeBlobStorageURL(settings.AccountName)

	serviceURL := azblob.NewServiceURL(*u, p)
	containerURL := serviceURL.NewContainerURL(containerName)

	return blob.NewBucket(&bucket{
		name:             containerName,
		PublicAccessType: settings.PublicAccessType,
		urls: serviceUrls{
			serviceURL:   &serviceURL,
			containerURL: &containerURL,
		},
	}), nil
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

	blobURL := b.urls.containerURL.NewBlockBlobURL(key)
	_, err := blobURL.Delete(ctx, azblob.DeleteSnapshotsOptionInclude, azblob.BlobAccessConditions{})

	if serr, ok := err.(azblob.StorageError); ok {
		switch serr.ServiceCode() {
		case azblob.ServiceCodeBlobNotFound:
			//	fallthrough
			//case "BlobNotFound":
			return azureError{bucket: b.name, key: key, msg: err.Error(), kind: driver.NotFound}
		default:
			return azureError{bucket: b.name, key: key, msg: err.Error(), kind: driver.GenericError}
		}
	}

	return err
}

type reader struct {
	body        io.ReadCloser
	size        int64
	contentType string
	modTime     time.Time
}

// NewRangeReader returns a reader that reads part of an object, reading at most
// length bytes starting at the given offset. If length is 0, it will read only
// the metadata. If length is negative, it will read till the end of the object.
func (b *bucket) NewRangeReader(ctx context.Context, key string, offset, length int64) (driver.Reader, error) {

	if key == "" {
		return nil, fmt.Errorf("Invalid/Empty Key")
	}

	// make url reference to the blob
	blockBlobURL := b.urls.containerURL.NewBlockBlobURL(key)
	blobPropertiesResponse, err := blockBlobURL.GetProperties(ctx, azblob.BlobAccessConditions{})
	if serr, ok := err.(azblob.StorageError); ok {
		switch serr.ServiceCode() {
		case azblob.ServiceCodeBlobNotFound:
			return nil, azureError{bucket: b.name, key: key, msg: err.Error(), kind: driver.NotFound}
		default:
			// test the http status code for 404/notfound
			errorStatusCode := serr.Response().StatusCode
			if errorStatusCode == 404 {
				return nil, azureError{bucket: b.name, key: key, msg: err.Error(), kind: driver.NotFound}
			}

			return nil, azureError{bucket: b.name, key: key, msg: err.Error(), kind: driver.GenericError}
		}
	}

	// determine content end for reader
	end := length
	if end < 0 {
		end = blobPropertiesResponse.ContentLength()

		if offset > blobPropertiesResponse.ContentLength() {
			offset = 0
		}
	}

	// return content reader
	if end > 0 {
		blobDownloadResponse, _ := blockBlobURL.Download(ctx, offset, end, azblob.BlobAccessConditions{}, false)
		if err != nil {
			return nil, err
		}
		return &reader{body: blobDownloadResponse.Response().Body, contentType: blobPropertiesResponse.ContentType(), size: blobPropertiesResponse.ContentLength(), modTime: blobPropertiesResponse.LastModified()}, nil
	}

	// return metadata reader
	emptyReader := ioutil.NopCloser(strings.NewReader(""))
	return &reader{body: emptyReader, contentType: blobPropertiesResponse.ContentType(), size: blobPropertiesResponse.ContentLength(), modTime: blobPropertiesResponse.LastModified()}, nil
}

func (r *reader) Read(p []byte) (int, error) {
	return r.body.Read(p)
}
func (r *reader) Close() error {
	return r.body.Close()
}
func (r *reader) Attrs() *driver.ObjectAttrs {
	return &driver.ObjectAttrs{
		Size:        r.size,
		ContentType: r.contentType,
	}
}

type writer struct {
	ctx         context.Context
	urls        *serviceUrls
	key         string
	contentType string
	blockIDs    []string
	mux         sync.Mutex
}

// NewTypedWriter returns a writer that writes to an object associated with key.
//
// A new object will be created unless an object with this key already exists.
// Otherwise any previous object with the same name will be replaced.
// The object will not be available (and any previous object will remain)
// until Close has been called.
//
// A WriterOptions can be given to change the default behavior of the writer.
//
// The caller must call Close on the returned writer when done writing.

// Delete deletes the object associated with key. It is a no-op if that object
// does not exist.
func (b *bucket) NewTypedWriter(ctx context.Context, key string, contentType string, opts *driver.WriterOptions) (driver.Writer, error) {

	if key == "" {
		return nil, fmt.Errorf("Invalid/Empty Key")
	}

	containerURL := b.urls.serviceURL.NewContainerURL(b.name)
	// try to create the azure container
	// if it already exists, continue..
	// if authorization failure, assume SASToken scope is limited to the Blob and continue..
	// fail all other exceptions
	_, err := containerURL.Create(ctx, nil, b.PublicAccessType)
	if err != nil {
		if serr, ok := err.(azblob.StorageError); ok {
			switch serr.ServiceCode() {
			case azblob.ServiceCodeContainerAlreadyExists:
				// Can be thrown if container already exist, ignore and continue
			case azblob.ServiceCodeAuthenticationFailed:
				fallthrough
			case "AuthorizationFailure":
				// Can be thrown if SASToken Scope is restricted to Read & Write on Blobs
				// Assume container exist, blob operations will succeed/fail based on SASToken scope
			default:
				return nil, azureError{bucket: b.name, key: key, msg: err.Error(), kind: driver.GenericError}
			}
		}
	}

	blockBlobURL := containerURL.NewBlockBlobURL(key)

	var blockIDs []string
	w := &writer{
		ctx:         ctx,
		key:         key,
		contentType: contentType,
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
// Each call to Write will append to the blockId list for final commit
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
			w.urls.blockBlobURL.StageBlock(w.ctx, bid, bytes.NewReader(c), azblob.LeaseAccessConditions{})
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

// Close completes the writer and close it. Any error occuring during write will
// be returned. If a writer is closed before any Write is called, Close will
// create an empty file at the given key.
func (w *writer) Close() error {
	_, err := w.urls.blockBlobURL.CommitBlockList(w.ctx, w.blockIDs, azblob.BlobHTTPHeaders{}, azblob.Metadata{}, azblob.BlobAccessConditions{})

	if err == nil && w.contentType != "" {
		var basicHeaders = azblob.BlobHTTPHeaders{
			ContentType: w.contentType,
		}
		w.urls.blockBlobURL.SetHTTPHeaders(w.ctx, basicHeaders, azblob.BlobAccessConditions{})
	}

	return err
}

type azureError struct {
	bucket, key, msg string
	kind             driver.ErrorKind
}

func (e azureError) BlobError() driver.ErrorKind {
	return e.kind
}

func (e azureError) Error() string {
	return fmt.Sprintf("azure://%s/%s: %s", e.bucket, e.key, e.msg)
}
