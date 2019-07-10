---
title: "Runtime Configuration"
date: 2019-06-18T14:51:12-07:00
showInSidenav: true
toc: true
---

Working with runtime variables using the Go CDK takes two steps:

1. Open the variable with the `runtimevar` provider of your choice. You can
   provide a [decoder][] parameter to specify whether the raw value stored
   in the variable is interpreted as a `string`, a `[]byte` or as JSON.
2. Use the `Latest` method to fetch the value of the variable.

[decoder]: https://godoc.org/gocloud.dev/runtimevar#Decoder
[GCP Runtime Configurator]: https://cloud.google.com/deployment-manager/runtime-configurator/

## Constructors versus URL openers

The easiest way to open a variable is using [`runtimevar.OpenVariable`][] and a
URL pointing to the variable, making sure you ["blank import"][] the driver
package to link it in. See [Concepts: URLs][] for more details. If you need
fine-grained control over the connection settings, you can call the constructor
function in the driver package directly (like `awsparamstore.OpenVariable`).
This guide shows how to use both forms for each provider.

[Concepts: URLs]: {{< ref "/concepts/urls.md" >}}
["blank import"]: https://golang.org/doc/effective_go.html#blank_import
[`runtimevar.OpenVariable`]:
https://godoc.org/gocloud.dev/runtimevar#OpenVariable

## GCP Runtime Configurator {#gcp-url}

To open a variable stored in [GCP Runtime Configurator][] via a URL, you can use
the `runtimevar.OpenVariable` function as follows.

{{< goexample
"gocloud.dev/runtimevar/gcpruntimeconfig.Example_openVariableFromURL" >}}

## GCP Runtime Configurator Constructor {#rc-ctor}

The [`gcpruntimeconfig.OpenVariable`][] constructor opens a Runtime Configurator
variable.

{{< goexample
"gocloud.dev/runtimevar/gcpruntimeconfig.ExampleOpenVariable" >}}

[`gcpruntimeconfig.OpenVariable`]: https://godoc.org/gocloud.dev/runtimevar/gcpruntimeconfig#OpenVariable

## AWS Parameter Store {#ps-url}

To open a variable stored in [AWS Parameter Store][] via a URL, you can use the
`runtimevar.OpenVariable` function as follows.

{{< goexample
"gocloud.dev/runtimevar/awsparamstore.Example_openVariableFromURL" >}}

[AWS Parameter Store]:
https://docs.aws.amazon.com/systems-manager/latest/userguide/systems-manager-parameter-store.html

## AWS Parameter Store Constructor {#ps-ctor}

The [`awsparamstore.OpenVariable`][] constructor opens a Parameter Store
variable.

{{< goexample "gocloud.dev/runtimevar/awsparamstore.ExampleOpenVariable" >}}

[`awsparamstore.OpenVariable`]:
https://godoc.org/gocloud.dev/runtimevar/awsparamstore#OpenVariable

## etcd {#etcd-url}

The Go CDK supports using [etcd][] for storing variables locally or
remotely. To open a variable stored in `etcd` via a URL, you can use the
`runtimevar.OpenVariable` function as follows.

{{< goexample
"gocloud.dev/runtimevar/etcdvar.Example_openVariableFromURL" >}}

[etcd]: https://etcd.io/

## etcd Constructor

The [`etcdvar.OpenVariable`][] constructor opens an etcd variable.

[`etcdvar.OpenVariable`]:
https://godoc.org/gocloud.dev/runtimevar/etcdvar#OpenVariable

{{< goexample "gocloud.dev/runtimevar/etcdvar.ExampleOpenVariable" >}}

## Fetching the latest value {#latest}

Once we have an open variable, we can use the [`Variable.Latest`][] method to
fetch its latest value. This method returns the latest good [`Snapshot`][] of
the variable value, blocking if no good value has *ever* been received. The
dynamic type of `Snapshot.Value` depends on the decoder you provided when
creating the `Variable`.

To avoid blocking, you can pass an already-`Done` context.

{{< goexample src="gocloud.dev/runtimevar.ExampleVariable_Latest"
imports="0" >}}

## Watch {#watch}

Type [`Variable`][] also has a [`Watch`][] method for obtaining the value of
a variable; it has different semantics than [`Variable.Latest`][] and may be
useful in some scenarios. We recommend starting with `Latest` as it's
conceptually simpler to work with.

[`Variable.Latest`]: https://godoc.org/gocloud.dev/runtimevar#Variable.Latest
[`Variable`]: https://godoc.org/gocloud.dev/runtimevar#Variable
[`Snapshot`]: https://godoc.org/gocloud.dev/runtimevar#Snapshot
[`Watch`]: https://godoc.org/gocloud.dev/runtimevar#Variable.Watch
