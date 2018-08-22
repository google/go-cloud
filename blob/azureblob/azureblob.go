package azureblob

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"strings"
	"time"

	"github.com/Azure/azure-storage-blob-go/2018-03-28/azblob"
	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/driver"
)

type bucket struct {
	name             string
	PublicAccessType azblob.PublicAccessType
	urls             serviceUrls
	containerExists  bool
}
type serviceUrls struct {
	serviceURL   *azblob.ServiceURL
	containerURL *azblob.ContainerURL
}

//Settings for Storage Account connections
type Settings struct {
	AccountName      string
	AccountKey       string
	PublicAccessType azblob.PublicAccessType
	SASToken         string
	// SASTokens can be scoped on a Blob only, in this case attempting to create or read container metadata will result in an AuthorizationFailure exception. To bypass this, set value to false.
	// Setting value to false assumes the container exists
	ContainerExists bool
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
	} else {

		return initBucketWithAccountKey(ctx, settings, containerName)
	}
}

func initBucketWithSASToken(ctx context.Context, settings *Settings, containerName string) (*blob.Bucket, error) {
	if settings.AccountName == "" {
		return nil, fmt.Errorf("Settings.AccountName is not set")
	}

	if settings.SASToken == "" {
		return nil, fmt.Errorf("Settings.SASToken is not set")
	}

	credentials := azblob.NewAnonymousCredential()
	p := azblob.NewPipeline(credentials, azblob.PipelineOptions{})

	u := makeBlobStorageURL(settings.AccountName)
	u.RawQuery = settings.SASToken

	serviceURL := azblob.NewServiceURL(*u, p)
	containerURL := serviceURL.NewContainerURL(containerName)

	return blob.NewBucket(&bucket{
		name:             containerName,
		PublicAccessType: settings.PublicAccessType,
		containerExists:  settings.ContainerExists,
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
	p := azblob.NewPipeline(credentials, azblob.PipelineOptions{})
	u := makeBlobStorageURL(settings.AccountName)

	serviceURL := azblob.NewServiceURL(*u, p)
	containerURL := serviceURL.NewContainerURL(containerName)

	return blob.NewBucket(&bucket{
		name:             containerName,
		PublicAccessType: settings.PublicAccessType,
		containerExists:  settings.ContainerExists,
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
	// check if the container exists
	_, err := b.urls.containerURL.GetProperties(ctx, azblob.LeaseAccessConditions{})
	if err != nil {
		if serr, ok := err.(azblob.StorageError); ok {
			switch serr.ServiceCode() {
			case azblob.ServiceCodeContainerNotFound:
			case azblob.ServiceCodeContainerBeingDeleted:
				return azureError{bucket: b.name, key: key, msg: err.Error(), kind: driver.NotFound}

			case azblob.ServiceCodeAuthenticationFailed:
			case "AuthorizationFailure":
				// SASToken could be scoped to the blob only
				if !b.containerExists {
					return azureError{bucket: b.name, key: key, msg: err.Error(), kind: driver.Unauthorized}
				}
			default:
				return azureError{bucket: b.name, key: key, msg: err.Error(), kind: driver.GenericError}
			}
		}
	}

	blobURL := b.urls.containerURL.NewBlockBlobURL(key)
	_, err = blobURL.Delete(ctx, azblob.DeleteSnapshotsOptionInclude, azblob.BlobAccessConditions{})
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
	// check if the container exists
	_, err := b.urls.containerURL.GetProperties(ctx, azblob.LeaseAccessConditions{})
	if err != nil {
		if serr, ok := err.(azblob.StorageError); ok {
			switch serr.ServiceCode() {
			case azblob.ServiceCodeContainerNotFound:
				return nil, azureError{bucket: b.name, key: key, msg: err.Error(), kind: driver.NotFound}

			case azblob.ServiceCodeAuthenticationFailed:
			case "AuthorizationFailure":
				// Can be thrown if SASToken Scope is restricted to Read & Write on Blobs
				if !b.containerExists {
					// Assume container exist, blob operations will succeed/fail based on SASToken scope
					return nil, azureError{bucket: b.name, key: key, msg: err.Error(), kind: driver.Unauthorized}
				}

				break

			default:
				return nil, azureError{bucket: b.name, key: key, msg: err.Error(), kind: driver.GenericError}
			}
		}
	}

	// make url reference to the blob
	blockBlobURL := b.urls.containerURL.NewBlockBlobURL(key)
	blobPropertiesResponse, err := blockBlobURL.GetProperties(ctx, azblob.BlobAccessConditions{})

	// determine content end for reader
	end := length
	if err == nil {
		if end < 0 {
			end = blobPropertiesResponse.ContentLength()
		}
	}

	// return content reader
	if end > 0 {
		blobDownloadResponse, err := blockBlobURL.Download(ctx, offset, end, azblob.BlobAccessConditions{}, false)
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
	ctx context.Context

	w *io.PipeWriter
	r *io.PipeReader

	urls *serviceUrls

	key         string
	contentType string

	donec chan struct{}
	err   error
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
	containerURL := b.urls.serviceURL.NewContainerURL(b.name)

	if !b.containerExists {
		_, err := containerURL.Create(ctx, nil, b.PublicAccessType)
		if err != nil {
			if serr, ok := err.(azblob.StorageError); ok {
				switch serr.ServiceCode() {
				case azblob.ServiceCodeContainerAlreadyExists:
					// Can be thrown if container already exist, ignore and continue
					break
				case azblob.ServiceCodeAuthenticationFailed:
				case "AuthorizationFailure":
					// Can be thrown if SASToken Scope is restricted to Read & Write on Blobs
					// Assume container exist, blob operations will succeed/fail based on SASToken scope
					return nil, azureError{bucket: b.name, key: key, msg: err.Error(), kind: driver.Unauthorized}
				default:
					return nil, azureError{bucket: b.name, key: key, msg: err.Error(), kind: driver.GenericError}
				}
			}
		}
	}

	w := &writer{
		ctx:         ctx,
		key:         key,
		contentType: contentType,
		urls: &serviceUrls{
			serviceURL:   b.urls.serviceURL,
			containerURL: &containerURL,
		},
		donec: make(chan struct{}),
	}

	return w, nil
}

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
	w.r = pr

	go func() {
		defer close(w.donec)
		var buf []byte
		buf, w.err = ioutil.ReadAll(w.r)
		if w.err != nil {
			w.r.CloseWithError(w.err)
		} else {
			blockBlobURL := w.urls.containerURL.NewBlockBlobURL(w.key)
			_, w.err = blockBlobURL.Upload(w.ctx, bytes.NewReader(buf), azblob.BlobHTTPHeaders{}, nil, azblob.BlobAccessConditions{})

			if w.err != nil {
				w.r.CloseWithError(w.err)
			}
		}
	}()

	return nil
}

// Close completes the writer and close it. Any error occuring during write will
// be returned. If a writer is closed before any Write is called, Close will
// create an empty file at the given key.
func (w *writer) Close() error {
	if err := w.w.Close(); err != nil {
		return err
	}
	<-w.donec
	return w.err
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
