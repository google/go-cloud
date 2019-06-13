---
title: "Run Queries"
date: 2019-06-12T17:41:56-04:00
draft: true
weight: 3
---

Docstore's `Get` action lets you retrieve a single document by its primary key.
Queries let you retrieve all documents that match some conditions.  You can also
use queries to delete or update all documents that match the conditions.

<!--more-->

## Getting Documents

Like [action lists]({{< ref "actions.md#actions-and-action-lists" >}}), queries are built up in a fluent style:

```go
iter := coll.Query().Where("Score", ">", 20).Get(ctx)
```


Just as a `Get` action returns one document, the `Query.Get` method returns
several documents, in the form of an iterator. Repeatedly calling `Next` on the
iterator will return all the matching documents. Like the `Get` action, `Next`
will populate an empty document that you pass to it:

```go
doc := &Player{}
err := iter.Next(ctx, doc)
```

The iteration is over when `Next` returns `io.EOF`.

{{< goexample "gocloud.dev/internal/docstore.ExampleQuery_Get" >}}

Queries support the following methods:

- `Where` describes a condition on a document.
- `OrderBy` specifies the order of the resulting documents.
- `Limit` limits the number of documents in the result.

## Deleting Documents

Call `Delete` on a `Query` instead of `Get` to delete all the documents in the
collection that match the conditions. 

{{< goexample "gocloud.dev/internal/docstore.ExampleQuery_Delete" >}}

## Updating Documents

Calling `Update` on a `Query` and passing in a set of modifications will update
all the documents that match the criteria. 

{{< goexample "gocloud.dev/internal/docstore.ExampleQuery_Update" >}}


