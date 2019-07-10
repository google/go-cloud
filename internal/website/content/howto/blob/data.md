---
title: "Store and Access Unstructured Data"
date: 2019-03-21T08:42:51-07:00
weight: 2
toc: true
---

Once you have [opened a bucket][] for the storage provider you want, you can
store and access data from it using the standard Go I/O patterns.

[opened a bucket]: {{< ref "./open-bucket.md" >}}

<!--more-->

## Writing data to a bucket {#writing}

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

## Reading data from a bucket {#reading}

Once you have written data to a bucket, you can read it back by creating a
reader. The reader implements [`io.Reader`][], so you can use any functions
that take an `io.Reader` like `io.Copy` or `io/ioutil.ReadAll`. You must
always close a reader after using it to avoid leaking resources.

{{< goexample src="gocloud.dev/blob.ExampleBucket_NewReader" imports="0" >}}

Many storage providers provide efficient random-access to data in buckets. To
start reading from an arbitrary offset in the blob, use `NewRangeReader`.

{{< goexample src="gocloud.dev/blob.ExampleBucket_NewRangeReader" imports="0" >}}

[`io.Reader`]: https://golang.org/pkg/io/#Reader

## Deleting blobs {#deleting}

You can delete blobs using the `Bucket.Delete` method.

{{< goexample src="gocloud.dev/blob.ExampleBucket_Delete" imports="0" >}}

## Wrapping up

These are the most common operations you will need to use with a bucket.
Other operations like listing and reading metadata are documented in the
[`blob` package documentation][].

[`blob` package documentation]: https://godoc.org/gocloud.dev/blob
