[![Build Status](https://travis-ci.com/google/go-cloud.svg?branch=master)][travis]
[![godoc](https://godoc.org/github.com/google/go-cloud?status.svg)][godoc]

# The Go X Cloud Project
_Write once, run any cloud ☁️_

The Go X Cloud Project is an initiative that will allow application developers to seamlessly deploy cloud applications on any combination of cloud providers. It does this by providing stable, idiomatic interfaces for common uses like storage, PubSub and databases. Think `database/sql` for cloud.

A key part of the project is to also provide a code generator called Wire. Wire is invoked using `go generate` and creates human-readable code that only imports the cloud SDKs for providers you use. This allows Go X Cloud to grow to support any number of cloud providers, without increasing compile times or binary sizes, and avoiding any side effects from `init()` functions.

Imagine writing this to read from blob storage (like Google Cloud Storage or S3):

```go
	blobReader, err := app.bucket.NewReader(context.Background(), "my-blob")
```

and being able to run that code on any cloud you want, avoiding all the ceremony of cloud-specific authorization, tracing, SDKs and all the other code required to make an application portable across cloud providers. 
	
## Project status
Go X Cloud is currently in __pre-alpha__. API breakage may occur at any time, import paths may change, compiles may break. We encourage contributions to help evolve Go X Cloud to meet your needs, but under no circumstances should it be deployed to any production environments. Sadness will ensue :(

The GitHub repository at [google/go-cloud](https://github.com/google/go-cloud) currently contains Google Cloud Platform and Amazon Web Services implementations as examples to prove everything is working. We are moving towards a goal where implementations of clouds sit in separate repositories, regardless of whether they are written by us or are externally-owned contributions. During this pre-alpha phase where APIs continue to evolve, we are storing GCP and AWS in this repository in order to make atomic changes. This will change in the future.

## Installation instructions
Installation is easy, but does require `vgo`. `vgo` is not yet stable, and so builds may break with `vgo` changes, but experience has shown this to be rare.

```
$ go get -u golang.org/x/vgo
$ vgo get -u github.com/google/go-cloud
```
Go X Cloud builds at the tip release of Go, previous Go versions may compile but are not supported.

## Example application
An example application which runs a guestbook app (just like it's [1999!](https://www.oocities.org/)) on Google Cloud Platform or Amazon Web Services can be found in [`samples/guestbook`](https://github.com/google/go-cloud/tree/master/samples/guestbook). The instructions take about 30 minutes to follow, and uses [Terraform](http://terraform.io) to automatically provision the cloud resources needed for the application.

## Current Features

Go X Cloud provides generic APIs for:

-   Unstructured binary (blob) storage
-   Variables that change at runtime (configuration)
-   Connecting to MySQL databases
-   Server startup and diagnostics: request logging, tracing, and health
    checking
