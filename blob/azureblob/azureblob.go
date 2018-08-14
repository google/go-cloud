package azureblob

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/driver"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"

	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2017-10-01/storage"
	mainStorage "github.com/Azure/azure-sdk-for-go/storage"
)

type bucket struct {
	name                string
	containerAccessType string
	client              *mainStorage.BlobStorageClient
}

type AzureBlobSettings struct {
	Authorizer          autorest.Authorizer
	EnvironmentName     string
	SubscriptionId      string
	ResourceGroupName   string
	StorageAccountName  string
	StorageKey          string
	ContainerAccessType string // See https://msdn.microsoft.com/en-us/library/azure/dd179468.aspx and "x-ms-blob-public-access" header.
}

func OpenBucket(ctx context.Context, blobSettings *AzureBlobSettings, containerName string) (*blob.Bucket, error) {

	if blobSettings.Authorizer == nil {
		return nil, fmt.Errorf("AzureBlobSettings.Authorizer is not set")
	}
	if blobSettings.EnvironmentName == "" {
		return nil, fmt.Errorf("AzureBlobSettings.EnvironmentName is not set")
	}
	if blobSettings.ResourceGroupName == "" {
		return nil, fmt.Errorf("AzureBlobSettings.ResourceGroupName is not set")
	}
	if blobSettings.StorageAccountName == "" {
		return nil, fmt.Errorf("AzureBlobSettings.StorageAccountName is not set")
	}
	if blobSettings.SubscriptionId == "" {
		return nil, fmt.Errorf("AzureBlobSettings.SubscriptionId is not set")
	}

	fetchKey := blobSettings.StorageKey == ""
	environment, err := azure.EnvironmentFromName(blobSettings.EnvironmentName)
	if err != nil {
		return nil, fmt.Errorf("Azure Environment %q is invalid", blobSettings.EnvironmentName)
	}

	accountClient := storage.NewAccountsClientWithBaseURI(environment.ResourceManagerEndpoint, blobSettings.SubscriptionId)
	accountClient.Authorizer = blobSettings.Authorizer
	accountClient.Sender = autorest.CreateSender(WithRequestLogging())

	if fetchKey {
		key, err := GetStorageAccountKey(&accountClient, blobSettings.ResourceGroupName, blobSettings.StorageAccountName)
		if err != nil {
			return nil, err
		} else {
			blobSettings.StorageKey = key
		}
	}

	storageClient, err := mainStorage.NewClient(blobSettings.StorageAccountName, blobSettings.StorageKey, environment.StorageEndpointSuffix,
		mainStorage.DefaultAPIVersion, true)

	if err != nil {
		return nil, fmt.Errorf("Error creating storage client for storage storeAccount %q: %s", blobSettings.StorageAccountName, err)
	}

	blobClient := storageClient.GetBlobService()

	return blob.NewBucket(&bucket{
		name:                containerName,
		containerAccessType: blobSettings.ContainerAccessType,
		client:              &blobClient,
	}), nil
}

type reader struct {
	body        io.ReadCloser
	size        int64
	contentType string
}

// NewRangeReader returns a reader that reads part of an object, reading at most
// length bytes starting at the given offset. If length is 0, it will read only
// the metadata. If length is negative, it will read till the end of the object.
func (b *bucket) NewRangeReader(ctx context.Context, key string, offset, length int64) (driver.Reader, error) {
	theContainer := b.client.GetContainerReference(b.name)

	exists, err := theContainer.Exists()
	if !exists {
		empty := ioutil.NopCloser(strings.NewReader(""))
		return &reader{body: empty}, err
	}

	theblob := theContainer.GetBlobReference(key)
	theblob.GetProperties(nil)

	if theblob == nil {
		return nil, azureError{bucket: b.name, key: key, msg: err.Error(), kind: driver.NotFound}
	}

	if length != 0 {

		var ioReader io.ReadCloser
		rangeEnd := length
		if length < 0 {
			rangeEnd = theblob.Properties.ContentLength
		}

		if length > 0 {
			rangeEnd = offset + length
		}

		readRange := mainStorage.BlobRange{Start: uint64(offset), End: uint64(rangeEnd)}
		ioReader, err = theblob.GetRange(&mainStorage.GetBlobRangeOptions{Range: &readRange})

		if err != nil {
			return nil, err
		} else {
			return &reader{body: ioReader, contentType: theblob.Properties.ContentType, size: theblob.Properties.ContentLength}, nil
		}
	} else {
		empty := ioutil.NopCloser(strings.NewReader(""))
		return &reader{body: empty, contentType: theblob.Properties.ContentType, size: theblob.Properties.ContentLength}, nil
	}
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

	container *mainStorage.Container
	blob      *mainStorage.Blob

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

	theContainer := b.client.GetContainerReference(b.name)
	_, err := theContainer.CreateIfNotExists(&mainStorage.CreateContainerOptions{Access: mainStorage.ContainerAccessType(b.containerAccessType)})

	if err != nil {
		return nil, err
	}

	theBlob := theContainer.GetBlobReference(key)
	theBlob.Properties.ContentType = contentType

	w := &writer{
		ctx:         ctx,
		container:   theContainer,
		blob:        theBlob,
		key:         key,
		contentType: contentType,
	}

	return w, nil
}

func (w *writer) Write(p []byte) (int, error) {
	w.open()

	select {
	case <-w.donec:
		return 0, w.err
	default:
	}

	return w.w.Write(p)
}

func (w *writer) open() error {
	if w.w == nil {
		pr, pw := io.Pipe()
		w.w = pw
		w.r = pr
	}

	go func() {
		defer close(w.donec)

		w.err = w.blob.CreateBlockBlobFromReader(w.r, nil)
		if w.err == nil {
			w.blob.SetProperties(nil)
		} else {
			w.r.CloseWithError(w.err)
		}
	}()

	return nil
}

// Close completes the writer and close it. Any error occuring during write will
// be returned. If a writer is closed before any Write is called, Close will
// create an empty file at the given key.
func (w *writer) Close() error {

	if w.w == nil {
		w.touch()
	} else if err := w.w.Close(); err != nil {
		return err
	}

	go func() {
		<-w.donec
	}()

	return w.err
}

// touch creates an empty object in the bucket. It is called if user creates a
// new writer but never calls write before closing it.
func (w *writer) touch() {
	defer close(w.donec)
	w.err = w.blob.CreateBlockBlob(nil)
}

// this deletes a file within a container
func (b *bucket) Delete(ctx context.Context, key string) error {
	theContainer := b.client.GetContainerReference(b.name)
	exists, _ := theContainer.Exists()

	if exists {
		theBlob := theContainer.GetBlobReference(key)
		_, err := theBlob.DeleteIfExists(nil)
		return err
	}

	return nil
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
