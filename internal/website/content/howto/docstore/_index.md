---
title: "Docstore"
date: 2019-06-08T15:11:57-04:00
lastmod: 2019-07-11T14:57:23-07:00
draft: true
showInSidenav: true
toc: true
---

Docstore provides an abstraction layer over common [document
stores](https://en.wikipedia.org/wiki/Document-oriented_database) like Amazon
DynamoDB, Google Cloud Firestore, and MongoDB. These guides show how to work
with Docstore in the Go CDK.


<!--more-->

## Opening a Collection {#opening}

The first step in using Docstore is connecting to your
document store provider. Every document store provider is a little different, but the Go CDK
lets you interact with all of them using the [`*docstore.Collection`][] type.

While every provider has the concept of a primary key that uniquely
distinguishes a document in a collection, each one specifies that key in its own
way. To be portable, Docstore requires that the key be part of the document's
contents. When you open a collection using one of the functions described here,
you specify how to find the provider's primary key in the document.


[`*docstore.Collection`]: https://godoc.org/gocloud.dev/docstore##Collection

<!--more-->

### Constructors versus URL openers

The easiest way to open a collection is using [`docstore.OpenCollection`][] and
a URL pointing to the topic, making sure you ["blank import"][] the driver
package to link it in. See [Concepts: URLs][] for more details. If you need
fine-grained control over the connection settings, you can call the constructor
function in the driver package directly (like `mongodocstore.OpenCollection`).
This guide will show how to use both forms for each document store provider.

[`docstore.OpenCollection`]:
https://godoc.org/gocloud.dev/docstore##OpenCollection
["blank import"]: https://golang.org/doc/effective_go.html##blank_import
[Concepts: URLs]: {{< ref "/concepts/urls.md" >}}

### DynamoDB {#dynamodb}

The
[`awsdynamodb`](https://godoc.org/gocloud.dev/docstore/awsdynamodb)
package supports [Amazon DynamoDB](https://aws.amazon.com/dynamodb).
A Docstore collection corresponds to a DynamoDB table.

DynamoDB URLs provide the table, partition key field and optionally the sort key
field for the collection.

{{< goexample
"gocloud.dev/docstore/awsdynamodb.Example_openCollectionFromURL" >}}

Full details about acceptable URLs can be found under the API reference for
[`awsdynamodb.URLOpener`][].

##### DynamoDB Constructor {#dynamodb-ctor}

The
[`awsdynamodb.OpenCollection`][] constructor opens a DynamoDB table as a Docstore collection. You must first
create an [AWS session][] with the same region as your collection:

{{< goexample "gocloud.dev/docstore/awsdynamodb.ExampleOpenCollection" >}}

[AWS session]: https://docs.aws.amazon.com/sdk-for-go/api/aws/session/
[`awsdynamodb.OpenCollection`]: https://godoc.org/gocloud.dev/docstore/awsdynamodb##OpenCollection
[`awsdynamodb.URLOpener`]: https://godoc.org/gocloud.dev/docstore/awsdynamodb##URLOpener

### Google Cloud Firestore {#firestore}

The [`gcpfirestore`](https://godoc.org/gocloud.dev/docstore/gcpfirestore)
package supports [Google Cloud
Firestore](https://https://cloud.google.com/firestore). Firestore documents
are uniquely named by paths that are not part of the document content. In
Docstore, these unique names are represented as part of the document. You must
supply a way to extract a document's name from its contents. This can be done by
specifying a document field that holds the name, or by providing a function to
extract the name from a document.

Firestore URLs provide the project and collection, as well as the field
that holds the document name.

{{< goexample "gocloud.dev/docstore/gcpfirestore.Example_openCollectionFromURL" >}}

Full details about acceptable URLs can be found under the API reference for
[`gcpfirestore.URLOpener`][].

[`gcpfirestore.URLOpener`]:  https://godoc.org/gocloud.dev/docstore/gcpfirestore##URLOpener

##### Firestore Constructors {#firestore-ctor}

The [`gcpfirestore.OpenCollection`][] constructor opens a Cloud Firestore collection
as a Docstore collection. You must first connect a Firestore client using
[`gcpfirestore.Dial`][] or the
[`cloud.google.com/go/firestore/apiv1`](https://godoc.org/cloud.google.com/go/firestore/apiv1)
package. In addition to a client, `OpenCollection` requires a Google Cloud project ID, the
path to the Firestore collection, and the name of the field that holds
the document name.

{{< goexample "gocloud.dev/docstore/gcpfirestore.ExampleOpenCollection" >}}

Instead of mapping the document name to a field, you can supply a function to construct the name
from the document contents with [`gcpfirestore.OpenCollectionWithNameFunc`][].
This can be useful for documents whose name is the combination of two or more fields.

{{< goexample "gocloud.dev/docstore/gcpfirestore.ExampleOpenCollectionWithNameFunc" >}}


[`gcpfirestore.Dial`]: https://godoc.org/gocloud.dev/docstore/gcpfirestore##Dial
[`gcpfirestore.OpenCollection`]: https://godoc.org/gocloud.dev/docstore/gcpfirestore##OpenCollection
[`gcpfirestore.OpenCollectionWithNameFunc`]: https://godoc.org/gocloud.dev/docstore/gcpfirestore##OpenCollectionWithNameFunc

### MongoDB {#mongo}

The [`mongodocstore`](https://godoc.org/gocloud.dev/docstore/mongodocstore)
package supports the popular
[MongoDB](https://mongodb.org) document store. MongoDB documents
are uniquely identified by a field called `_id`. In
Docstore, you can choose a different name for this field, or provide a function to
extract the document ID from a document.

MongoDB URLs provide the database and collection, and optionally the field
that holds the document ID. Specify the Mongo server URL by setting the
`MONGO_SERVER_URL` environment variable.

{{< goexample "gocloud.dev/docstore/mongodocstore.Example_openCollectionFromURL" >}}

Full details about acceptable URLs can be found under the API reference for
[`mongodocstore.URLOpener`][].

[`mongodocstore.URLOpener`]:  https://godoc.org/gocloud.dev/docstore/mongodocstore##URLOpener


##### MongoDB Constructors {#mongo-ctor}

The [`mongodocstore.OpenCollection`][] constructor opens a MongoDB collection.
You must first obtain a standard MongoDB Go client using
[`mongodocstore.Dial`][] or the package
[`go.mongodb.org/mongo-driver/mongo`](https://godoc.org/go.mongodb.org/mongo-driver/mongo).
Obtain a `*mongo.Collection` from the client with
`client.Database(dbName).Collection(collName)`. Then pass the result to
`mongodocstore.OpenCollection` along with the name of the ID field, or `""` to use `_id`.

{{< goexample "gocloud.dev/docstore/mongodocstore.ExampleOpenCollection" >}}

Instead of mapping the document ID to a field, you can supply a function to construct the ID
from the document contents with [`mongodocstore.OpenCollectionWithIDFunc`][].
This can be useful for documents whose name is the combination of two or more fields.

{{< goexample "gocloud.dev/docstore/mongodocstore.ExampleOpenCollectionWithIDFunc" >}}

[`mongodocstore.Dial`]: https://godoc.org/gocloud.dev/docstore/mongodocstore##Dial
[`mongodocstore.OpenCollection`]: https://godoc.org/gocloud.dev/docstore/mongodocstore##OpenCollection
[`mongodocstore.OpenCollectionWithIDFunc`]: https://godoc.org/gocloud.dev/docstore/mongodocstore##OpenCollectionWithIDFunc

### In-Memory Document Store {#mem}

The
[`memdocstore`](https://godoc.org/gocloud.dev/docstore/mongodocstore)
package implements an in-memory document store suitable for testing and
development.

URLs for the in-memory store have a `mem:` scheme and provide the name of the
document field to use as a primary key. There is no collection name; each call
to one of the `OpenCollection` functions creates a distinct collection.

{{< goexample "gocloud.dev/docstore/memdocstore.Example_openCollectionFromURL" >}}

Full details about acceptable URLs can be found under the API reference for
[`memdocstore.URLOpener`][].

[`memdocstore.URLOpener`]:  https://godoc.org/gocloud.dev/docstore/memdocstore##URLOpener

##### Mem Constructors {##mem-ctor}

The [`memdocstore.OpenCollection`][] constructor creates and opens a collection,
taking the name of the key field.

{{< goexample "gocloud.dev/docstore/memdocstore.ExampleOpenCollection" >}}

You can instead supply a function to construct the primary key
from the document contents with [`memdocstore.OpenCollectionWithKeyFunc`][].
This can be useful for documents whose name is the combination of two or more fields.

{{< goexample "gocloud.dev/docstore/memdocstore.ExampleOpenCollectionWithKeyFunc" >}}

[`memdocstore.OpenCollection`]: https://godoc.org/gocloud.dev/docstore/memdocstore##OpenCollection
[`memdocstore.OpenCollectionWithKeyFunc`]: https://godoc.org/gocloud.dev/docstore/memdocstore##OpenCollectionWithKeyFunc

## Actions {#actions}

In Docstore, you use actions to read, modify and write documents. You can
execute a single action, or run multiple actions together in an _action list_.

<!--more-->

### Representing Documents {#rep-doc}

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
documentation](https://godoc.org/gocloud.dev/docstore##hdr-Documents)
for more information.

The `DocstoreRevision` field holds information about the latest revision of the
document. We discuss it [below]({{< ref "##docrev" >}}).

### Actions and Action Lists {#act-list}

Once you have [opened a collection]({{< ref "#opening" >}}), you
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

{{< goexample "gocloud.dev/docstore.ExampleCollection_Actions_bulkWrite" >}}

`ActionList` has a fluent API, so you can build and execute a sequence of
actions in one line of code. Here we `Put` a document and immediately `Get` its new
contents.

{{< goexample "gocloud.dev/docstore.ExampleCollection_Actions_getAfterWrite" >}}

If the underlying provider is eventually consistent, the result of the `Get`
might not reflect the `Put`. Docstore only guarantees that it will perform the
`Get` after the `Put` completes.

See the documentation for [`docstore.ActionList`][] for the semantics of action
list execution.

[`docstore.ActionList`]: https://godoc.org/gocloud.dev/docstore##ActionList

### Updates {#act-update}

Use `Update` to modify individual fields of a document. The `Update` action takes a
set of modifications to document fields, and applies them all atomically. You
can change the value of a field, increment it, or delete it.

{{< goexample "gocloud.dev/docstore.ExampleCollection_Update" >}}

### Revisions {#docrev}

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

{{< goexample "gocloud.dev/docstore.Example_optimisticLocking" >}}

See [the Revisions section of the package
documentation](https://godoc.org/gocloud.dev/docstore##hdr-Revisions)
for more on revisions.

## Queries {#queries}

Docstore's `Get` action lets you retrieve a single document by its primary key.
Queries let you retrieve all documents that match some conditions.  You can also
use queries to delete or update all documents that match the conditions.

<!--more-->

### Getting Documents {#qr-get}

Like [action lists]({{< ref "#act-list" >}}), queries are built up in a fluent style.
Just as a `Get` action returns one document, the `Query.Get` method returns
several documents, in the form of an iterator. 

```go
iter := coll.Query().Where("Score", ">", 20).Get(ctx)
defer iter.Stop() // Always call Stop on an iterator.
```

Repeatedly calling `Next` on the iterator will return all the matching
documents. Like the `Get` action, `Next` will populate an empty document that
you pass to it:

```go
doc := &Player{}
err := iter.Next(ctx, doc)
```

The iteration is over when `Next` returns `io.EOF`.

{{< goexample "gocloud.dev/docstore.ExampleQuery_Get" >}}

You can pass a list of fields to `Get` to reduce the amount of data transmitted.

Queries support the following methods:

- `Where` describes a condition on a document. You can ask whether a field is
  equal to, greater than, or less than a value. The "not equals" comparison
  isn't supported, because it isn't portable across providers.
- `OrderBy` specifies the order of the resulting documents, by field and
  direction. For portability, you can specify at most one `OrderBy`, and its
  field must also be mentioned in a `Where` clause. 
- `Limit` limits the number of documents in the result.

If a query returns an error, the message may help you fix the problem. Some
features, like full table scans, have to be enabled via constructor options,
because they can be expensive. Other queries may require that you manually
create an index on the collection.

### Deleting Documents {#qr-del}

Call `Delete` on a `Query` instead of `Get` to delete all the documents in the
collection that match the conditions. 

{{< goexample "gocloud.dev/docstore.ExampleQuery_Delete" >}}

### Updating Documents {#qr-update}

Calling `Update` on a `Query` and passing in a set of modifications will update
all the documents that match the criteria. 

{{< goexample "gocloud.dev/docstore.ExampleQuery_Update" >}}
