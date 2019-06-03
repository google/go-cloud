---
title: "URLs"
date: 2019-05-06T09:55:09-07:00
weight: 2
---

In addition to creating portable types via provider-specific constructors
(e.g., creating a `*blob.Bucket` using [`s3blob.OpenBucket`][]), many portable types
can also be created using a URL. The scheme of the URL specifies the provider,
and each provider implementation has code to convert the URL into the data
needed to call its constructor. For example, calling
`blob.OpenBucket("s3blob://my-bucket")` will return a `*blob.Bucket` created
using [`s3blob.OpenBucket`][].

[`s3blob.OpenBucket`]: https://godoc.org/gocloud.dev/blob/s3blob#OpenBucket

<!--more-->

Each portable API package will document the types that it supports opening
by URL. For example, the `blob` package supports `Bucket`s, while the `pubsub`
package supports `Topic`s and `Subscription`s. Each provider implementation will
document what scheme(s) it registers for, and what format of URL it expects.

Each portable type URL opener will accept URL schemes with an `<api>+` prefix
(e.g. `blob+file:///dir` instead of `file:///dir`, as well as schemes with an
`<api>+<type>+` prefix (e.g. `blob+bucket+file:///dir`).

Each portable API package should include an example using a URL, and
many providers will include provider-specific examples as well.

## Muxes

Each portable type that is openable via URL will have a top-level function
you can call, like [`blob.OpenBucket`][]. This top-level function uses a default
instance of a `URLMux` multiplexer to map schemes to a provider-specific
opener for the type. For example, `blob` has a [`BucketURLOpener`][] interface
that providers implement and then register using [`RegisterBucket`][] on the
result of [`DefaultURLMux`][].

Many applications will work just fine using the default mux through the
top-level `Open` functions. However, if you want more control, you can create
your own `URLMux` and register the provider `URLOpener`s you need. Most
providers will export URLOpeners that give you more fine grained control over
the arguments needed by the constructor. In particular, portable types opened
via URL will often use default credentials from the environment. For example,
the AWS URL openers use the credentials saved by "aws login" (we don't want
to include credentials in the URL itself, since they are likely to be
sensitive).

1. Instantiate the provider's `URLOpener` with the specific fields you need.
   For example, `s3blob.URLOpener{ConfigProvider: myAWSProvider}` using a
   `ConfigProvider` that holds explicit AWS credentials.
2. Create your own instance of the `URLMux`. For example: `mymux := new(blob.URLMux)`
3. Register your custom URLOpener on your mux. For example:
   `mymux.RegisterBucket(s3blob.Scheme, myS3URLOpener)`
4. Now use your mux to open URLs: `mymux.OpenBucket("s3://my-bucket")`

[`blob.OpenBucket`]: https://godoc.org/gocloud.dev/blob#OpenBucket
[`BucketURLOpener`]: https://godoc.org/gocloud.dev/blob#BucketURLOpener
[`DefaultURLMux`]: https://godoc.org/gocloud.dev/blob#DefaultURLMux
[`RegisterBucket`]: https://godoc.org/gocloud.dev/blob#URLMux.RegisterBucket
