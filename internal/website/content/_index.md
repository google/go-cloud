---
title: "Go CDK"
pkgmeta: true
---

# The Go Cloud Development Kit

The Go Cloud Development Kit (Go CDK) is an open source project building
libraries and tools to improve the experience of developing for the cloud with
Go.

Go CDK provides commonly used, vendor-neutral generic APIs that you can deploy
across cloud providers. The idea is to support hybrid cloud deployments while
combining on-prem (local) and cloud tools.

This project also lays the foundation for other open source projects to write
cloud libraries that work across providers. It does this by providing stable,
idiomatic interfaces for use cases like storage, events and databases.

For more background about the project, check out the
[announcement blog post](https://blog.golang.org/go-cloud) and
[our talk from Cloud Next 2018](https://www.youtube.com/watch?v=_2ZwhvIkgek).

If you're interested in contributing to the Go CDK or are interested in checking
out the code, head to [our GitHub project
page](https://github.com/google/go-cloud).

## Installing and getting started

To start using the Go CDK, install it using `go get`:

```shell
go get gocloud.dev
```

Then follow the [Go CDK tutorial][]. Links to additional documentation and
samples are available below and in the site navigation bar.

[Go CDK tutorial]: {{< ref "/tutorials/cli.md" >}}

## Portable Cloud APIs in Go

At this time, the Go CDK focuses on a set of portable APIs for cloud
programming. We strive to implement these APIs for the leading Cloud providers:
AWS, GCP and Azure, as well as provide a local (on-prem) implementation.

Using the Go CDK you can write your application code once using these idiomatic
APIs, test locally using the local versions, and then deploy to a cloud provider
with only minimal setup-time changes.

Please check out the linked pages for a detailed description of each API and
examples of how to use it:

* Unstructured binary storage ([blob]({{< ref "/ref/blob.md" >}}))
* Publisher/Subscriber ([pubsub]({{< ref "/ref/pubsub.md" >}}))
* Variables that change at runtime ([runtimevar]({{< ref "/ref/runtimevar.md" >}}))
* HTTP server with request logging, tracing and health checking
  ([server]({{< ref "/ref/server.md" >}}))
* Secret management, encryption and decryption ([secrets]({{< ref "/ref/secrets.md" >}}))
* Connecting to hosted SQL services ([mysql/postgres]({{< ref "/ref/sql.md" >}}))

## Project status

We're looking for early adopters to help us validate the APIs before releasing
a beta version. Please try it and provide feedback!

* File a [GitHub issue](https://github.com/google/go-cloud/issues)
* Post questions to the
[project's mailing list](https://groups.google.com/forum/#!forum/go-cloud)
* Send us private feedback at <go-cdk-feedback@google.com>
