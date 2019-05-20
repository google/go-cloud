---
title: Using provider-specific APIs
date: 2019-05-10T11:17:09-07:00
---

It is not feasible or desirable for APIs like `blob.Bucket` to encompass the
full functionality of every provider. Rather, we intend to provide a subset
of the most commonly used functionality. There will be cases where a
developer wants to access provider-specific functionality, such as unexposed
APIs or data fields, errors, or options. This can be accomplished using `As`
functions.

<!--more-->

## `As` {#as}

`As` functions in the APIs provide the user a way to escape the Go CDK
abstraction to access provider-specific types. They might be used as an
interim solution until a feature request to the Go CDK is implemented. Or,
the Go CDK may choose not to support specific features, and the use of `As`
will be permanent.

Using `As` implies that the resulting code is no longer portable; the
provider-specific code will need to be ported in order to switch providers.
Therefore, it should be avoided if possible.

Each API includes examples demonstrating how to use its various `As`
functions, and each provider implementation documents what types it supports
for each.

Usage:

1. Declare a variable of the provider-specific type you want to access.
2. Pass a pointer to it to `As`.
3. If the type is supported, `As` will return `true` and copy the
   provider-specific type into your variable. Otherwise, it will return `false`.

Provider-specific types that are intended to be mutable will be exposed
as a pointer to the underlying type.
