---
title: "Tutorial: Command-Line Uploader"
linkTitle: "Command-Line Uploader"
date: 2019-03-19T18:45:53-07:00
weight: 1
---

This quickstart will build a command line application called `upload` that
uploads files to blob storage on both AWS and GCP. Blob storage stores binary
data under a string key, and is one of the most frequently used cloud
services.

<!--more-->

When we're done, our command line application will work like this:

```shell
# uploads gopher.png to GCS
$ ./upload gs://go-cloud-bucket gopher.png

# uploads gopher.png to S3
$ ./upload s3://go-cloud-bucket gopher.png

# uploads gopher.png to Azure
$ ./upload azblob://go-cloud-bucket gopher.png
```

(You can download the finished tutorial [from GitHub][samples/tutorial]).

[samples/tutorial]: https://github.com/google/go-cloud/tree/master/samples/tutorial/

## Setup

We start with a skeleton for our program to read from command-line
arguments to configure the bucket URL.

```go
// Command upload saves files to blob storage on GCP, AWS, and Azure.
package main

import (
    "log"
    "os"
)

func main() {
    // Define our input.
    if len(os.Args) != 3 {
        log.Fatal("usage: upload BUCKET_URL FILE")
    }
    bucketURL := os.Args[1]
    file := os.Args[2]
    _, _ = bucketURL, file
}
```

Now that we have a basic skeleton in place, let's start filling in the pieces.

## Connecting to the bucket

The easiest way to open a portable bucket API is with `blob.OpenBucket`.

```go
package main

import (
    // previous imports omitted

    "gocloud.dev/blob"
)

func main() {
    // previous code omitted

    // Open a connection to the bucket.
    b, err := blob.OpenBucket(context.Background(), bucketURL)
    if err != nil {
        log.Fatalf("Failed to setup bucket: %s", err)
    }
    defer b.Close()
}
```

This is all we need in the `main` function to connect to the bucket. However,
as written, this function call will always fail: the Go CDK does not link in any
cloud-specific implementations of `blob.OpenBucket` unless you specifically
depend on them. This ensures you're only depending on the code you need.
To link in implementations, use blank imports:

```go
package main

import (
    // previous imports omitted

    // Import the blob packages we want to be able to open.
    _ "gocloud.dev/blob/azureblob"
    _ "gocloud.dev/blob/gcsblob"
    _ "gocloud.dev/blob/s3blob"
)

func main() {
    // ...
}
```

With the setup done, we're ready to use the bucket connection. Note, as a design
principle of the Go CDK, `blob.Bucket` does not support creating a bucket and
instead focuses solely on interacting with it. This separates the concerns of
provisioning infrastructure and using infrastructure.

## Reading the file

We need to read our file into a slice of bytes before uploading it. The process
is the usual one:

```go
package main

import (
    // previous imports omitted
    "os"
    "io/ioutil"
)

func main() {
    // ... previous code omitted

    // Prepare the file for upload.
    data, err := ioutil.ReadFile(file)
    if err != nil {
        log.Fatalf("Failed to read file: %s", err)
    }
}
```

## Writing the file to the bucket

Now, we have `data`, our file in a slice of bytes. Let's get to the fun part and
write those bytes to the bucket!

```go
package main

// No new imports.

func main() {
    // ...

    w, err := b.NewWriter(ctx, file, nil)
    if err != nil {
        log.Fatalf("Failed to obtain writer: %s", err)
    }
    _, err = w.Write(data)
    if err != nil {
        log.Fatalf("Failed to write to bucket: %s", err)
    }
    if err := w.Close(); err != nil {
        log.Fatalf("Failed to close: %s", err)
    }
}
```

First, we create a writer based on the bucket connection. In addition to a
`context.Context` type, the method takes the key under which the data will be
stored and the mime type of the data.

The call to `NewWriter` creates a `blob.Writer`, which implements `io.Writer`.
With the writer, we call `Write` passing in the data. In response, we get the
number of bytes written and any error that might have occurred.

Finally, we close the writer with `Close` and check the error.

Alternatively, we could have used the shortcut `b.WriteAll(ctx, file, data,
nil)`.

## Uploading an image

That's it! Let's try it out. As setup, we will need to create an
[S3 bucket][s3-bucket], a [GCS bucket][gcs-bucket], and an
[Azure container][azure-container]. In the code above, I called that bucket
`go-cloud-bucket`, but you can change that to match whatever your bucket is
called. Of course, you are free to try the code on any subset of Cloud
providers.

*   For GCP, you will need to login with
    [gcloud](https://cloud.google.com/sdk/install). If you do not want to
    install `gcloud`, see
    [here](https://cloud.google.com/docs/authentication/production) for
    alternatives.
*   For AWS, you will need an access key ID and a secret access key. See
    [here](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_access-keys.html#Using_CreateAccessKey)
    for details. You then need to set the `AWS_REGION` environment variable to
    the region your bucket is in.
*   For Azure, you will need to add your storage account name and key as
    environment variables (`AZURE_STORAGE_ACCOUNT` and
    `AZURE_STORAGE_KEY`, respectively). See
    [here](https://docs.microsoft.com/en-us/azure/storage/blobs/storage-quickstart-blobs-portal)
    for details.

With our buckets created and our credentials set up, we'll build the program
first:

```shell
$ go build -o upload
```

Then, we will send `gopher.png` (in the same directory as this README) to GCS:

```shell
$ ./upload gs://go-cloud-bucket gopher.png
```

Then, we send that same gopher to S3:

```shell
$ export AWS_REGION=us-west-1
$ ./upload s3://go-cloud-bucket gopher.png
```

Finally, we send that same gopher to Azure:

```shell
$ ./upload azblob://go-cloud-bucket gopher.png
```

If we check the buckets, we should see our gopher in each of them! We're done!

[s3-bucket]: https://docs.aws.amazon.com/AmazonS3/latest/gsg/CreatingABucket.html
[gcs-bucket]: https://cloud.google.com/storage/docs/creating-buckets
[azure-container]: https://docs.microsoft.com/en-us/azure/storage/blobs/storage-blobs-introduction

## Wrapping Up

In conclusion, we have a program that can seamlessly switch between multiple
Cloud storage providers using just one code path. You can see the finished
tutorial [on GitHub][samples/tutorial].

We hope this example demonstrates how having one type for multiple clouds is a
huge win for simplicity and maintainability. By writing an application using a
generic interface like `*blob.Bucket`, we retain the option of using
infrastructure in whichever cloud that best fits our needs all without having to
worry about a rewrite. If you want to learn more, you can read about
[Structuring Portable Code][].

If you want to see how to deploy a Go CDK application, see [other tutorials][].
If you want to see how to use Go CDK APIs in your application, see the
[how-to guides][].

[how-to guides]: {{< ref "/howto/_index.md" >}}
[other tutorials]: {{< ref "/tutorials/_index.md" >}}
[Structuring Portable Code]: {{< ref "/concepts/structure/index.md" >}}
