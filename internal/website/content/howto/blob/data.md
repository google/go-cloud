---
title: "Store and Access Unstructured Data"
date: 2019-03-21T08:42:51-07:00
weight: 2
---

Once you have [opened a bucket][] for the storage provider you want, you can
store and access data from it using the standard Go I/O patterns.

[opened a bucket]: {{< ref "./open-bucket.md" >}}

<!--more-->

## Writing data to a bucket

To write data to a bucket, you create a writer, write data to it, and then
close the writer. Closing the writer commits the write to the provider,
flushing any buffers, and releases any resources used while writing, so you
must always check the error of `Close`.

The writer implements [`io.Writer`][], so you can use any functions that take
an `io.Writer` like `io.Copy` or `fmt.Fprintln`.

```go
// Open the key "foo.txt" for writing with the default options.
w, err := bucket.NewWriter(ctx, "foo.txt", nil)
if err != nil {
    return w
}
_, writeErr := fmt.Fprintln(w, "Hello, World!")
// Always check the return value of Close when writing.
closeErr := w.Close()
if writeErr != nil {
    return writeErr
}
if closeErr != nil {
    return closeErr
}
```

In some cases, you may want to cancel an in-progress write to avoid the blob
being created or overwritten. A typical reason for wanting to cancel a write
is encountering an error in the stream your program is copying from. To abort
a write, you cancel the `Context` you pass to the writer. Again, you must
always `Close` the writer to release the resources, but in this case you can
ignore the error because the write's failure is expected.

```go
// Create a cancelable context from the existing context.
writeCtx, cancelWrite := context.WithCancel(ctx)
defer cancelWrite()

// Open the key "foo.txt" for writing with the default options.
w, err := bucket.NewWriter(ctx, "foo.txt", nil)
if err != nil {
    return err
}

// Assume some writes happened and we encountered an error.
// Now we want to abort the write.

// First cancel the context.
cancelWrite()
// You must still close the writer to avoid leaking resources.
w.Close()
```

[`io.Writer`]: https://golang.org/pkg/io/#Writer

## Reading data from a bucket

Once you have written data to a bucket, you can read it back by creating a
reader. The reader implements [`io.Reader`][], so you can use any functions
that take an `io.Reader` like `io.Copy` or `io/ioutil.ReadAll`. You must
always close a reader after using it to avoid leaking resources.

```go
// Open the key "foo.txt" for reading with the default options.
r, err := bucket.NewReader(ctx, "foo.txt", nil)
if err != nil {
    return err
}
defer r.Close()
// Readers also have a limited view of the blob's metadata.
fmt.Println("Content-Type:", r.ContentType())
fmt.Println()
// Copy from the reader to stdout.
if _, err := io.Copy(os.Stdout, r); err != nil {
    return err
}
```

Many storage providers provide efficient random-access to data in buckets. To
start reading from an arbitrary offset in the blob, use `NewRangeReader`.

```go
// Open the key "foo.txt" for reading at offset 1024 and read up to 4096 bytes.
r, err := bucket.NewRangeReader(ctx, "foo.txt", 1024, 4096, nil)
if err != nil {
    return err
}
defer r.Close()
// Copy from the read range to stdout.
if _, err := io.Copy(os.Stdout, r); err != nil {
    return err
}
```

[`io.Reader`]: https://golang.org/pkg/io/#Reader

## Deleting blobs

You can delete blobs using the `Bucket.Delete` method.

```go
if err := bucket.Delete(ctx, "foo.txt"); err != nil {
    return err
}
```

## Wrapping up

These are the most common operations you will need to use with a bucket.
Other operations like listing and reading metadata are documented in the
[`blob` package documentation][].

[`blob` package documentation]: https://godoc.org/gocloud.dev/blob
