---
title: "Read and Write Documents"
date: 2019-06-11T10:11:59-04:00
draft: true
weight: 2
---

In Docstore, you use actions to read, modify and write documents. You can
execute a single action, or run multiple actions together in an _action list_.

<!--more-->

## Representing Documents

We'll use a collection with documents represented by this Go struct:

```go
type Player struct {
	Name             string
	Score            int
	DocstoreRevision interface{}
}
```

We recommend using structs for documents because they impose some structure on
your data, but Docstore also accepts `map[string]interface{}` values. See
[the `docstore` package
documentation](https://godoc.org/gocloud.dev/internal/docstore#hdr-Documents)
for more information.

The `DocstoreRevision` field holds information about the latest revision of the
document. We discuss it [below]({{< ref "#docrev" >}}).

## Actions and Action Lists

Once you have [opened a collection]({{< ref "open-collection.md" >}}), you
can call action methods on it. We will use `coll` as the variable holding the collection.

Docstore supports six actions on documents:

- `Get` retrieves a document.
- `Create` creates a new document.
- `Replace` replaces an existing document.
- `Put` puts a document whether or not it already exists.
- `Update` applies a set of modifications to a document.
- `Delete` deletes a document.

You can create a single document with the `Collection.Create` method:

```go
err := coll.Create(ctx, &Player{Name: "Pat", Score: 10})
if err != nil {
    return err
}
```

If you have more than one action to perform, it is better to use an action
list. Drivers can optimize action lists by using bulk RPCs, running the actions
concurrently, or employing a provider's special features to improve efficiency
and reduce cost. Here we create several documents using an action list.

{{< goexample "gocloud.dev/internal/docstore.ExampleCollection_Actions_bulkWrite" >}}

`ActionList` has a fluent API, so you can build and execute a sequence of
actions in one line of code. Here we `Put` a document and immediately `Get` its new
contents.

{{< goexample "gocloud.dev/internal/docstore.ExampleCollection_Actions_getAfterWrite" >}}

If the underlying provider is eventually consistent, the result of the `Get`
might not reflect the `Put`. Docstore only guarantees that it will perform the
`Get` after the `Put` completes.

See the documentation for [`docstore.ActionList`][] for the semantics of action
list execution.

[`docstore.ActionList`]: https://godoc.org/gocloud.dev/internal/docstore#ActionList

## Updates

Use `Update` to modify individual fields of a document. The `Update` action takes a
set of modifications to document fields, and applies them all atomically. You
can change the value of a field, increment it, or delete it.

{{< goexample "gocloud.dev/internal/docstore.ExampleCollection_Update" >}}

## Revisions {#docrev}

Docstore maintains a revision for every document. Whenever the document is
changed, the revision is too. By default, Docstore stores the revision
in a field named `DocstoreRevision`, but you can change the field name via an
option to a `Collection` constructor.

You can use revisions to perform _optimistic locking_, a technique for updating
a document atomically:

1. `Get` a document. This reads the current revision.
2. Modify the document contents on the client (but do not change the revision).
3. `Replace` the document. If the document was changed since it was retrieved in
   step 1, the revision will be different, and Docstore will return an error
   instead of overwriting the document.
4. If the `Replace` failed, start again from step 1.

{{< goexample "gocloud.dev/internal/docstore.Example_optimisticLocking" >}}

See [the Revisions section of the package
documentation](https://godoc.org/gocloud.dev/internal/docstore#hdr-Revisions)
for more on revisions.

