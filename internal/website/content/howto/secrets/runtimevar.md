---
title: "Keep Secrets in Configuration"
date: 2019-03-21T17:45:27-07:00
weight: 3
---

Once you have [opened a secrets keeper][] for the secrets provider you want,
you can use a secrets keeper to access sensitive configuration stored in an
encrypted `runtimevar`.

First, you create a [`*runtimevar.Decoder`][] configured to use your secrets
keeper using [`runtimevar.DecryptDecode`][]. In this example, we assume the
data is a plain string, but the configuration could be a more structured
type.

{{< goexample src="gocloud.dev/runtimevar.ExampleDecryptDecode" imports="0" >}}

Then you can pass the decoder to the runtime configuration provider of your
choice. See the [Runtime Configuration How-To Guide][] for more on how to set up
runtime configuration.

[opened a secrets keeper]: {{< ref "./open-keeper.md" >}}
[Runtime Configuration How-To Guide]: {{< ref "/howto/runtimevar/_index.md" >}}
[`*runtimevar.Decoder`]: https://godoc.org/gocloud.dev/runtimevar#Decoder
[`runtimevar.DecryptDecode`]: https://godoc.org/gocloud.dev/runtimevar#DecryptDecode

