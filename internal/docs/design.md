# Design Decisions

This document outlines important design decisions made for this repository and
attempts to provide succinct rationales.

This is a [Living Document](https://en.wikipedia.org/wiki/Living_document). The
decisions in here are not set in stone, but simply describe our current thinking
about how to guide the Go Cloud project. While it is useful to link to this
document when having discussions in an issue, it is not to be used as a means of
closing issues without discussion at all. Discussion on an issue can lead to
revisions of this document.

## Developers and Operators

Go Cloud is designed with two different personas in mind: the developer and the
operator. In the world of DevOps, these may be the same person. A developer may
be directly deploying their application into production, especially on smaller
teams. In a larger organization, these may be different teams entirely, but
working closely together. Regardless, these two personas have two very different
ways of looking at a Go program:

-   The developer persona wants to write business logic that is agnostic of
    underlying cloud provider. Their focus is on making software correct for the
    requirements at hand.
-   The operator persona wants to incorporate the business logic into the
    organization's policies and provision resources for the logic to run. Their
    focus is making software run predictably and reliably with the resources at
    hand.

Go Cloud uses Go interfaces at the boundary between these two personas: a
developer is meant to use an interface, and an operator is meant to provide an
implementation of that interface. This distinction prevents Go Cloud going down
a path of complexity that makes application portability difficult. The
[`blob.Bucket`] type is a prime example: the API does not provide a way of
creating a new bucket. To properly and safely create such a bucket requires
careful consideration, getting something like ACLs wrong could lead to a
catastrophic data leak. To generate the ACLs correctly requires modeling of IAM
users and roles for each cloud platform, and some way of mapping those users and
roles across clouds. While not impossible, the level of complexity and the high
likelihood of a leaky abstraction leads us to believe this is not the right
direction for Go Cloud.

Instead of adding large amounts of leaky complexity to Go Cloud, we expect the
operator role to handle the management of non-portable platform-specific
resources. An implementor of the `Bucket` interface does not need to determine
the content type of incoming data, as that is a developer's concern. This
separation of concerns allows these two personas to communicate using a shared
language while focusing on their respective areas of expertise.

[`blob.Bucket`]: https://godoc.org/github.com/google/go-cloud/blob#Bucket

## Drivers and User-Facing Types

The generic APIs that Go Cloud exports (like [`blob.Bucket`][] or
[`runtimevar.Variable`][] are concrete types, not interfaces. To understand why,
imagine if we used a plain interface:

![Diagram showing user code depending on blob.Bucket, which is implemented by
awsblob.Bucket.](img/user-facing-type-no-driver.png)

Consider the [`Bucket.NewWriter` method][], which infers the content type of the
blob based on the first bytes written to it. If `blob.Bucket` was an interface,
each implementation of `blob.Bucket` would have to replicate this behavior
precisely. This does not scale: conformance tests would be needed to ensure that
each interface method actually behaves in the way that the docs describe. This
makes the interfaces hard to implement, which runs counter to the goals of the
project.

Instead, we follow the example of [`database/sql`][] and separate out the
implementation-agnostic logic from the interface. We call the interface the
**driver type** and the wrapper the **user-facing type**. Visually, it looks
like this:

![Diagram showing user code depending on blob.Bucket, which holds a
driver.Bucket implemented by awsblob.Bucket.](img/user-facing-type.png)

This has a number of benefits:

-   The user-facing type can perform higher level logic without making the
    interface complex to implement. In the blob example, the user-facing type's
    `NewWriter` method can do the content type detection and then pass the final
    result to the driver type.
-   Methods can be added to the user-facing type without breaking compatibility.
    Contrast with adding methods to an interface, which is a breaking change.
-   As new operations on the driver are added as new optional interfaces, the
    user-facing type can hide the need for type-assertions from the user.

As a rule, if a method `Foo` has the same inputs and semantics in the
user-facing type and the driver type, then the driver method may be called
`Foo`, even though the return signatures may differ. Otherwise, the driver
method name should be different to reduce confusion.

[`runtimevar.Variable`]:
https://godoc.org/github.com/google/go-cloud/runtimevar#Variable
[`Bucket.NewWriter` method]:
https://godoc.org/github.com/google/go-cloud/blob#Bucket.NewWriter
[`database/sql`]: https://godoc.org/database/sql

## Driver Naming Convention

Inside this repository, we name packages that handle cloud services after the
service name, not the providing cloud (`s3blob` instead of `awsblob`). While a
cloud provider may provide a unique offering for a particular API, they may not
always provide only one, so distinguishing them in this way keeps the API
symbols stable over time.

The exception to this rule is if the name is not unique across providers. The
canonical example is `gcpkms` and `awskms`.

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
    method than to have separate methods.

-   Transient network errors should be handled by an interface's implementation
    and not bubbled up as a distinguishable error through a generic interface.
    Retry logic is best handled as low in the stack as possible to avoid
    [cascading failure][]. APIs should try to surface "permanent" errors (e.g.
    malformed request, bad permissions) where appropriate so that application
    logic does not attempt to retry idempotent operations, but the
    responsibility is largely on the library, not on the application.

[cascading failure]:
https://landing.google.com/sre/book/chapters/addressing-cascading-failures.html

## Escape Hatches

It is not feasible or desirable for APIs like [`blob.Bucket`] to encompass the
full functionality of every provider. Rather, we intend to provide a subset of
the most commonly used functionality. There will be cases where a developer
wants to access provider-specific functionality, which might consist of:

1.  **Top-level APIs**. For example, `blob` does not expose a `Copy`, but some
    provider might.
1.  **Data fields**. For example, **blob** exposes a few attributes like
    ContentType and Size, but S3 and GCS both have many more.
1.  **Options**. Different providers may support different options for
    functionality.

**Escape hatches** provide the user a way to escape the Go Cloud abstraction to
access provider-specific functionality. They might be used as an interim
solution until a feature request to Go Cloud is implemented. Or, Go Cloud may
choose not to support specific features, and the escape hatch will be permanent.
As an example, both S3 and GCS blobs have the concept of ACLs, but it might be
difficult to represent them in a generic way (although, we have not tried).

Using an escape hatch implies that the resulting code is no longer portable; the
escape hatched code will need to be ported in order to switch providers.
Therefore, they should be avoided if possible.

### Ways To Escape Hatch

Users can always access the provider service directly, by constructing the
top-level handle and making API calls, bypassing Go Cloud.

*   For top-level operations, this may be fine, although it might require a
    bunch of plumbing code to pass the provider service handle to where it is
    needed.
*   For data objects, it implies dropping Go Cloud entirely; for example,
    instead of using `blob.Reader` to read a blob, the user would have to use
    the provider-specific method for reading.

This approach exists today. Due to its shortcomings, we are designing a second
approach. The proposal is to extend Go Cloud APIs, at the top-level (e.g.
`blob.Bucket`) and in data objects, to expose provider-specific handles. For
example:

```
// The existing blob.Reader exposes some blob attributes, but not everything
// that every provider exposes.
type Reader struct {
    ...
    Size        int64
    ContentType string
    ...

    // New field!
    Sys interface{}
}
```

The name `Sys` is modeled after
[examples](https://golang.org/pkg/os/#ProcessState.Sys) in the `os` package.
Using the `Sys` escape hatch to read an S3-specific attribute would look
something like this:

```
if r, err := bucket.NewReader(ctx, "foo.txt"); err == nil {
  acls := r.Sys.(*s3.GetObjectOutput).ACLs
  ...
}
```

Each provider implementation would document what type it returns for each of the
escape hatch functions. `nil` is a valid return value for providers that don't
support the escape hatch.

We are also considering an alternative to `Sys` that would look something like
this:

```
var s3obj s3.GetObjectOutput
if r.As(&s3obj) {
  acls := s3obj.ACLs
  ...
}
```

This alternative allows providers to map to multiple types.

Design discussions regarding escape hatches are ongoing; we welcome input either
on the [tracking issue](https://github.com/google/go-cloud/issues/470) or on the
[mailing list](https://groups.google.com/forum/#!forum/go-cloud).

## Tests

Portable API integration tests require developer-specific resources to be
created and destroyed. We use [Terraform](http://terraform.io) to do so, and
record the resource info and network interactions so that they can be replayed
as fast and repeatable unit tests.

### Replay Mode

Tests normally run in replay mode. In this mode, they don't require any
provisioned resources or network interactions. Replay tests verify that:

-   The same test inputs produce the same requests (e.g., HTTP requests) to the
    cloud provider. Some parts of the request may be dynamic (e.g., dates in the
    HTTP request headers), so the replay tests do some scrubbing when verifying
    that requests match. Some parts of this scrubbing are provider-specific.

-   The replayed provider responses produce the expected results from the
    portable API library.

### Record Mode

In `-record` mode, tests run as integration tests, making live requests to
backend servers and recording the requests/responses for later use in replay
mode.

To use `-record`:

1.  Provision resources.

    -   For example, the tests for the AWS implementation of Blob requires a
        bucket to be provisioned.
    -   TODO(issue #300): Use Terraform scripts to provision the resources
        needed for a given test.
    -   For now, do this manually.

2.  Run the test with `--record`.

    -   TODO(issue #300): The test will read the Terraform output to find its
        inputs.
    -   For now, pass the required resources via test-specific flags. In some
        cases, tests are
        [hardcoded to specific resource names](https://github.com/google/go-cloud/issues/286).

3.  The test will save the network interactions for subsequent replays.

    -   TODO(issue #300): The test will save the Terraform output to a file in
        order to replay using the same inputs.
    -   Commit the new replay files along with your code change. Expect to see
        lots of diffs; see below for more discussion.

### Diffs in replay files

Each time portable API tests are run in `--record` mode, the resulting replay
files are different. Looking at diffs of these files isn't particularly useful.

We [considered](https://github.com/google/go-cloud/issues/276) trying to scrub
the files of dynamic data so that diffs would be useful. We ended up deciding
not to do this, for several reasons:

-   There's a lot of dynamic data, in structured data of various forms (e.g.,
    HTTP headers, XML/JSON body, etc.). It would be difficult and fragile to
    scrub it all.

-   The scrub process would also be fragile relative to changes in providers
    (e.g., adding a new dynamic HTTP response header).

-   The scrub process would need to be implemented for every new provider,
    increasing the barrier to entry for new implementations.

-   Scrubbing would likely be even more difficult for providers using a
    non-HTTP-based protocol (e.g., gRPC).

-   Scrubbing the data decreases the fidelity of the replay test, since it
    wouldn't be operating on the original data.

Overall, massive diffs in the replay files are expected and fine. As part of a
code change, you may want to check for things like the number of RPCs made to
identify performance regressions.

## Module Boundaries

With the advent of [Go modules], there are mechanisms to release different parts
of a repository on different schedules. This permits one API to be in alpha/beta
(pre-1.0), whereas another API can be stable (1.0 or later).

As of 2018-09-13, Go Cloud as a whole still is not stable enough to call any
part 1.0 yet. Until this milestone is reached, all of the Go Cloud libraries
will be placed under a single module. The exceptions are standalone tools like
[Contribute Bot][] that are part of the project, but not part of the library.
After 1.0 is released, it is expected that each interface in Go Cloud will be
released as one module. Provider implementations will live in separate modules.
The exact details remain to be determined.

[Go modules]: https://github.com/golang/go/wiki/Modules
[Contribute Bot]: https://github.com/google/go-cloud/tree/master/internal/contributebot
