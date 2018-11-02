<img src="logo-gopherblue.png" alt="" width="448" height="221">

# The Go Cloud Project
_Write once, run on any cloud ☁️_

[![Build Status](https://travis-ci.com/google/go-cloud.svg?branch=master)][travis]
[![godoc](https://godoc.org/github.com/google/go-cloud?status.svg)][godoc]

The Go Cloud Project is an initiative that will allow application developers to
seamlessly deploy cloud applications on any combination of cloud providers. It
does this by providing stable, idiomatic interfaces for common uses like storage
and databases. Think `database/sql` for cloud products.

Imagine writing this to read from blob storage (like Google Cloud Storage or
S3):

```go
blobReader, err := bucket.NewReader(context.Background(), "my-blob")
```

and being able to run that code on any cloud you want, avoiding all the ceremony
of cloud-specific authorization, tracing, SDKs and all the other code required
to make an application portable across cloud platforms.

Another key part of the project is a code generator called
[Wire](https://github.com/google/go-cloud/blob/master/wire/README.md). It
creates human-readable code that only imports the cloud SDKs for providers you
use. This allows Go Cloud to grow to support any number of cloud providers,
without increasing compile times or binary sizes, and avoiding any side effects
from `init()` functions.


You can learn more about the project from our [announcement blog post][], or our
talk at Next 2018:

[![Video: Building Go Applications for the Open Cloud (Cloud Next '18)](https://img.youtube.com/vi/_2ZwhvIkgek/0.jpg)][video]

[announcement blog post]: https://blog.golang.org/go-cloud
[godoc]: https://godoc.org/github.com/google/go-cloud
[travis]: https://travis-ci.com/google/go-cloud
[video]: https://www.youtube.com/watch?v=_2ZwhvIkgek

## Installation instructions

```shell
go get github.com/google/go-cloud
go get github.com/google/go-cloud/wire/cmd/wire
```

Go Cloud builds at the latest stable release of Go. Previous Go versions may
compile but are not supported.

## Samples

[`samples/tutorial`][tutorial] shows how to get started with the project by
using blob storage.

[`samples/guestbook`][guestbook] contains an example guestbook application (just
like it's 1999!) that can be run locally, on Google Cloud Platform or on Amazon
Web Services. The instructions take about 5 minutes to follow if running
locally. If you want to see the guestbook app running on cloud resources, it
will take about 30 minutes to follow, and uses [Terraform](http://terraform.io)
to automatically provision the resources needed.

[tutorial]: https://github.com/google/go-cloud/tree/master/samples/tutorial
[guestbook]: https://github.com/google/go-cloud/tree/master/samples/guestbook

## Project status

**This project is in alpha and is not yet suitable for production.**

While in alpha, the API is subject to breaking changes.

## Current features

Go Cloud provides generic APIs for:
-   Unstructured binary (blob) storage
-   Variables that change at runtime (configuration)
-   Connecting to MySQL databases
-   Server startup and diagnostics: request logging, tracing, and health
    checking

## Contributing

Thank you for your interest in contributing to Go Cloud! :heart:

Everyone is welcome to contribute to Go Cloud, whether it's in the form of code,
documentation, bug reports, feature requests, or anything else. We encourage you
to experiment with Go Cloud and make contributions to help evolve it to meet
your needs!

The GitHub repository at [google/go-cloud][go-cloud] currently contains
[Google Cloud Platform][gcp] and [Amazon Web Services][aws] implementations as
examples to prove everything is working. Unfortunately, we cannot support every
cloud provider directly from the project.

If you create a repository that implements the Go Cloud interfaces for other
cloud providers, let us know!
We would be happy to link to it here and give you a heads-up before making any
breaking changes.

See [the contributing guide](./CONTRIBUTING.md) for more details.

[go-cloud]: https://github.com/google/go-cloud
[gcp]: http://cloud.google.com
[aws]: http://aws.amazon.com

## Community

You can contact us on the [go-cloud mailing list][].

This project is covered by the Go [Code of Conduct][].

[Code of Conduct]: ./CODE_OF_CONDUCT.md
[go-cloud mailing list]: https://groups.google.com/forum/#!forum/go-cloud
