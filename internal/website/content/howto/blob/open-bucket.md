---
title: "Open a Bucket"
date: 2019-03-20T14:51:29-07:00
weight: 1
---

The first step in interacting with unstructured storage is connecting to your
storage provider. Every storage provider is a little different, but the Go CDK
lets you interact with all of them using the [`*blob.Bucket` type][].

[`*blob.Bucket` type]: https://godoc.org/gocloud.dev/blob#Bucket

<!--more-->

## Constructors versus URL openers

If you know that your program is always going to use a particular storage
provider or you need fine-grained control over the connection settings, you
should call the constructor function in the driver package directly (like
`s3blob.OpenBucket`). However, if you want to change providers based on
configuration, you can use `blob.OpenBucket`, making sure you ["blank
import"][] the driver package to link it in. See the
[documentation on URLs][] for more details. This guide will show how to use
both forms for each storage provider.

["blank import"]: https://golang.org/doc/effective_go.html#blank_import
[documentation on URLs]: {{< ref "/concepts/urls.md" >}}

### Prefixed Buckets {#prefix}

You can wrap a `*blob.Bucket` to always operate on a subfolder of the bucket
using `blob.PrefixedBucket`:

{{< goexample "gocloud.dev/blob/ExamplePrefixedBucket" >}}

Alternatively, you can configure the prefix directly in the `blob.OpenBucket`
URL:

{{< goexample "gocloud.dev/blob/Example_openFromURLWithPrefix" >}}

## S3 {#s3}

S3 URLs in the Go CDK closely resemble the URLs you would see in the AWS CLI.
You can specify the `region` query parameter to ensure your application connects
to the correct region, but otherwise `blob.OpenBucket` will use the region found
in the environment variables or your AWS CLI configuration.

{{< goexample "gocloud.dev/blob/s3blob.Example_openBucketFromURL" >}}

Full details about acceptable URLs can be found under the API reference for
[`s3blob.URLOpener`][].

[`s3blob.URLOpener`]: https://godoc.org/gocloud.dev/blob/s3blob#URLOpener

### S3 Constructor {#s3-ctor}

The [`s3blob.OpenBucket`][] constructor opens an [S3][] bucket. You must first
create an [AWS session][] with the same region as your bucket:

{{< goexample "gocloud.dev/blob/s3blob.ExampleOpenBucket" >}}

[`s3blob.OpenBucket`]: https://godoc.org/gocloud.dev/blob/s3blob
[AWS session]: https://docs.aws.amazon.com/sdk-for-go/api/aws/session/
[S3]: https://aws.amazon.com/s3/

### S3-compatible storage servers {#s3-compatible}

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

## Google Cloud Storage {#gcs}

[Google Cloud Storage][] (GCS) URLs in the Go CDK closely resemble the URLs
you would see in the `gsutil` CLI. `blob.OpenBucket` will use [Application
Default Credentials][GCP creds].

{{< goexample "gocloud.dev/blob/gcsblob.Example_openBucketFromURL" >}}

Full details about acceptable URLs can be found under the API reference for
[`gcsblob.URLOpener`][].

[Google Cloud Storage]: https://cloud.google.com/storage/
[`gcsblob.URLOpener`]: https://godoc.org/gocloud.dev/blob/gcsblob#URLOpener

### Google Cloud Storage Constructor {#gcs-ctor}

The [`gcsblob.OpenBucket`][] constructor opens a GCS bucket. You must first
create a `*net/http.Client` that sends requests authorized by [Google Cloud
Platform credentials][GCP creds]. (You can reuse the same client for any
other API that takes in a `*gcp.HTTPClient`.) You can find functions in the
[`gocloud.dev/gcp`][] package to set this up for you.

{{< goexample "gocloud.dev/blob/gcsblob.ExampleOpenBucket" >}}

[GCP creds]: https://cloud.google.com/docs/authentication/production
[`gcsblob.OpenBucket`]: https://godoc.org/gocloud.dev/blob/gcsblob#OpenBucket
[`gocloud.dev/gcp`]: https://godoc.org/gocloud.dev/gcp

## Azure Storage {#azure}

Azure Storage URLs in the Go CDK allow you to identify [Azure Storage][] containers
when opening a bucket with `blob.OpenBucket`. Go CDK uses the environment
variables `AZURE_STORAGE_ACCOUNT`, `AZURE_STORAGE_KEY`, and
`AZURE_STORAGE_SAS_TOKEN` to configure the credentials. `AZURE_STORAGE_ACCOUNT`
is required, along with one of the other two.

{{< goexample "gocloud.dev/blob/azureblob.Example_openBucketFromURL" >}}

Full details about acceptable URLs can be found under the API reference for
[`azureblob.URLOpener`][].

[Azure Storage]: https://azure.microsoft.com/en-us/services/storage/
[`azureblob.URLOpener`]: https://godoc.org/gocloud.dev/blob/azureblob#URLOpener

### Azure Storage Constructor {#azure-ctor}

The [`azureblob.OpenBucket`][] constructor opens an Azure Storage container.
`azureblob` operates on [Azure Storage Block Blobs][]. You must first create
Azure Storage credentials and then create an Azure Storage pipeline before
you can open a container.

{{< goexample "gocloud.dev/blob/azureblob.ExampleOpenBucket" >}}

[Azure Storage Block Blobs]: https://docs.microsoft.com/en-us/rest/api/storageservices/understanding-block-blobs--append-blobs--and-page-blobs#about-block-blobs
[`azureblob.OpenBucket`]: https://godoc.org/gocloud.dev/blob/azureblob#OpenBucket

## Local Storage {#local}

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

### Local Storage Constructors {#local-ctor}

You can create an in-memory bucket with [`memblob.OpenBucket`][]:

{{< goexample "gocloud.dev/blob/memblob.ExampleOpenBucket" >}}

You can use a local filesystem directory with [`fileblob.OpenBucket`][]:

{{< goexample "gocloud.dev/blob/fileblob.ExampleOpenBucket" >}}

[`fileblob.OpenBucket`]: https://godoc.org/gocloud.dev/blob/fileblob#OpenBucket
[`memblob.OpenBucket`]: https://godoc.org/gocloud.dev/blob/memblob#OpenBucket

## What's Next

Now that you have opened a bucket, you can [store data in and access data
from][] the bucket using portable operations.

[store data in and access data from]: {{< ref "./data.md" >}}
