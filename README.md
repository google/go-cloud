# Go X Cloud

[![Build Status](https://travis-ci.com/google/go-cloud.svg?branch=master)][travis]
[![godoc](https://godoc.org/github.com/google/go-cloud?status.svg)][godoc]

This repository is a design experiment to make it possible to write Go programs
across multiple cloud platforms.

```
$ go get -u github.com/google/go-cloud
```

See the examples under `aws/awscloud` or `gcp/gcpcloud` for how to get started.

[travis]: https://travis-ci.com/google/go-cloud
[godoc]: http://godoc.org/github.com/google/go-cloud

## Features

Go X Cloud provides generic APIs for:

-   Unstructured binary (blob) storage
-   Variables that change at runtime (configuration)
-   Connecting to MySQL databases
-   Server startup and diagnostics: request logging, tracing, and health
    checking
