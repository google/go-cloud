# Design Decisions

This document outlines important design decisions made for this repository and
attempts to provide succinct rationales. Recording these decisions helps
maintain consistency across packages, especially as an open source project where
contributors can join at any point during development.

A broad design goal for the Go Cloud Development Kit (Go CDK) is for the API
style to be consistent. Consistency aids users in building a mental model of how
to use the APIs. As such, the design of individual packages must also consider
their impact on the Go CDK as a whole.

This is a [Living Document](https://en.wikipedia.org/wiki/Living_document). The
decisions in here are not set in stone, but simply describe our current thinking
about how to guide the Go Cloud Development Kit project. While it is useful to
link to this document when having discussions in an issue, it is not to be used
as a means of closing issues without discussion at all. Discussion on an issue
can lead to revisions of this document.

## Developers and Operators

The Go CDK is designed with two different personas in mind: the developer and
the operator. In the world of DevOps, these may be the same person. A developer
may be directly deploying their application into production, especially on
smaller teams. In a larger organization, these may be different teams entirely,
but working closely together. Regardless, these two personas have two very
different ways of looking at a Go program:

-   The developer persona wants to write business logic that is agnostic of
    underlying cloud provider. Their focus is on making software correct for the
    requirements at hand.
-   The operator persona wants to incorporate the business logic into the
    organization's policies and provision resources for the logic to run. Their
    focus is making software run predictably and reliably with the resources at
    hand.

The Go CDK uses Go interfaces at the boundary between these two personas: a
developer is meant to use an interface, and an operator is meant to provide an
implementation of that interface. This distinction prevents the Go CDK going
down a path of complexity that makes application portability difficult. The
[`blob.Bucket`][] type is a prime example: the API does not provide a way of
creating a new bucket. To properly and safely create such a bucket requires
careful consideration, getting something like ACLs wrong could lead to a
catastrophic data leak. To generate the ACLs correctly requires modeling of IAM
users and roles for each cloud platform, and some way of mapping those users and
roles across clouds. While not impossible, the level of complexity and the high
likelihood of a leaky abstraction leads us to believe this is not the right
direction for the Go CDK.

Instead of adding large amounts of leaky complexity to the Go CDK, we expect the
operator role to handle the management of non-portable platform-specific
resources. An implementor of the `Bucket` interface does not need to determine
the content type of incoming data, as that is a developer's concern. This
separation of concerns allows these two personas to communicate using a shared
language while focusing on their respective areas of expertise.

[`blob.Bucket`]: https://godoc.org/github.com/google/go-cloud/blob#Bucket

## Portable Types and Drivers

The portable APIs that the Go CDK exports (like [`blob.Bucket`][] or
[`runtimevar.Variable`][]) are concrete types, not interfaces. To understand
why, imagine if we used a plain interface:

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
implementation-agnostic logic from the interface. The implementation-agnostic
logic-containing concrete type is the **portable type**. We call the interface
the **driver**. Visually, it looks like this:

![Diagram showing user code depending on blob.Bucket, which holds a
driver.Bucket implemented by awsblob.Bucket.](img/user-facing-type.png)

This has a number of benefits:

-   The portable type can perform higher level logic without making the
    interface complex to implement. In the blob example, the portable type's
    `NewWriter` method can do the content type detection and then pass the final
    result to the driver type.
-   Methods can be added to the portable type without breaking compatibility.
    Contrast with adding methods to an interface, which is a breaking change.
-   When new operations on the driver are added as new optional interfaces, the
    portable type can hide the need for type-assertions from the user.

As a rule, if a method `Foo` has the same inputs and semantics in the portable
type and the driver type, then the driver method may be called `Foo`, even
though the return signatures may differ. Otherwise, the driver method name
should be different to reduce confusion.

New Go CDK APIs should always follow this portable type and driver pattern.

[`runtimevar.Variable`]:
https://godoc.org/github.com/google/go-cloud/runtimevar#Variable
[`Bucket.NewWriter` method]:
https://godoc.org/github.com/google/go-cloud/blob#Bucket.NewWriter
[`database/sql`]: https://godoc.org/database/sql

## Minimize Global State

As a library, the Go CDK should not introduce global state. Global state is
difficult to reason about in large codebases, where it can be necessary for
different parts of the application to use different states. Instead of adding
global state, push responsibility to the application to inject the state where
it is needed.

The exception we permit is URL scheme registration as documented under
[URLs](#urls). The amount of boilerplate setup code required for URL muxes for
multiple drivers without use of a tool like Wire is an unreasonable burden for
users of Go CDK. We want the Go CDK to be usable both with and without Wire. A
global registry is acceptable as long as its use is not mandatory, but the
burden is to prove the benefit over the cost.

## Driver Package Naming Conventions

Inside this repository, we name packages that handle cloud services after the
service name, not the providing cloud (`s3blob` instead of `awsblob`). While a
cloud provider may provide a unique offering for a particular API, they may not
always provide only one, so distinguishing them in this way keeps the API
symbols stable over time.

The naming convention is `<provider><product><api>`, where:

*   `<provider>` is the provider name, like `aws` or `gcp` or `azure`.
    *   Omit for 3rd party/open source/local packages.
    *   May also be omitted in cases where the product name is sufficient (e.g.,
        `s3blob` not `awss3blob` since S3 is well-known, `gcsblob` not
        `gcpgcsblob` since GCS already references Google).
    *   Required if the product name is not unique across providers (e.g.,
        `gcpkms` and `awskms`).
*   `<product>`is the product/service name.
*   `<api>` is the portable API name.
    *   Include for local/test packages like (e.g., `fileblob`, `mempubsub`).
    *   May be omitted when it makes the package name too long (e.g. `awssnssqs`
        is long enough, don't add `pubsub`).
    *   Encouraged when it helps distinguish the package from the service's own
        package name (e.g., `s3blob` not `s3`).

## Portable Type Constructors

Portable type constructors are the functions defined in driver packages that end
users call to get an instance of the portable type. For example,
`gcsblob.OpenBucket`, which returns an instance of the `*blob.Bucket` portable
type backed by GCS.

-   Portable type constructors should be top-level functions that return the
    portable type directly. Avoid helpers (e.g., a `Client` struct with a
    function that returns the portable type instead of it being top-level) and
    wrappers (e.g., a `fooblob.Bucket` type returned from `fooblob.OpenBucket`
    that wraps the portable type). Top level functions without wrappers are
    easier to use, especially when we're consistent about it.
-   Order arguments that are less likely to change across multiple calls to the
    constructor before ones that are likely to change. For example, connection
    and authorization related arguments should go before names, so
    `OpenBucket(ctx, client, "mybucket")` instead of `OpenBucket(ctx,
    "mybucket", client)`.
-   All public constructors should take an `Options` struct (see next section).

### Option Structs

All public constructors should take an `Options` struct, even if it is currently
empty, to ensure that we can add arguments to the APIs in the future without
breaking backward compatibility.

-   This includes driver constructors (e.g., `gcsblob.OpenBucket`) as well as
    API functions (e.g., `blob.NewReader`). When in doubt, if you think it's
    possible that we'll add arguments, add `Options`.
-   The argument should be of type `*Options`, so that `nil` can be passed in
    the default case.
-   Name the `Options` struct appropriately. `Options` is usually fine for
    portable type constructors since the package generally only exposes a
    constructor. Inside a driver interface or in a portable type like `blob`,
    use more descriptive names like `ReaderOptions` or `WriterOptions`.
-   If a function already has a struct argument, don't add a separate `Options`
    struct. Example: the various `sql.Open` functions take a `Params` struct
    with connection parameters; we chose to add new options to `Params` instead
    of introducing a separate `Options` struct. This keeps the function
    signature simpler and avoid confusion about which struct new parameters
    should be added to.
-   When similar `Options` are part of a driver interface and also part of the
    portable type (e.g., `blob.WriterOptions`), duplicate the struct instead of
    aliasing or embedding it, and copy the struct fields explicitly where
    needed. This allows the godoc for each type to be tailored to the
    appropriate audience (e.g. end-users for the portable type, driver
    implementors for the driver interface), and also allows the structs to
    diverge over time if appropriate.
-   Required arguments must not be in an `Options` struct, and all fields of the
    `Options` struct must have reasonable defaults. Exception: struct arguments
    that don't have `Options` in the name can contain required arguments (e.g.,
    see the `Params` example for `sql.Open` above).

Regarding empty `Options` structs: we considered only adding them when the first
option is added, and using a separate constructor for compatibility (e.g., start
with `foo.New(...)` and later add `foo.NewWithOptions(..., opts *Options)` if
needed). However, this would result in inconsistent names over time (e.g., some
packages would expose `New` with an `Options`, while others would expose
`NewWithOptions`).

### Compound IDs

Many cloud providers have resource IDs that are made up of subcomponents in
some well-defined syntax. For example, [GCP KMS key IDs][] take the form
`projects/[PROJECT_ID]/locations/[LOCATION]/keyRings/[KEY_RING]/cryptoKeys/[KEY]`.
We call these _compound IDs_.

There are two broad compound ID usage patterns we have observed:

1. Applications will keep resources in the same location, so the application
   will build the ID from the subcomponents rather than passing the entire
   resource ID around.
2. Applications will pass a verbatim string from configuration down to the
   API, since this is what was easily copy-pasteable from the cloud console UI.

Go CDK constructors that take in compound IDs should take in a `string` with the
full compound ID. Helper functions to build these compound IDs from
subcomponents may be provided as needed. URL openers (described below) should
prefer to use the full compound ID in their URL format.

[GCP KMS key IDs]: https://cloud.google.com/kms/docs/object-hierarchy#key

### URLs

To enable the [Backing services factor][] of a Twelve-Factor Application, Go
Cloud includes the ability to construct each of its API objects using
identifying URLs. The portable type's package should include APIs like the
following:

```go
// Package foo is a portable API. foo could be something like blob or pubsub.
//
// Throughout this example, Widget is used as a stand-in for a portable type
// inside foo, like Bucket or Subscription.
package foo

// A type that implements WidgetURLOpener can open widgets based on a URL.
// The opener must not modify the URL argument. OpenWidgetURL must be safe to
// call from multiple goroutines.
//
// WidgetURLOpeners should not assume that the URL has a particular scheme.
type WidgetURLOpener interface {
  OpenWidgetURL(ctx context.Context, u *url.URL) (*Widget, error)
}

// URLMux is a URL opener multiplexer. It matches the scheme of the URLs
// against a set of registered schemes and calls the opener that matches the
// URL's scheme.
//
// The zero value is a multiplexer with no registered schemes.
type URLMux struct {
  // ...
}

// RegisterWidget registers the opener with the given scheme. If an opener
// already exists for the scheme, RegisterWidget panics.
func (mux *URLMux) RegisterWidget(scheme string, opener WidgetURLOpener) {
  // ...
}

// OpenWidget calls OpenWidgetURL with the URL parsed from urlstr.
// OpenWidget is safe to call from multiple goroutines.
func (mux *URLMux) OpenWidget(ctx context.Context, urlstr string) (*Widget, error) {
  u, err := url.Parse(urlstr)
  if err != nil {
    return nil, fmt.Errorf("open widget: %v", err)
  }
  return mux.OpenWidgetURL(ctx, u)
}

// OpenWidgetURL dispatches the URL to the opener that is registered with the
// URL's scheme. OpenWidgetURL is safe to call from multiple goroutines.
func (mux *URLMux) OpenWidgetURL(ctx context.Context, u *url.URL) (*Widget, error) {
  // ...
}

// DefaultURLMux returns the URLMux used by OpenWidget.
func DefaultURLMux() *URLMux {
  return defaultURLMux
}

var defaultURLMux = new(URLMux)

// OpenWidget opens the Widget identified by the URL given. URL openers must be
// registered in the DefaultURLMux, which is typically done in driver
// packages' initialization.
func OpenWidget(ctx context.Context, urlstr string) (*Widget, error) {
  return DefaultURLMux().OpenWidget(urlstr)
}
```

The repetition of `Widget` in the method names permits a type to handle multiple
resources within the API. Exporting the `URLMux` allows applications to build
their own muxes, potentially wrapping existing ones.

Driver packages should include their own `URLOpener` struct type which
implements all the relevant `WidgetURLOpener` methods. The URL should only serve
to identify which resource to open. Any credentials or other complex values
should be taken in as struct fields, not as input from URL. If the driver
package registers its `URLOpener` with the `DefaultURLMux`, then it should
populate these complex fields from environment variables. If doing so is
undesirable or expensive, then it should not register with the `DefaultURLMux`
and instead rely on users to create their own mux. If there already exists a
well-established URI format for the backend (like S3 URLs or database connection
URIs), then drivers should honor them where possible.

[Backing services factor]: https://12factor.net/backing-services

#### URL Examples

A `WidgetURLOpener` implementation for a hypothetical GCP service:

```go
package gcpfoo

// ...

const Scheme = "gcpwidget"

type URLOpener struct {
  Client  *gcp.HTTPClient
  Options Options
}

func (o *URLOpener) OpenWidgetURL(ctx context.Context, u *url.URL) (*foo.Widget, error) {
  // ...
  return OpenWidget(ctx, o.Client, u.Host, &o.Options)
}

type lazyURLOpener struct {
  init   sync.Once
  opener *URLOpener
  err    error
}

func (o *lazyURLOpener) OpenWidgetURL(ctx context.Context, u *url.URL) (*foo.Widget, error) {
  o.init.Once(func() {
    creds, err := gcp.DefaultCredentials(ctx)
    if err != nil {
      o.err = err
      return
    }
    o.opener = new(URLOpener)
    o.opener.Client, _ = gcp.NewHTTPClient(http.DefaultTransport, creds.TokenSource)
  })
  if o.err != nil {
    return nil, o.err
  }
  return o.opener.OpenWidgetURL(ctx, u)
}

func init() {
  foo.DefaultURLMux().Register(Scheme, new(lazyURLOpener))
}

// OpenWidget is the exported non-URL constructor.
func OpenWidget(ctx context.Context, c *gcp.HTTPClient, name string, opts *Options) (*foo.Widget, error) {
  // ...
}
```

Using the global default mux:

```go
import _ "gocloud.dev/foo/gcpfoo"

// ...

widget, err := foo.OpenWidget(context.Background(), "gcpwidget://xyzzy")
```

Using a custom mux created during server initialization:

```go
myMux := new(foo.URLMux)
myMux.Register(gcpfoo.Scheme, &gcpfoo.URLOpener{
  Client: client,
})
widget, err := myMux.OpenWidget(context.Background(), "gcpwidget://xyzzy")
```

## Errors

### General

-   The callee is expected to return `error`s with messages that include
    information about the particular call, as opposed to the caller adding this
    information. This aligns with common Go practice.

### Drivers

Driver implementations should:

-   Return the raw errors from the underlying service, and not wrap them in
    `fmt.Errorf` calls, so that they can be exposed to end users via `ErrorAs`.

### Portable Types

Portable types should:

-   Wrap errors returned from driver implementations before returning them to
    end users, so that users can't peek into driver-specific error details
    without using `As`. Make sure not to double-wrap.

-   Use `internal/gcerr.New` when wrapping driver errors, like so: `if err :=
    driver.Call(xyz); err != nil { return gcerr.New(code, err, 1, "blob") }` The
    first argument is an error code. See below for advice on choosing the
    appropriate code.

    The third argument is the distance in stack frames from the function whose
    location should be associated with the error. It should be `1` if you are
    calling `New` from the same function that made the driver call, `2` if you
    are calling new from a helper function, and so on. The fourth argument is an
    additional string that will display with the error. You should pass the API
    name.

-   By default, choose the code `Unknown`, keeping details of returned `error`s
    unspecified. The most common case is that the caller will only care whether
    an operation succeeds or not.

-   If certain `error`s are interesting for callers to distinguish, choose one
    of the other codes from the `gcerrors.ErrorCode` enum, so user programs can
    act on the kind of error without having to look at driver-specific errors.

    -   If more than one error code makes sense, choose the most specific one.
    -   If none make sense, choose `Unknown`.
    -   If none make sense but you want something more specific than `Unknown`:
        -   If you can generalize your code to make it applicable to more than
            just your API, add it to `gcerrors.ErrorCode`. Look at the
            [gRPC error codes](https://github.com/grpc/grpc-go/blob/master/codes/codes.go)
            for inspiration.
        -   Otherwise, you can define a custom code in your portable API
            package. Your code should use a negative integer.

-   For now, your package should expose an `ErrorAs` function to allow users to
    access driver-specific error types. We may review this choice if
    `golang.org/x/xerrors.As` becomes part of the standard library.

-   Handle transient network errors. Retry logic is best handled as low in the
    stack as possible to avoid [cascading failure][]. APIs should try to surface
    "permanent" errors (e.g. malformed request, bad permissions) where
    appropriate so that application logic does not attempt to retry
    non-idempotent operations, but the responsibility is largely on the library,
    not on the application.

[cascading failure]:
https://landing.google.com/sre/book/chapters/addressing-cascading-failures.html

## Escape Hatches using As

The Go CDK allows users to escape the abstraction as needed using `As`
functions, described in more detail in the
[concept guide](https://gocloud.dev/concepts/as/). `As` functions take an
`interface{}` and return a `bool`; they return `true` if the underlying concrete
type could be converted into the type provided as the `interface{}`.

An alternative approach would have been something like
[`os.ProcessState.Sys`](https://golang.org/pkg/os/#ProcessState.Sys), which
returns an `interface{}` that the user can then type cast/assert to
service-specific types.

We ended up going with `As` because:

1.  Most portable types have an `As` function for errors; choosing `As` results
    in an easy and natural implementation for chained errors once the
    [Go 2 proposal for errors](https://go.googlesource.com/proposal/+/master/design/29934-error-values.md)
    arrives. It is currently implemented in
    [xerrors](https://godoc.org/golang.org/x/xerrors#As), and we're already
    using that in some drivers.
2.  `As` adds more flexibility for drivers to support conversions to multiple
    types. Specifically, not the case where there are multiple possible
    underlying types, but rather that a single underlying type can be converted
    to multiple types.
    *   Chained errors is one example of this, where the top-level error may
        always be the same type, but may also represent a chain of other errors
        with different types.
    *   Another example is that a driver might choose to support `As`-level
        compatibility with another driver; e.g., driver `foo` could support all
        of the `As` types defined by `s3blob`, converting them internally, and
        then any code that runs with driver `s3blob` would also work with driver
        `foo` (even if it uses the `As` escape hatches).

## Enforcing Portability

The Go CDK APIs will end up exposing functionality that is not supported by all
services. In addition, some functionality details will differ across services.
Some theoretical examples using [`blob.Bucket`][]:

1.  **Top-level APIs**: There might be a service that supports reads, but not
    writes or deletes.
1.  **Data fields**. Some services may support key/value metadata associated
    with a blob, others may not.
1.  **Naming rules**. Different services may allow different name lengths, or
    allow/disallow non-ASCII unicode characters. See [Strings](#strings) below
    for more on handling string differences.
1.  **Semantic guarantees**. Different services may have different consistency
    guarantees; for example, S3 only provides eventually consistency while GCS
    provides strong consistency.

How can we maintain portability while these differences exist?

### Guiding Principle

Any incompatibilities between drivers should be visible to the user as soon as
possible. From best to worst:

1.  At compile time
1.  At configuration/app startup time (e.g., when the portable type is created)
1.  At runtime (e.g., when the incompatible behavior is accessed), via a non-nil
    error
1.  At runtime, via panic

### Approaches Considered

1.  **Documentation**. We could try to document non-uniform or optional
    functionality across drivers. Optional fields or functionality would
    return "not implemented" errors or zero values.
1.  **Restrict functionality to the intersection**. We could explicitly only
    support the intersection of all services. For example, if not all services
    allow unicode characters in names, then **blob** would not allow it either.
1.  **Enforced feature codes**: Go CDK APIs could enumerate the ways in which
    drivers differ as a `FeatureCode` enum.
    *   Drivers would declare which feature codes they support, enforced by
        extensions to the existing conformance tests.
    *   API users would declare which feature codes they need.
    *   Mismatches between what a user requests and what the driver supports
        would be enforced at initialization time.
    *   As much as possible, the API (via the portable type) would enforce that
        the user is only exposed to optional functionality that they asked for.
    *   For example, the default legal name for a blob might be ASCII only, with
        a `FeatureUnicodeNames` feature code. Users that don't request this
        feature code would only be able to use blobs with ASCII names, even if
        the underlying service supports unicode. If the user requested
        `FeatureUnicodeNames`, and their driver supports it, they could then
        use blobs with unicode; if their driver doesn't support it, they would
        get an initialization-time error.

```
b, err := blob.NewBucket(d, blob.FeatureUnicodeNames)
...
```

Design discussions regarding enforcing portability are ongoing; we welcome input
on the [mailing list](https://groups.google.com/forum/#!forum/go-cloud).

### Strings

Services often differ on what they accept in particular strings (e.g., blob
names, metadata keys, etc.). A couple of specific examples:

*   Azure Blob only
    [accepts C# identifiers](https://docs.microsoft.com/en-us/azure/storage/blobs/storage-properties-metadata)
    as metadata keys.
*   S3 drops double slashes in blob names (e.g., `foo//bar` will end up being
    saved as `foo/bar`).

These differences lead to a loss of portability and predictability for users.

To resolve this issue, we insist that Go CDK can handle any UTF-8 string, and
force drivers to use escaping mechanisms to handle strings that the underlying
service can't handle. We enforce driver compliance with conformance tests.
Behavior for non-UTF-8 strings is undefined (but see
https://github.com/google/go-cloud/issues/1281 and
https://github.com/google/go-cloud/issues/1260).

We try to use URL encoding as the escaping mechanism where possible; however,
sometimes it is not and we'll use custom escaping. As an example, a driver for a
service that only allows underscores and ASCII alphanumeric characters might
escape the string `foo.bar` to `foo__0x2e__bar` (URL escaping won't work because
`%` isn't allowed).

Pros of this approach:

*   Go CDK APIs are internally consistent in that a user can write any string to
    any service and get the original string back when they read it back.
*   Go CDK APIs have visibility into all existing strings for all services.

Cons:

*   Go CDK could overwrite existing data if a Go CDK-written key escapes to an
    already-existing value (e.g., if the `foo__0x2e__bar` string already
    existed, it would be overwritten by a Go CDK write to `foo.bar`).
*   Escaping may push a string over the maximum allowed string length for a
    service. Escaping does not solve (and in fact may exacerbate) problems with
    different maximum string lengths across services.
*   Existing strings that happen to look like Go CDK-escaped strings will be
    unescaped by Go CDK (e.g., an existing string `foo__0x2e__bar` would appear
    as `foo.bar` when read through the Go CDK).
*   Strings that were written through the Go CDK and needed escaping will appear
    in their escaped form when viewed outside of Go CDK (e.g., `foo__0x2e__bar`
    would appear on the service's UI).

Most of these cons are mitigated by choosing unusual-looking escape mechanisms
that are unlikely to appear in existing data.

Drivers should escape strings when writing to the underlying service, and
unescape them when reading them back. The Go CDK will provide helpers for these
operations, as well as a test suite of strings for conformance tests.

Sample code for the helper for escaping strings:

```
// package escape provides helpers for escaping and unescaping strings.
package escape

// Escape returns s, with all runes for which shouldEscape returns true
// escaped to "__0xXXXX__", where XXXX is the hex representation of the rune
// value. For example, " " would escape to "__0x20__".
//
// Non-UTF-8 strings will have their non-UTF-8 characters escaped to
// unicode.ReplacementChar; the original value is lost. Please file an
// issue if you need non-UTF8 support.
//
// Note: shouldEscape takes the whole string as a slice of runes and an
// index. Passing it a single byte or a single rune doesn't provide
// enough context for some escape decisions; for example, the caller might
// want to escape the second "/" in "//" but not the first one.
// We pass a slice of runes instead of the string or a slice of bytes
// because some decisions will be made on a rune basis (e.g., encode
// all non-ASCII runes).
func Escape(s string, shouldEscape func(s []rune, i int) bool) string { ... }

// Unescape reverses Escape.
func Unescape(s string) string {...}
```

Sample code for how a driver might use it, using metadata keys for a `blob` as
the example string:

```
// When writing metadata keys, escape the keys:
// ... gcdkMetadata is the metadata passed to the GCDK API.
for k, v := range gcdkMetadata {
    e := escape.Escape(k, func (r []rune, i int) bool {...})
    if _, ok := serviceMetadata[e]; ok {
      return fmt.Errorf("duplicate keys after escaping: %q => %q", k, e)
    }
    serviceMetadata[e] = v
}
// ... write serviceMetadata to the service.

// When reading metadata keys, unescape them:
// ... serviceMetadata is the metadata read from the service.
for k, v := range serviceMetadata {
    gcdkMetadata[escape.Unescape(k)] = v
}
// ... return gcdkMetadata.
```

The details of what runes need to be escaped will vary from service to
service. The details of how to escape may also vary, although we expect to use
URL encoding where possible, and a common custom escaping where not. For the
custom escaping, we plan to escape each rune for which `shouldEscape` returns
true with `__0xXXX__`, where `XCX` is the hex representation of the rune value.

### Alternatives Considered

*   We considered restricting Go CDK's APIs to strings that all services
    support. For example, we could have asserted that Go CDK's `blob` only
    supports ASCII plus `/` for blob names (and no `//`!). However, such a rule
    would mean that we couldn't cleanly handle existing strings created through
    some mechanism other than through Go CDK APIs that violate the rule. For
    example, an existing blob in S3 with a unicode name. Filtering out such
    strings so that they aren't visible at all through the Go CDK would be both
    surprising and limiting, and could easily result in data loss (e.g., if a
    user read a set of metadata for a blob via the Go CDK, and some keys were
    filtered out, and then wrote the metadata back, the filtered keys would be
    lost). Not filtering such strings would mean that the Go CDK isn't
    internally consistent (i.e., you can read some strings but not write them).
    Overall, we decided that this approach is unacceptable.

*   We could expose the escaper used by drivers in their `Options` structs
    (including options like disabling it, overriding the set of bytes to be
    escaped, or overriding the escaping mechanism), but we'll wait to see if
    there's demand for that.

## Coding Conventions

We try to adhere to commonly accepted Go coding conventions, some of which are
described on the
[Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
wiki page. We also adopt the following guidelines:

-   Prefer `map[K]V{}` to `make(map[K]V)`. It's more concise.
-   When writing a loop appending to a slice `s`, prefer

    ```
      var s []T
      for ... {
        ...
        s = append(s, ...)
        ...
      }
    ```

    to

    ```
      s := make([]T, 0, N)
      for ... {
        ...
        s = append(s, ...)
        ...
      }
    ```

    or

    ```
      s := make([]T, N)
      for ... {
        ...
        s[i] = ...
        ...
      }
    ```

    (Exception: the loop body is trivial and the loop is performance-sensitive.)
    The first version is shorter and easier to read, and it is impossible to get
    the length wrong.

-   Prefer `log.Fatal` to `panic` in example tests.

-   Ensure you've run `goimports` on your code to properly group import
    statements.

-   Order arguments that are less likely to change across multiple calls to the
    constructor before ones that are likely to change. For example, connection
    and authorization related arguments should go before names, so
    `OpenBucket(ctx, client, "mybucket")` instead of `OpenBucket(ctx,
    "mybucket", client)`.

## Tests

### Conformance Tests

Since our goal is for users to be able to use drivers interchangeably, it is
critical that they behave similarly. To this end, each portable API (e.g.,
`blob`) must provide a suite of conformance tests that driver implementations
should run. The conformance tests should be comprehensive; drivers should not
need additional unit tests for the core driver semantics.

### Provisioning For Tests

Portable API integration tests require developer-specific resources to be
created and destroyed. We use [Terraform](http://terraform.io) to do so, and
record the resource info and network interactions so that they can be replayed
as fast and repeatable unit tests.

### Replay Mode

Tests normally run in replay mode. In this mode, they don't require any
provisioned resources or network interactions. Replay tests verify that:

-   The same test inputs produce the same requests (e.g., HTTP requests) to the
    cloud service. Some parts of the request may be dynamic (e.g., dates in the
    HTTP request headers), so the replay tests do some scrubbing when verifying
    that requests match. Some parts of this scrubbing are service-specific.

-   The replayed service responses produce the expected results from the
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

2.  Run the test with `-record`.

    -   TODO(issue #300): The test will read the Terraform output to find its
        inputs.
    -   For now, pass the required resources via test-specific flags.
    -   When changing or adding tests, please only record the tests that are
        changed/affected by passing the `-run` flag to `go test` with the
        name of the test(s). Re-recording all tests of a driver creates a lot
        of noise and a large diff that's difficult to review.

3.  The test will save the network interactions for subsequent replays.

    -   TODO(issue #300): The test will save the Terraform output to a file in
        order to replay using the same inputs.
    -   Commit the new replay files along with your code change. Expect to see
        lots of diffs; see below for more discussion.

### Diffs in replay files

Each time portable API tests are run in `-record` mode, the resulting replay
files are different. Looking at diffs of these files isn't particularly useful.

We [considered](https://github.com/google/go-cloud/issues/276) trying to scrub
the files of dynamic data so that diffs would be useful. We ended up deciding
not to do this, for several reasons:

-   There's a lot of dynamic data, in structured data of various forms (e.g.,
    HTTP headers, XML/JSON body, etc.). It would be difficult and fragile to
    scrub it all.

-   The scrub process would also be fragile relative to changes in services
    (e.g., adding a new dynamic HTTP response header).

-   The scrub process would need to be implemented for every new service,
    increasing the barrier to entry for new implementations.

-   Scrubbing would likely be even more difficult for services using a
    non-HTTP-based protocol (e.g., gRPC).

-   Scrubbing the data decreases the fidelity of the replay test, since it
    wouldn't be operating on the original data.

Overall, massive diffs in the replay files are expected and fine. As part of a
code change, you may want to check for things like the number of RPCs made to
identify performance regressions.
