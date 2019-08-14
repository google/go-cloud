---
title: "blob"
date: 2019-02-21T16:21:29-08:00
aliases:
- /pages/blob/
---

Blobs are a common abstraction for storing unstructured data on cloud storage
providers and accessing them via HTTP.

Package `blob` provides an easy and portable way to interact with blobs within a
storage location ("bucket"). It supports operations like reading and writing
blobs (using standard interfaces from the `io` package), deleting blobs, and
listing blobs in a bucket.

<!--more-->

[How-to guide]({{< ref "/howto/blob/_index.md" >}})<br>
[Top-level package documentation](https://godoc.org/gocloud.dev/blob)

## Supported Services

* [GCS blob](https://godoc.org/gocloud.dev/blob/gcsblob)
* [AWS S3 blob](https://godoc.org/gocloud.dev/blob/s3blob)
* [Azure blob](https://godoc.org/gocloud.dev/blob/azureblob)
* [In-memory local blob](https://godoc.org/gocloud.dev/blob/memblob) - mainly
  useful for local testing
* [File-backed local blob](https://godoc.org/gocloud.dev/blob/fileblob) - local
  blob implementation using the file system

## Usage Samples

* [CLI Tutorial]({{< ref "/tutorials/cli-uploader.md" >}})
* [CLI Sample](https://github.com/google/go-cloud/tree/master/samples/gocdk-blob)
* [Guestbook sample](https://gocloud.dev/tutorials/guestbook/)
* [blob package examples](https://godoc.org/gocloud.dev/blob#pkg-examples)

