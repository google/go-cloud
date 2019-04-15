---
title: "Runtimevar"
date: 2019-02-21T16:21:29-08:00
aliases:
- /pages/runtimevar/
---

Package `runtimevar` provides easy and portable access to the latest value of
remote configuration variables.

<!--more-->

Top-level package documentation: https://godoc.org/gocloud.dev/runtimevar

## Supported Providers

* [AWS Paramstore](https://godoc.org/gocloud.dev/runtimevar/awsparamstore)
* [GCP Runtime
  Configurator](https://godoc.org/gocloud.dev/runtimevar/gcpruntimeconfig)
* [blobvar](https://godoc.org/gocloud.dev/runtimevar/blobvar) - a blob-backed
  implementation supported by any provider that has [blob support]({{< relref "blob.md#supported-providers">}})
* [httpvar](https://godoc.org/gocloud.dev/runtimevar/httpvar) - an
  implementation that fetches an arbitrary HTTP endpoint
* [Local read-only constant
  vars](https://godoc.org/gocloud.dev/runtimevar/constantvar) - an in-memory
  local implementation, mainly useful for testing
* [Local etcd runtimevar](https://godoc.org/gocloud.dev/runtimevar/etcdvar) - a
  local implementation using the [etcd distributed key-value
  store](https://github.com/etcd-io/etcd)

## Usage Samples

* [Guestbook
  sample](https://github.com/google/go-cloud/tree/master/samples/guestbook)
* [runtimevar package
  examples](https://godoc.org/gocloud.dev/runtimevar#pkg-examples)
