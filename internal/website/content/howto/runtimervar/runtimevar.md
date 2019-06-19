---
title: "Fetching the latest value of a variable"
date: 2019-06-18T15:46:26-07:00
---

Working with runtime variables using the Go CDK takes two steps:

1. Open the variable with the `runtimevar` provider of your choice.
2. As many times as needed, use the `Latest` or `Watch` methods to fetch the
   value of the variable.

TODO: start with an end-to-end walkthrough

## Constructors versus URL openers

TODO: follow decision per email thread

## GCP Runtime Configurator {#rc-url}

To open a variable stored in [GCP Runtime Configurator][] via a URL, you can use
the `runtimevar.OpenVariable` function as follows. It uses the `string` decoder;
see section TODO for a discussion of decoders.

{{< goexample
"gocloud.dev/runtimevar/gcpruntimeconfig.Example_openVariableFromURL" >}}

[GCP Runtime Configurator]: https://cloud.google.com/deployment-manager/runtime-configurator/

## GCP Runtime Configurator Constructor {#rc-ctor}

The [`gcpruntimeconfig.OpenVariable`][] constructor opens a Runtime Configurator
variable.

{{< goexample
"gocloud.dev/runtimevar/gcpruntimeconfig.Example_openVariableHowto" >}}

[`gcpruntimeconfig.OpenVariable`]: https://godoc.org/gocloud.dev/runtimevar/gcpruntimeconfig#OpenVariable

## Fetching the latest value {#latest}

Once we have an open variable, we can use the [`Variable.Latest`][] method to
fetch its latest value. This method returns the latest good [`Snapshot`][] of
the variable value, blocking if no good value has ever been received. To avoid
blocking, you can pass an already-`Done` context.

{{< goexample src="gocloud.dev/runtimevar.Example_latestStringVariableHowto"
imports="0" >}}

[`Variable.Latest`]: https://godoc.org/gocloud.dev/runtimevar#Variable.Latest
[`Snapshot`]: https://godoc.org/gocloud.dev/runtimevar#Snapshot
