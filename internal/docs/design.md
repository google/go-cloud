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

- The developer persona wants to write business logic that is agnostic of
	underlying cloud provider. Their focus is on making software correct for the
	requirements at hand.
- The operator persona wants to incorporate the business logic into the
	organization's policies and provision resources for the logic to run. Their
	focus is making software run predictably and reliably with the resources at
	hand.

Go Cloud uses Go interfaces at the boundary between these two personas: a
developer is meant to use an interface, and an operator is meant to provide an
implementation of that interface. This distinction prevents Go Cloud going down
a path of complexity that makes application portability difficult.  The
[`blob.Bucket`] type is a prime example: the API does not provide a way of
creating a new bucket.  To properly and safely create such a bucket requires
careful consideration, getting something like ACLs wrong could lead to a
catastrophic data leak. To generate the ACLs correctly requires modeling of IAM
users and roles for each cloud platform, and some way of mapping those users and
roles across clouds. While not impossible, the level of complexity and the high
likelihood of a leaky abstraction leads us to believe this is not the right
direction for Go Cloud.

Instead of adding large amounts of leaky complexity to Go Cloud, we expect the
operator role to handle the management of non-portable platform-specific
resources. An implementor of the `Bucket` interface does not need to determine
the content type of incoming data, as that is a developer's concern.  This
separation of concerns allows these two personas to communicate using a shared
language while focusing on their respective areas of expertise.

[`blob.Bucket`]: https://godoc.org/github.com/google/go-cloud/blob#Bucket

The distinction between developer and operator becomes more blurred when
investigating testing. Integration tests might well want to create and destroy
resources. We recommend [Terraform](http://terraform.io) for such cases, but
we have not investigated the CI workflow for this at the time of writing.

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

-  The user-facing type can perform higher level logic without making the
	 interface complex to implement. In the blob example, the user-facing type's
	 `NewWriter` method can do the content type detection and then pass the final
	 result to the driver type.
-  Methods can be added to the user-facing type without breaking compatibility.
	 Contrast with adding methods to an interface, which is a breaking change.
-  As new operations on the driver are added as new optional interfaces, the
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
