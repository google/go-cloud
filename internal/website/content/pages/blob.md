---
title: "Blob"
---

Blobs are a common abstraction for storing unstructured data on cloud storage
providers and accessing them via HTTP.

Package `blob` provides an easy and portable way to interact with blobs within a
storage location ("bucket"). It supports operations like reading and writing
blobs (using standard interfaces from the `io` package), deleting blobs, and
listing blobs in a bucket.

Top-level package documentation: https://godoc.org/gocloud.dev/blob

## Supported Providers

* [AWS S3 blob](https://godoc.org/gocloud.dev/blob/s3blob)
* [GCS blob](https://godoc.org/gocloud.dev/blob/gcsblob)
* [Azure blob](https://godoc.org/gocloud.dev/blob/azureblob)
* [In-memory local blob](https://godoc.org/gocloud.dev/blob/memblob) - mainly
  useful for local testing
* [File-backed local blob](https://godoc.org/gocloud.dev/blob/fileblob) - local
  blob implementation using the file system

The blob package can also interact with most any server that implements the AWS
S3 HTTP API, like [Minio][], [Ceph][], or [SeaweedFS][]. You can change the
endpoint used in an S3 URL using query parameters like so:

```go
bucket, err := blob.OpenBucket("s3://mybucket?" +
    "endpoint=my.minio.local:8080&" +
    "disableSSL=true&" +
    "s3ForcePathStyle=true")
```

See [`aws.ConfigFromURLParams`][] for more details on supported URL options for S3.

[`aws.ConfigFromURLParams`]: https://godoc.org/gocloud.dev/aws#ConfigFromURLParams
[Ceph]: https://ceph.com/
[Minio]: https://www.minio.io/
[SeaweedFS]: https://github.com/chrislusf/seaweedfs

## Usage Samples

* [Tutorial
  sample](https://github.com/google/go-cloud/tree/master/samples/tutorial)
* [Guestbook
  sample](https://github.com/google/go-cloud/tree/master/samples/guestbook)
* [blob package examples](https://godoc.org/gocloud.dev/blob#pkg-examples)

