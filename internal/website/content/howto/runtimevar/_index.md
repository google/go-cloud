---
title: "Runtime Configuration"
date: 2019-07-11T12:00:00-07:00
showInSidenav: true
toc: true
---

The runtimevar package provides an easy and portable way to watch runtime
configuration variables. This guide shows how to work with runtime configuration
variables using the Go CDK.

<!--more-->

## Opening a Variable {#opening}

The first step in watching a variable is to instantiate a
[`*runtimevar.Variable`][].

The easiest way to do so is to use [`runtimevar.OpenVariable`][] and a URL pointing
to the variable, making sure you ["blank import"][] the driver package to link
it in. See [Concepts: URLs][] for more details. If you need fine-grained control
over the connection settings, you can call the constructor function in the
driver package directly (like `etcdvar.OpenVariable`).

When opening the variable, you can provide a [decoder][] parameter (either as a
query parameter for URLs, or explicitly to the constructor) to specify whether
the raw value stored in the variable is interpreted as a `string`, a `[]byte`,
or as JSON.

See the [guide below][] for usage of both forms for each supported provider.

[`*runtimevar.Variable`]: https://godoc.org/gocloud.dev/runtimevar#Variable
[`runtimevar.OpenVariable`]: https://godoc.org/gocloud.dev/runtimevar#OpenVariable
["blank import"]: https://golang.org/doc/effective_go.html#blank_import
[Concepts: URLs]: {{< ref "/concepts/urls.md" >}}
[decoder]: https://godoc.org/gocloud.dev/runtimevar#Decoder
[guide below]: {{< ref "#services" >}}

## Using a Variable {#using}

Once you have opened a `runtimevar.Variable` for the provider you want, you can
use it portably.

### Latest {#latest}

The easiest way to a `Variable` is to use the [`Variable.Latest`][] method. It
returns the latest good [`Snapshot`][] of the variable value, blocking if no
good value has *ever* been received. The dynamic type of `Snapshot.Value`
depends on the decoder you provided when creating the `Variable`.

To avoid blocking, you can pass an already-`Done` context.

{{< goexample src="gocloud.dev/runtimevar.ExampleVariable_Latest"
imports="0" >}}

[`Variable.Latest`]: https://godoc.org/gocloud.dev/runtimevar#Variable.Latest
[`Snapshot`]: https://godoc.org/gocloud.dev/runtimevar#Snapshot

### Watch {#watch}

`Variable` also has a [`Watch`][] method for obtaining the value of a variable;
it has different semantics than `Latest` and may be useful in some scenarios. We
recommend starting with `Latest` as it's conceptually simpler to work with.

[`Watch`]: https://godoc.org/gocloud.dev/runtimevar#Variable.Watch

## Supported Services {#services}

### GCP Runtime Configurator {#gcprc}

To open a variable stored in [GCP Runtime Configurator][] via a URL, you can use
the `runtimevar.OpenVariable` function as follows.

[GCP Runtime Configurator]: https://cloud.google.com/deployment-manager/runtime-configurator/

{{< goexample
"gocloud.dev/runtimevar/gcpruntimeconfig.Example_openVariableFromURL" >}}

#### GCP Constructor {#gcprc-ctor}

The [`gcpruntimeconfig.OpenVariable`][] constructor opens a Runtime Configurator
variable.

{{< goexample "gocloud.dev/runtimevar/gcpruntimeconfig.ExampleOpenVariable" >}}

[`gcpruntimeconfig.OpenVariable`]: https://godoc.org/gocloud.dev/runtimevar/gcpruntimeconfig#OpenVariable

### AWS Parameter Store {#awsps}

To open a variable stored in [AWS Parameter Store][] via a URL, you can use the
`runtimevar.OpenVariable` function as follows.

{{< goexample
"gocloud.dev/runtimevar/awsparamstore.Example_openVariableFromURL" >}}

[AWS Parameter Store]:
https://docs.aws.amazon.com/systems-manager/latest/userguide/systems-manager-parameter-store.html

#### AWS Constructor {#awsps-ctor}

The [`awsparamstore.OpenVariable`][] constructor opens a Parameter Store
variable.

{{< goexample "gocloud.dev/runtimevar/awsparamstore.ExampleOpenVariable" >}}

[`awsparamstore.OpenVariable`]:
https://godoc.org/gocloud.dev/runtimevar/awsparamstore#OpenVariable

### etcd {#etcd}

The Go CDK supports using [etcd][] for storing variables locally or remotely. To
open a variable stored in `etcd` via a URL, you can use the
`runtimevar.OpenVariable` function as follows.

{{< goexample "gocloud.dev/runtimevar/etcdvar.Example_openVariableFromURL" >}}

[etcd]: https://etcd.io/

#### Etcd Constructor {#etcd-ctor}

The [`etcdvar.OpenVariable`][] constructor opens an etcd variable.

[`etcdvar.OpenVariable`]:
https://godoc.org/gocloud.dev/runtimevar/etcdvar#OpenVariable

{{< goexample "gocloud.dev/runtimevar/etcdvar.ExampleOpenVariable" >}}
