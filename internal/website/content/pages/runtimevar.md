---
title: "Runtimevar"
---

Package `runtimevar` provides easy and portable access to the latest value of
remote configuration variables.

Top-level package documentation: https://godoc.org/gocloud.dev/runtimevar

Supported providers:

* [AWS Paramstore](https://godoc.org/gocloud.dev/runtimevar/paramstore)
* [GCP Runtime
  Configurator](https://godoc.org/gocloud.dev/runtimevar/runtimeconfigurator)
* [blobvar](https://godoc.org/gocloud.dev/runtimevar/blobvar) - a blob-backed
  implementation supported by any provider that has blob support
* [Local read-only constant
  vars](https://godoc.org/gocloud.dev/runtimevar/constantvar) - an in-memory
  local implementation, mainly useful for testing
* [Local etcd runtimevar](https://godoc.org/gocloud.dev/runtimevar/etcdvar) - a
  local implementation using the [etcd distributed key-value
  store](https://github.com/etcd-io/etcd)

Usage samples:

* [Guestbook
  sample](https://github.com/google/go-cloud/tree/master/samples/guestbook)
* [runtimevar package
  examples](https://godoc.org/gocloud.dev/runtimevar#pkg-examples)
