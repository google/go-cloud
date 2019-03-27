---
title: "Keep Secrets in Configuration"
date: 2019-03-21T17:45:27-07:00
draft: true
weight: 3
---

Once you have [opened a secrets keeper][] for the secrets provider you want,
you can use a secrets keeper to access sensitive configuration stored in an
encrypted `runtimevar`.

**TODO(light):** This change after https://github.com/google/go-cloud/issues/1667

First, you create a [`*runtimevar.Decoder`][] configured to use your secrets
keeper using [`runtimevar.DecryptDecode`][]. In this example, we assume the
data is a plain string, but the configuration could be a more structured
type. (**TODO(light)**: link to `runtimevar` guide when ready.)

```go
decodeFunc := runtimevar.DecryptDecode(ctx, keeper, runtimevar.StringDecode)
decoder := runtimevar.NewDecoder("", decodeFunc)
```

Then you can pass the decoder to the runtime configuration provider of your
choice. (**TODO(light)**: Link to opening `runtimevar` guide when ready.)

[opened a secrets keeper]: {{< ref "./open-keeper.md" >}}
[`*runtimevar.Decoder`]: https://godoc.org/gocloud.dev/runtimevar#Decoder
[`runtimevar.DecryptDecode`]: https://godoc.org/gocloud.dev/runtimevar#DecryptDecode

