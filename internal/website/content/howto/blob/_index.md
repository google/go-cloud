---
title: "Blob"
date: 2019-07-09T16:46:29-07:00
lastmod: 2019-07-29T12:00:00-07:00
showInSidenav: true
toc: true
---

Blobs are a common abstraction for storing unstructured data on cloud storage
providers and accessing them via HTTP. These guides show how to work with
blobs in the Go CDK.

<!--more-->

## Opening a Bucket {#opening}

The first step in interacting with unstructured storage is connecting to your
storage provider. Every storage provider is a little different, but the Go CDK
lets you interact with all of them using the [`*blob.Bucket` type][].

The easiest way to open a blob is using [`blob.OpenBucket`][] and a URL
pointing to the blob, making sure you ["blank import"][] the driver package to
link it in. See [Concepts: URLs][] for more details. If you need
fine-grained control over the connection settings, you can call the constructor
function in the driver package directly (like `s3blob.OpenBucket`).

See the [guide below][] for usage of both forms for each supported provider.

[`*blob.Bucket` type]: https://godoc.org/gocloud.dev/blob#Bucket
[`blob.OpenBucket`]:
https://godoc.org/gocloud.dev/blob#OpenBucket
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

## Using a Bucket {#using}

Once you have opened a bucket for the storage provider you want, you can
store and access data from it using the standard Go I/O patterns.

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
that take an `io.Reader` like `io.Copy` or `io/ioutil.ReadAll`. You must
always close a reader after using it to avoid leaking resources.

{{< goexample src="gocloud.dev/blob.ExampleBucket_NewReader" imports="0" >}}

Many storage providers provide efficient random-access to data in buckets. To
start reading from an arbitrary offset in the blob, use `NewRangeReader`.

{{< goexample src="gocloud.dev/blob.ExampleBucket_NewRangeReader" imports="0" >}}

[`io.Reader`]: https://golang.org/pkg/io/#Reader

### Deleting a Bucket {#deleting}

You can delete blobs using the `Bucket.Delete` method.

{{< goexample src="gocloud.dev/blob.ExampleBucket_Delete" imports="0" >}}

### Other Operations {#other}

These are the most common operations you will need to use with a bucket.
Other operations like listing and reading metadata are documented in the
[`blob` package documentation][].

[`blob` package documentation]: https://godoc.org/gocloud.dev/blob

## Supported Storage Services {#services}

### Google Cloud Storage {#gcs}

[Google Cloud Storage][] (GCS) URLs in the Go CDK closely resemble the URLs
you would see in the [`gsutil`][] CLI.

[Google Cloud Storage]: https://cloud.google.com/storage/
[`gsutil`]: https://cloud.google.com/storage/docs/gsutil

`blob.OpenBucket` will use Application Default Credentials; if you have
authenticated via [`gcloud auth login`][], it will use those credentials. See
[Application Default Credentials][GCP creds] to learn about authentication
alternatives, including using environment variables.

[GCP creds]: https://cloud.google.com/docs/authentication/production
[`gcloud auth login`]: https://cloud.google.com/sdk/gcloud/reference/auth/login

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

S3 URLs in the Go CDK closely resemble the URLs you would see in the AWS CLI.
You should specify the `region` query parameter to ensure your application
connects to the correct region.

`blob.OpenBucket` will create a default AWS Session with the
`SharedConfigEnable` option enabled; if you have authenticated with the AWS CLI,
it will use those credentials. See [AWS Session][] to learn about authentication
alternatives, including using environment variables.

[AWS Session]: https://docs.aws.amazon.com/sdk-for-go/api/aws/session/

{{< goexample "gocloud.dev/blob/s3blob.Example_openBucketFromURL" >}}

Full details about acceptable URLs can be found under the API reference for
[`s3blob.URLOpener`][].

[`s3blob.URLOpener`]: https://godoc.org/gocloud.dev/blob/s3blob#URLOpener

#### S3 Constructor {#s3-ctor}

The [`s3blob.OpenBucket`][] constructor opens an [S3][] bucket. You must first
create an [AWS session][] with the same region as your bucket:

{{< goexample "gocloud.dev/blob/s3blob.ExampleOpenBucket" >}}

[`s3blob.OpenBucket`]: https://godoc.org/gocloud.dev/blob/s3blob#OpenBucket
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
`AZURE_STORAGE_SAS_TOKEN` to configure the credentials. `AZURE_STORAGE_ACCOUNT`
is required, along with one of the other two.

{{< goexample "gocloud.dev/blob/azureblob.Example_openBucketFromURL" >}}

Full details about acceptable URLs can be found under the API reference for
[`azureblob.URLOpener`][].

[Azure Blob Storage]: https://azure.microsoft.com/en-us/services/storage/blobs/
[`azureblob.URLOpener`]: https://godoc.org/gocloud.dev/blob/azureblob#URLOpener

#### Azure Blob Constructor {#azure-ctor}

The [`azureblob.OpenBucket`][] constructor opens an Azure Blob Storage container.
`azureblob` operates on [Azure Storage Block Blobs][]. You must first create
Azure Storage credentials and then create an Azure Storage pipeline before
you can open a container.

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
