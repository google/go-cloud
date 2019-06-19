---
title: "Fetching the latest value of a variable"
date: 2019-06-18T15:46:26-07:00
---

Working with runtime variables using the Go CDK takes two steps:

1. Open the variable with the `runtimevar` provider of your choice.
2. As many times as needed, use the `Latest` or `Watch` methods to fetch the
   value of the variable.

## Constructors versus URL openers

TODO: follow decision per email thread

## GCP Runtime Configurator {#rc-ctor}

To open a variable stored in [GCP Runtime Configurator][] via a URL, you can use
the `runtimevar.OpenVariable` function as follows:

{{< goexample
"gocloud.dev/runtimevar/gcpruntimeconfig.Example_openVariableFromURL" >}}

[GCP Runtime Configurator]: https://cloud.google.com/deployment-manager/runtime-configurator/
