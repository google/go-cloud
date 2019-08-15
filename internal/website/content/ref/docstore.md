---
title: "docstore"
date: 2019-08-14T00:00:00-08:00
aliases:
- /pages/docstore/
---

The docstore package provides an abstraction layer over common document stores
like Google Cloud Firestore, Amazon DynamoDB, and MongoDB.

<!--more-->

[How-to guide]({{< ref "/howto/docstore/_index.md" >}})<br>
[Top-level package documentation](https://godoc.org/gocloud.dev/docstore)

## Supported Services

* [GCP Firestore](https://godoc.org/gocloud.dev/docstore/gcpfirestore)
* [AWS DynamoDB](https://godoc.org/gocloud.dev/docstore/awsdynamodb)
* [MongoDB](https://godoc.org/gocloud.dev/docstore/mongodocstore)
  * AWS DocumentDB and Azure Cosmos DB have MongoDB-compatible APIs.
* [In-memory](https://godoc.org/gocloud.dev/docstore/memdocstore) - mainly
  useful for local testing

## Usage Samples

* [CLI Sample](https://github.com/google/go-cloud/tree/master/samples/gocdk-docstore)
* [Order Processor sample](https://gocloud.dev/tutorials/order/)
* [docstore package examples](https://godoc.org/gocloud.dev/docstore#pkg-examples)

