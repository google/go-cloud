---
title: "Blob"
date: 2019-07-09T16:46:29-07:00
lastmod: 2019-07-29T12:00:00-07:00
showInSidenav: true
toc: true
---

Blobs are a common abstraction for storing unstructured data on Cloud storage
services and accessing them via HTTP. This guide shows how to work with
blobs in the Go CDK.

<!--more-->

The [`blob` package][] supports operations like reading and writing blobs (using standard
[`io` package][] interfaces), deleting blobs, and listing blobs in a bucket.

Subpackages contain driver implementations of blob for various services,
including Cloud and on-prem solutions. You can develop your application
locally using [`fileblob`][], then deploy it to multiple Cloud providers with
minimal initialization reconfiguration.

[`blob` package]: https://godoc.org/gocloud.dev/blob
[`io` package]: https://golang.org/pkg/io/
[`fileblob`]: https://godoc.org/gocloud.dev/blob/fileblob

## Opening a Bucket {#opening}

The first step in interacting with unstructured storage is
to instantiate a portable [`*blob.Bucket`][] for your storage service.

The easiest way to do so is to use [`blob.OpenBucket`][] and a service-specific URL
pointing to the bucket, making sure you ["blank import"][] the driver package to
link it in.

```go
import (
	"gocloud.dev/blob"
	_ "gocloud.dev/blob/<driver>"
)
...
bucket, err := blob.OpenBucket(context.Background(), "<driver-url>")
if err != nil {
    return fmt.Errorf("could not open bucket: %v", err)
}
defer bucket.Close()
// bucket is a *blob.Bucket; see usage below
...
``` 

See [Concepts: URLs][] for general background and the [guide below][] for URL usage
for each supported service.

Alternatively, if you need
fine-grained control over the connection settings, you can call the constructor
function in the driver package directly.

```go
import "gocloud.dev/blob/<driver>"
...
bucket, err := <driver>.OpenBucket(...)
...
```

You may find the [`wire` package][] useful for managing your initialization code
when switching between different backing services.

See the [guide below][] for constructor usage for each supported service.

[`wire` package]: http://github.com/google/wire
[`*blob.Bucket`]: https://godoc.org/gocloud.dev/blob#Bucket
[`blob.OpenBucket`]: https://godoc.org/gocloud.dev/blob#OpenBucket
["blank import"]: https://golang.org/doc/effective_go.html#blank_import
[Concepts: URLs]: {{< ref "/concepts/urls.md" >}}
[guide below]: {{< ref "#services" >}}

### Prefixed Buckets {#prefix}

You can wrap a `*blob.Bucket` to always operate on a subfolder of the bucket
using `blob.PrefixedBucket`:

{{< goexample "gocloud.dev/blob.ExamplePrefixedBucket" >}}

Alternatively, you can configure the prefix directly in the `blob.OpenBucket`
URL:

{{< goexample "gocloud.dev/blob.Example_openFromURLWithPrefix" >}}

### Single Key Buckets {#singlekey}

You can wrap a `*blob.Bucket` to always operate on a single key
using `blob.SingleKeyBucket`:

{{< goexample "gocloud.dev/blob.ExampleSingleKeyBucket" >}}

Alternatively, you can configure the single key directly in the `blob.OpenBucket`
URL:

{{< goexample "gocloud.dev/blob.Example_openFromURLWithSingleKey" >}}

The resulting bucket will ignore the `key` parameter to its functions,
and always refer to the single key. This can be useful to allow configuration
of a specific "file" via a single URL.

`List` functions will not work on single key buckets.

## Using a Bucket {#using}

Once you have opened a bucket for the storage provider you want, you can
store and access data from it using the standard Go I/O patterns described
below. Other operations like listing and reading metadata are documented in the
[`blob` package documentation][].

[`blob` package documentation]: https://godoc.org/gocloud.dev/blob

### Writing Data to a Bucket {#writing}

To write data to a bucket, you create a writer, write data to it, and then
close the writer. Closing the writer commits the write to the provider,
flushing any buffers, and releases any resources used while writing, so you
must always check the error of `Close`.

The writer implements [`io.Writer`][], so you can use any functions that take
an `io.Writer` like `io.Copy` or `fmt.Fprintln`.

{{< goexample src="gocloud.dev/blob.ExampleBucket_NewWriter" imports="0" >}}

In some cases, you may want to cancel an in-progress write to avoid the blob
being created or overwritten. A typical reason for wanting to cancel a write
is encountering an error in the stream your program is copying from. To abort
a write, you cancel the `Context` you pass to the writer. Again, you must
always `Close` the writer to release the resources, but in this case you can
ignore the error because the write's failure is expected.

{{< goexample src="gocloud.dev/blob.ExampleBucket_NewWriter_cancel" imports="0" >}}

[`io.Writer`]: https://golang.org/pkg/io/#Writer

### Reading Data from a Bucket {#reading}

Once you have written data to a bucket, you can read it back by creating a
reader. The reader implements [`io.Reader`][], so you can use any functions
that take an `io.Reader` like `io.Copy` or `io/io.ReadAll`. You must
always close a reader after using it to avoid leaking resources.

{{< goexample src="gocloud.dev/blob.ExampleBucket_NewReader" imports="0" >}}

Many storage providers provide efficient random-access to data in buckets. To
start reading from an arbitrary offset in the blob, use `NewRangeReader`.

{{< goexample src="gocloud.dev/blob.ExampleBucket_NewRangeReader" imports="0" >}}

[`io.Reader`]: https://golang.org/pkg/io/#Reader

### Deleting a Bucket {#deleting}

You can delete blobs using the `Bucket.Delete` method.

{{< goexample src="gocloud.dev/blob.ExampleBucket_Delete" imports="0" >}}

## Other Usage Samples

* [CLI Tutorial]({{< ref "/tutorials/cli-uploader.md" >}})
* [CLI Sample](https://github.com/google/go-cloud/tree/master/samples/gocdk-blob)
* [Guestbook sample](https://gocloud.dev/tutorials/guestbook/)
* [blob package examples](https://godoc.org/gocloud.dev/blob#pkg-examples)

## Supported Storage Services {#services}

### Google Cloud Storage {#gcs}

[Google Cloud Storage][] (GCS) URLs in the Go CDK closely resemble the URLs
you would see in the [`gsutil`][] CLI.

[Google Cloud Storage]: https://cloud.google.com/storage/
[`gsutil`]: https://cloud.google.com/storage/docs/gsutil

`blob.OpenBucket` will use Application Default Credentials; if you have
authenticated via [`gcloud auth application-default login`][], it will use those credentials. See
[Application Default Credentials][GCP creds] to learn about authentication
alternatives, including using environment variables.

[GCP creds]: https://cloud.google.com/docs/authentication/production
[`gcloud auth application-default login`]: https://cloud.google.com/sdk/gcloud/reference/auth/application-default/login

{{< goexample "gocloud.dev/blob/gcsblob.Example_openBucketFromURL" >}}

Full details about acceptable URLs can be found under the API reference for
[`gcsblob.URLOpener`][].

[`gcsblob.URLOpener`]: https://godoc.org/gocloud.dev/blob/gcsblob#URLOpener

#### GCS Constructor {#gcs-ctor}

The [`gcsblob.OpenBucket`][] constructor opens a GCS bucket. You must first
create a `*net/http.Client` that sends requests authorized by [Google Cloud
Platform credentials][GCP creds]. (You can reuse the same client for any
other API that takes in a `*gcp.HTTPClient`.) You can find functions in the
[`gocloud.dev/gcp`][] package to set this up for you.

{{< goexample "gocloud.dev/blob/gcsblob.ExampleOpenBucket" >}}

[`gcsblob.OpenBucket`]: https://godoc.org/gocloud.dev/blob/gcsblob#OpenBucket
[`gocloud.dev/gcp`]: https://godoc.org/gocloud.dev/gcp

### S3 {#s3}

S3 URLs in the Go CDK closely resemble the URLs you would see in the [AWS CLI][].
You should specify the `region` query parameter to ensure your application
connects to the correct region.

If you set the "awssdk=v1" query parameter,
`blob.OpenBucket` will create a default AWS Session with the
`SharedConfigEnable` option enabled; if you have authenticated with the AWS CLI,
it will use those credentials. See [AWS Session][] to learn about authentication
alternatives, including using environment variables.

If you set the "awssdk=v2" query parameter, it will instead create an AWS
Config based on the AWS SDK V2; see [AWS V2 Config][] to learn more.

If no "awssdk" query parameter is set, Go CDK will use a default (currently V2).

Full details about acceptable URLs can be found under the API reference for
[`s3blob.URLOpener`][].

{{< goexample "gocloud.dev/blob/s3blob.Example_openBucketFromURL" >}}

[AWS CLI]: https://aws.amazon.com/cli/
[AWS Session]: https://docs.aws.amazon.com/sdk-for-go/api/aws/session/
[AWS V2 Config]: https://aws.github.io/aws-sdk-go-v2/docs/configuring-sdk/
[`s3blob.URLOpener`]: https://godoc.org/gocloud.dev/blob/s3blob#URLOpener

#### S3 Constructor {#s3-ctor}

The [`s3blob.OpenBucket`][] constructor opens an [S3][] bucket. You must first
create an [AWS session][] with the same region as your bucket:

{{< goexample "gocloud.dev/blob/s3blob.ExampleOpenBucket" >}}

[`s3blob.OpenBucketV2`][] is similar but uses the AWS SDK V2.

{{< goexample "gocloud.dev/blob/s3blob.ExampleOpenBucketV2" >}}

[`s3blob.OpenBucket`]: https://godoc.org/gocloud.dev/blob/s3blob#OpenBucket
[`s3blob.OpenBucketV2`]: https://godoc.org/gocloud.dev/blob/s3blob#OpenBucketV2
[AWS session]: https://docs.aws.amazon.com/sdk-for-go/api/aws/session/
[S3]: https://aws.amazon.com/s3/

#### S3-Compatible Servers {#s3-compatible}

The Go CDK can also interact with [S3-compatible storage servers][] that
recognize the same REST HTTP endpoints as S3, like [Minio][], [Ceph][], or
[SeaweedFS][]. You can change the endpoint by changing the [`Endpoint` field][]
on the `*aws.Config` you pass to `s3blob.OpenBucket`. If you are using
`blob.OpenBucket`, you can switch endpoints by using the S3 URL using query
parameters like so:

```go
bucket, err := blob.OpenBucket("s3://mybucket?" +
    "endpoint=my.minio.local:8080&" +
    "disableSSL=true&" +
    "s3ForcePathStyle=true")
```

See [`aws.ConfigFromURLParams`][] for more details on supported URL options for S3.

[`aws.ConfigFromURLParams`]: https://godoc.org/gocloud.dev/aws#ConfigFromURLParams
[`Endpoint` field]: https://godoc.org/github.com/aws/aws-sdk-go/aws#Config.Endpoint
[Ceph]: https://ceph.com/
[Minio]: https://www.minio.io/
[SeaweedFS]: https://github.com/chrislusf/seaweedfs
[S3-compatible storage servers]: https://en.wikipedia.org/wiki/Amazon_S3#S3_API_and_competing_services

### Azure Blob Storage {#azure}

Azure Blob Storage URLs in the Go CDK allow you to identify [Azure Blob Storage][] containers
when opening a bucket with `blob.OpenBucket`. Go CDK uses the environment
variables `AZURE_STORAGE_ACCOUNT`, `AZURE_STORAGE_KEY`, and
`AZURE_STORAGE_SAS_TOKEN`, among others, to configure the credentials.

{{< goexample "gocloud.dev/blob/azureblob.Example_openBucketFromURL" >}}

Full details about acceptable URLs can be found under the API reference for
[`azureblob.URLOpener`][].

[Azure Blob Storage]: https://azure.microsoft.com/en-us/services/storage/blobs/
[`azureblob.URLOpener`]: https://godoc.org/gocloud.dev/blob/azureblob#URLOpener

#### Azure Blob Constructor {#azure-ctor}

The [`azureblob.OpenBucket`][] constructor opens an Azure Blob Storage container.
`azureblob` operates on [Azure Storage Block Blobs][]. You must first create
an Azure Service Client before you can open a container.

{{< goexample "gocloud.dev/blob/azureblob.ExampleOpenBucket" >}}

[Azure Storage Block Blobs]: https://docs.microsoft.com/en-us/rest/api/storageservices/understanding-block-blobs--append-blobs--and-page-blobs#about-block-blobs
[`azureblob.OpenBucket`]: https://godoc.org/gocloud.dev/blob/azureblob#OpenBucket

### Local Storage {#local}

The Go CDK provides blob drivers for storing data in memory and on the local
filesystem. These are primarily intended for testing and local development,
but may be useful in production scenarios where an NFS mount is used.

Local storage URLs take the form of either `mem://` or `file:///` URLs.
Memory URLs are always `mem://` with no other information and always create a
new bucket. File URLs convert slashes to the operating system's native file
separator, so on Windows, `C:\foo\bar` would be written as
`file:///C:/foo/bar`.

```go
import (
    "gocloud.dev/blob"
    _ "gocloud.dev/blob/fileblob"
    _ "gocloud.dev/blob/memblob"
)

// ...

bucket1, err := blob.OpenBucket(ctx, "mem://")
if err != nil {
    return err
}
defer bucket1.Close()

bucket2, err := blob.OpenBucket(ctx, "file:///path/to/dir")
if err != nil {
    return err
}
defer bucket2.Close()
```

#### Local Storage Constructors {#local-ctor}

You can create an in-memory bucket with [`memblob.OpenBucket`][]:

{{< goexample "gocloud.dev/blob/memblob.ExampleOpenBucket" >}}

You can use a local filesystem directory with [`fileblob.OpenBucket`][]:

{{< goexample "gocloud.dev/blob/fileblob.ExampleOpenBucket" >}}

[`fileblob.OpenBucket`]: https://godoc.org/gocloud.dev/blob/fileblob#OpenBucket
[`memblob.OpenBucket`]: https://godoc.org/gocloud.dev/blob/memblob#OpenBucket

