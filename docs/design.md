# Design Decisions

This document outlines important design decisions made for this repository and
attempts to provide succinct rationales.

## Drivers and User-Facing Types

The generic APIs that Go X Cloud exports (like [`blob.Bucket`][] or
[`runtimevar.Variable`][] are concrete types, not interfaces. To understand why,
imagine if we used a plain interface:

![Diagram showing user code depending on blob.Bucket, which is implemented by
awsblob.Bucket.](img/abstract-type-no-driver.png)

Consider the [`Bucket.NewReader` method][], which is defined to be the same as
calling `NewRangeReader` with some default argument values. If `blob.Bucket` was
an interface, each implementation of `blob.Bucket` would have to copy an
implementation of `NewReader`. In this simple example, this might not be that
bad, but it does not scale for more complex behaviors: conformance tests would
need to ensure that each operation actually behaves in the way that the docs
describe. This makes the interfaces hard to implement, which runs counter to the
goals of the project.

Instead, we follow the example of [`database/sql`][] and separate out the
implementation-agnostic logic from the interface. We call the interface the
**driver type** and the wrapper the **user-facing type**. Visually, it looks
like this:

![Diagram showing user code depending on blob.Bucket, which holds a
driver.Bucket implemented by awsblob.Bucket.](img/abstract-type.png)

This has a number of benefits:

-  The user-facing type can perform higher level logic without making the
   interface complex to implement. In the blob example, the user-facing type can
   have a `NewReader` method that is guaranteed to act the same, regardless of
   underlying implementation, without requiring conformance testing.
-  Methods can be added to the user-facing type without breaking compatibility.
-  As new operations on the driver are added as new optional interfaces, the
   user-facing type can hide the need for type-assertions from the user.

As a rule, if a method `Foo` has the same inputs and semantics in the
user-facing type and the driver type, then the driver method may be called
`Foo`, even though the return signatures may differ. Otherwise, the driver
method name must be different to reduce confusion.

[`blob.Bucket`]: https://godoc.org/github.com/google/go-x-cloud/blob#Bucket
[`runtimevar.Variable`]: https://godoc.org/github.com/google/go-x-cloud/runtimevar#Variable
[`Bucket.NewReader` method]: https://godoc.org/github.com/google/go-x-cloud/blob#Bucket.NewReader
[`database/sql`]: https://godoc.org/database/sql

## Errors

-   The callee is expected to return `error`s with messages that include
    information about the particular call, as opposed to the caller adding this
    information. This aligns with common Go practice.

-   Prefer to keep details of returned `error`s unspecified. The most common
    case is that the caller will only care whether an operation succeeds or not.

-   If certain kinds of `error`s are interesting for the caller of a function or
    non-interface method to distinguish, prefer to expose additional information
    through the use of predicate functions like
    [`os.IsNotExist`](https://golang.org/pkg/os/#IsNotExist). This allows the
    internal representation of the `error` to change over time while being
    simple to use.

-   If it is important to distinguish different kinds of `error`s returned from
    an interface method, then the `error` should implement extra methods and the
    interface should document these assumptions. Just remember that each method
    can be implemented independently: if one method is mutually exclusive with
    another, it would be better to return a more complicated data type from one
    methodthan to have separate methods.

-   Transient network errors should be handled by an interface's implementation
    and not bubbled up as a distinguishible error through a generic interface.
    Retry logic is best handled as low in the stack as possible to avoid
    [cascading failure][]. APIs should try to surface "permanent" errors (e.g.
    malformed request, bad permissions) where appropriate so that application
    logic does not attempt to retry idempotent operations, but the
    responsibility is largely on the library, not on the application.

[cascading failure]: https://landing.google.com/sre/book/chapters/addressing-cascading-failures.html
