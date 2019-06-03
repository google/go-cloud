---
title: "Structuring Portable Code"
date: 2019-06-03T07:34:22-07:00
weight: 1
---

The Go CDK's APIs are intentionally structured to make it easier to separate
your application's core logic from the details of the services it is using.

<!--more-->

## Motivation

Consider the [uploader tutorial][]. Without the Go CDK, we would have had to
write a code path for Amazon's Simple Storage Service (S3) and another code
path for Google Cloud Storage (GCS). That would work, but it would be
tedious. We would have to learn the semantics of uploading files to both blob
storage services. Even worse, we would have two code paths that effectively
do the same thing, but would have to be maintained separately. It would be
much nicer if we could write the upload logic once and reuse it across
providers. That's exactly the kind of [separation of concerns][] that the Go
CDK makes possible.

(More details available in the [Go CDK design doc][Developers and Operators].)

[Developers and Operators]: https://github.com/google/go-cloud/blob/master/internal/docs/design.md#developers-and-operators
[separation of concerns]: https://en.wikipedia.org/wiki/Separation_of_concerns
[uploader tutorial]: {{< ref "/tutorials/cli-uploader.md" >}}

## Portable Types and Drivers

The portable APIs that the Go CDK exports (like [`blob.Bucket`][] or
[`runtimevar.Variable`][]) are concrete types, not interfaces. To understand
why, imagine if we used a plain interface:

{{< figure class="FullWidthFigure" src="portable-type-no-driver.png" link="portable-type-no-driver.png" alt="Diagram showing user code depending on blob.Bucket, which is implemented by awsblob.Bucket." >}}

Consider the [`Bucket.NewWriter` method][], which infers the content type of the
blob based on the first bytes written to it. If `blob.Bucket` was an interface,
each implementation of `blob.Bucket` would have to replicate this behavior
precisely. This does not scale: conformance tests would be needed to ensure that
each interface method actually behaves in the way that the docs describe. This
makes the interfaces hard to implement, which runs counter to the goals of the
project.

Instead, we follow the example of [`database/sql`][] and separate out the
implementation-agnostic logic from the interface. The implementation-agnostic
logic-containing concrete type is the **portable type**. We call the interface
the **driver**. Visually, it looks like this:

{{< figure class="FullWidthFigure" src="portable-type.png" link="portable-type.png" alt="Diagram showing user code depending on blob.Bucket, which holds a driver.Bucket implemented by awsblob.Bucket." >}}

This has a number of benefits:

-   The portable type can perform higher level logic without making the
    interface complex to implement. In the blob example, the portable type's
    `NewWriter` method can do the content type detection and then pass the final
    result to the driver type.
-   Methods can be added to the portable type without breaking compatibility.
    Contrast with adding methods to an interface, which is a breaking change.
-   As new operations on the driver are added as new optional interfaces, the
    portable type can hide the need for type-assertions from the user.

(More details available in the [Go CDK design doc][Portable Types and Drivers].)

[Portable Types and Drivers]: https://github.com/google/go-cloud/blob/master/internal/docs/design.md#portable-types-and-drivers
[`blob.Bucket`]: https://godoc.org/github.com/google/go-cloud/blob#Bucket
[`runtimevar.Variable`]:
https://godoc.org/github.com/google/go-cloud/runtimevar#Variable
[`Bucket.NewWriter` method]:
https://godoc.org/github.com/google/go-cloud/blob#Bucket.NewWriter
[`database/sql`]: https://godoc.org/database/sql

## Best Practices

-  **Create portable types as close to program startup as possible.** Since
   creation of a portable type requires using driver-specific setup, this
   separates your driver-specific details from the rest of your application.
-  **Pass portable types around as arguments or struct fields instead of as
   package variables.** This allows you to easily swap out the portable type
   for a local implementation in unit tests. It also enables you to use
   dependency injection tools like [Wire][] to set up your application.
-  **Avoid using [`As`][] functions when possible.** Using driver-specific
   options makes it harder to test your code with confidence or migrate to
   another driver later. If your application needs to use driver-specific
   options, try to make it so that other drivers fall back gracefully. For
   example, you may need to use a particular ACL setting for a write to a Google
   Cloud Storage bucket. When testing for the driver-specific write options,
   don't return an error if the `As` function doesn't have the right type. That
   way, when running against an in-memory bucket for tests, the write will still
   occur and can be observed. Leave provider-specific checks to integration
   tests.

[`As`]: {{< ref "/concepts/as.md#as" >}}
[Wire]: https://github.com/google/wire
