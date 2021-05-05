---
title: "Docstore"
date: 2019-06-08T15:11:57-04:00
lastmod: 2019-07-29T12:00:00-07:00
showInSidenav: true
toc: true
---

The [`docstore` package][] provides an abstraction layer over common
[document stores](https://en.wikipedia.org/wiki/Document-oriented_database) like
Google Cloud Firestore, Amazon DynamoDB and MongoDB. This guide shows how to
work with document stores in the Go CDK.

<!--more-->

A document store is a service that stores data in semi-structured JSON-like
documents grouped into collections. Like other NoSQL databases, document stores
are schemaless.

The [`docstore` package][] supports operations to add, retrieve, modify and
delete documents.

Subpackages contain driver implementations of docstore for various services,
including Cloud and on-prem solutions. You can develop your application
locally using [`memdocstore`][], then deploy it to multiple Cloud providers with
minimal initialization reconfiguration.

[`docstore` package]: https://godoc.org/gocloud.dev/docstore
[`memdocstore`]: https://godoc.org/gocloud.dev/docstore/memdocstore

## Opening a Collection {#opening}

The first step in interacting with a document store is to instantiate
a portable [`*docstore.Collection`][] for your service.

While every docstore service has the concept of a primary key that uniquely
distinguishes a document in a collection, each one specifies that key in its own
way. To be portable, Docstore requires that the key be part of the document's
contents. When you open a collection using one of the functions described here,
you specify how to find the provider's primary key in the document.

The easiest way to open a collection is using [`docstore.OpenCollection`][] and
a service-specific URL pointing to it, making sure you ["blank import"][]
the driver package to link it in.

```go
import (
    "gocloud.dev/docstore"
    _ "gocloud.dev/docstore/<driver>"
)
...
coll, err := docstore.OpenCollection(context.Background(), "<driver-url>")
if err != nil {
    return fmt.Errorf("could not open collection: %v", err)
}
defer coll.Close()
// coll is a *docstore.Collection; see usage below
...
```

See [Concepts: URLs][] for general background and the [guide below][] for
URL usage for each supported service.

Alternatively, if you need
fine-grained control over the connection settings, you can call the constructor
function in the driver package directly (like `mongodocstore.OpenCollection`).

```go
import "gocloud.dev/docstore/<driver>"
...
coll, err := <driver>.OpenCollection(...)
...
```

You may find the [`wire` package][] useful for managing your initialization code
when switching between different backing services.

See the [guide below][] for constructor usage for each supported service

[`*docstore.Collection`]: https://godoc.org/gocloud.dev/docstore#Collection
[`docstore.OpenCollection`]:
https://godoc.org/gocloud.dev/docstore#OpenCollection
["blank import"]: https://golang.org/doc/effective_go.html#blank_import
[Concepts: URLs]: {{< ref "/concepts/urls.md" >}}
[guide below]: {{< ref "#services" >}}
[`wire` package]: http://github.com/google/wire

## Using a Collection {#using}

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
[the `docstore` package documentation](https://godoc.org/gocloud.dev/docstore#hdr-Documents)
for more information.

The `DocstoreRevision` field holds information about the latest revision of the
document. We discuss it [below]({{< ref "#rev" >}}).

### Actions {#actions}

Once you have [opened a collection]( {{< ref "#opening" >}}), you can call
action methods on it to read, modify and write documents. You can execute a
single action, or run multiple actions together in an [_action list_]({{ ref
"act-list" }}).

Docstore supports six kinds of actions on documents:

-   `Get` retrieves a document.
-   `Create` creates a new document.
-   `Replace` replaces an existing document.
-   `Put` puts a document whether or not it already exists.
-   `Update` applies a set of modifications to a document.
-   `Delete` deletes a document.

You can create a single document with the `Collection.Create` method, we will
use `coll` as the variable holding the collection throughout the guide:

```go
err := coll.Create(ctx, &Player{Name: "Pat", Score: 10})
if err != nil {
    return err
}
```

#### Action Lists {#act-list}

When you use an action list to perform multiple actions at once, drivers can
optimize action lists by using bulk RPCs, running the actions concurrently,
or employing a provider's special features to improve efficiency and reduce
cost. Here we create several documents using an action list.

{{< goexample "gocloud.dev/docstore.ExampleCollection_Actions_bulkWrite" >}}

`ActionList` has a fluent API, so you can build and execute a sequence of
actions in one line of code. Here we `Put` a document and immediately `Get` its
new contents.

{{< goexample "gocloud.dev/docstore.ExampleCollection_Actions_getAfterWrite" >}}

If the underlying provider is eventually consistent, the result of the `Get`
might not reflect the `Put`. Docstore only guarantees that it will perform the
`Get` after the `Put` completes.

See the documentation for [`docstore.ActionList`][] for the semantics of action
list execution.

[`docstore.ActionList`]: https://godoc.org/gocloud.dev/docstore#ActionList

#### Updates {#act-update}

Use `Update` to modify individual fields of a document. The `Update` action
takes a set of modifications to document fields, and applies them all
atomically. You can change the value of a field, increment it, or delete it.

{{< goexample "gocloud.dev/docstore.ExampleCollection_Update" >}}

### Queries {#queries}

Docstore's `Get` action lets you retrieve a single document by its primary key.
Queries let you retrieve all documents that match some conditions. You can also
use queries to delete or update all documents that match the conditions.

#### Getting Documents {#qr-get}

Like [actions]({{< ref "#actions" >}}), queries are built up in a fluent
style. Just as a `Get` action returns one document, the `Query.Get` method
returns several documents, in the form of an iterator.

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

-   `Where` describes a condition on a document. You can ask whether a field is
    equal to, greater than, or less than a value. The "not equals" comparison
    isn't supported, because it isn't portable across providers.
-   `OrderBy` specifies the order of the resulting documents, by field and
    direction. For portability, you can specify at most one `OrderBy`, and its
    field must also be mentioned in a `Where` clause.
-   `Limit` limits the number of documents in the result.

If a query returns an error, the message may help you fix the problem. Some
features, like full table scans, have to be enabled via constructor options,
because they can be expensive. Other queries may require that you manually
create an index on the collection.

### Revisions {#rev}

Docstore maintains a revision for every document. Whenever the document is
changed, the revision is too. By default, Docstore stores the revision in a
field named `DocstoreRevision`, but you can change the field name via an option
to a `Collection` constructor.

You can use revisions to perform _optimistic locking_, a technique for updating
a document atomically:

1.  `Get` a document. This reads the current revision.
2.  Modify the document contents on the client (but do not change the revision).
3.  `Replace` the document. If the document was changed since it was retrieved
    in step 1, the revision will be different, and Docstore will return an error
    instead of overwriting the document.
4.  If the `Replace` failed, start again from step 1.

{{< goexample "gocloud.dev/docstore.Example_optimisticLocking" >}}

See
[the Revisions section of the package documentation](https://godoc.org/gocloud.dev/docstore#hdr-Revisions)
for more on revisions.

## Other Usage Samples

* [CLI Sample](https://github.com/google/go-cloud/tree/master/samples/gocdk-docstore)
* [Order Processor sample](https://gocloud.dev/tutorials/order/)
* [docstore package examples](https://godoc.org/gocloud.dev/docstore#pkg-examples)

## Supported Docstore Services {#services}

### Google Cloud Firestore {#firestore}

The [`gcpfirestore`](https://godoc.org/gocloud.dev/docstore/gcpfirestore)
package supports
[Google Cloud Firestore](https://cloud.google.com/firestore). Firestore
documents are uniquely named by paths that are not part of the document content.
In Docstore, these unique names are represented as part of the document. You
must supply a way to extract a document's name from its contents. This can be
done by specifying a document field that holds the name, or by providing a
function to extract the name from a document.

Firestore URLs provide the project and collection, as well as the field that
holds the document name.

`docstore.OpenCollection` will use Application Default Credentials; if you have
authenticated via [`gcloud auth application-default login`][], it will use those credentials. See
[Application Default Credentials][GCP creds] to learn about authentication
alternatives, including using environment variables.

[GCP creds]: https://cloud.google.com/docs/authentication/production
[`gcloud auth application-default login`]: https://cloud.google.com/sdk/gcloud/reference/auth/application-default/login

{{< goexample
"gocloud.dev/docstore/gcpfirestore.Example_openCollectionFromURL" >}}

Full details about acceptable URLs can be found under the API reference for
[`gcpfirestore.URLOpener`][].

[`gcpfirestore.URLOpener`]:  https://godoc.org/gocloud.dev/docstore/gcpfirestore#URLOpener

#### Firestore Constructors {#firestore-ctor}

The [`gcpfirestore.OpenCollection`][] constructor opens a Cloud Firestore
collection as a Docstore collection. You must first connect a Firestore client
using [`gcpfirestore.Dial`][] or the
[`cloud.google.com/go/firestore/apiv1`](https://godoc.org/cloud.google.com/go/firestore/apiv1)
package. In addition to a client, `OpenCollection` requires a Google Cloud
project ID, the path to the Firestore collection, and the name of the field that
holds the document name.

{{< goexample "gocloud.dev/docstore/gcpfirestore.ExampleOpenCollection" >}}

Instead of mapping the document name to a field, you can supply a function to
construct the name from the document contents with
[`gcpfirestore.OpenCollectionWithNameFunc`][]. This can be useful for documents
whose name is the combination of two or more fields.

{{< goexample
"gocloud.dev/docstore/gcpfirestore.ExampleOpenCollectionWithNameFunc" >}}

[`gcpfirestore.Dial`]: https://godoc.org/gocloud.dev/docstore/gcpfirestore#Dial
[`gcpfirestore.OpenCollection`]: https://godoc.org/gocloud.dev/docstore/gcpfirestore#OpenCollection
[`gcpfirestore.OpenCollectionWithNameFunc`]: https://godoc.org/gocloud.dev/docstore/gcpfirestore#OpenCollectionWithNameFunc

### Amazon DynamoDB {#dynamodb}

The [`awsdynamodb`](https://godoc.org/gocloud.dev/docstore/awsdynamodb) package
supports [Amazon DynamoDB](https://aws.amazon.com/dynamodb). A Docstore
collection corresponds to a DynamoDB table.

DynamoDB URLs provide the table, partition key field and optionally the sort key
field for the collection.

`docstore.OpenCollection` will create a default AWS Session with the
`SharedConfigEnable` option enabled; if you have authenticated with the AWS CLI,
it will use those credentials. See [AWS Session][] to learn about authentication
alternatives, including using environment variables.

[AWS Session]: https://docs.aws.amazon.com/sdk-for-go/api/aws/session/

{{< goexample
"gocloud.dev/docstore/awsdynamodb.Example_openCollectionFromURL" >}}

Full details about acceptable URLs can be found under the API reference for
[`awsdynamodb.URLOpener`][].

#### DynamoDB Constructor {#dynamodb-ctor}

The [`awsdynamodb.OpenCollection`][] constructor opens a DynamoDB table as a
Docstore collection. You must first create an [AWS session][] with the same
region as your collection:

{{< goexample "gocloud.dev/docstore/awsdynamodb.ExampleOpenCollection" >}}

[AWS session]: https://docs.aws.amazon.com/sdk-for-go/api/aws/session/
[`awsdynamodb.OpenCollection`]: https://godoc.org/gocloud.dev/docstore/awsdynamodb#OpenCollection
[`awsdynamodb.URLOpener`]: https://godoc.org/gocloud.dev/docstore/awsdynamodb#URLOpener

### Azure Cosmos DB {#cosmosdb}

[Azure Cosmos DB][] is compatible with the MongoDB API. You can use the
[`mongodocstore`][] package to connect to Cosmos DB. You must
[create an Azure Cosmos account][] and get the MongoDB [connection string][].

When you use MongoDB URLs to connect to Cosmos DB, specify the Mongo server
URL by setting the `MONGO_SERVER_URL` environment variable to the connection
string. See the [MongoDB section][] for more details and examples on how to
use the package.

[Azure Cosmos DB]: https://docs.microsoft.com/en-us/azure/cosmos-db/
[create an Azure Cosmos account]: https://docs.microsoft.com/en-us/azure/cosmos-db/create-mongodb-dotnet
[connection string]: https://docs.microsoft.com/en-us/azure/cosmos-db/connect-mongodb-account#QuickstartConnection
[MongoDB section]: {{< ref "#mongo" >}}

#### Cosmos DB Constructors {#cosmosdb-ctor}

The [`mongodocstore.OpenCollection`][] constructor can open a Cosmos DB
collection. You must first obtain a standard MongoDB Go client with your
Cosmos connections string. See the [MongoDB constructor section][] for more
details and examples.

[MongoDB constructor section]: {{< ref "#mongo-ctor" >}}

### MongoDB {#mongo}

The [`mongodocstore`][] package supports the popular
[MongoDB](https://mongodb.org) document store. MongoDB documents are uniquely
identified by a field called `_id`. In Docstore, you can choose a different
name for this field, or provide a function to extract the document ID from a
document.

MongoDB URLs provide the database and collection, and optionally the field that
holds the document ID. Specify the Mongo server URL by setting the
`MONGO_SERVER_URL` environment variable.

{{< goexample
"gocloud.dev/docstore/mongodocstore.Example_openCollectionFromURL" >}}

Full details about acceptable URLs can be found under the API reference for
[`mongodocstore.URLOpener`][].

[`mongodocstore.URLOpener`]:  https://godoc.org/gocloud.dev/docstore/mongodocstore#URLOpener

#### MongoDB Constructors {#mongo-ctor}

The [`mongodocstore.OpenCollection`][] constructor opens a MongoDB collection.
You must first obtain a standard MongoDB Go client using
[`mongodocstore.Dial`][] or the package
[`go.mongodb.org/mongo-driver/mongo`](https://godoc.org/go.mongodb.org/mongo-driver/mongo).
Obtain a `*mongo.Collection` from the client with
`client.Database(dbName).Collection(collName)`. Then pass the result to
`mongodocstore.OpenCollection` along with the name of the ID field, or `""` to
use `_id`.

{{< goexample "gocloud.dev/docstore/mongodocstore.ExampleOpenCollection" >}}

Instead of mapping the document ID to a field, you can supply a function to
construct the ID from the document contents with
[`mongodocstore.OpenCollectionWithIDFunc`][]. This can be useful for documents
whose name is the combination of two or more fields.

{{< goexample
"gocloud.dev/docstore/mongodocstore.ExampleOpenCollectionWithIDFunc" >}}

[`mongodocstore`]: https://godoc.org/gocloud.dev/docstore/mongodocstore
[`mongodocstore.Dial`]: https://godoc.org/gocloud.dev/docstore/mongodocstore#Dial
[`mongodocstore.OpenCollection`]: https://godoc.org/gocloud.dev/docstore/mongodocstore#OpenCollection
[`mongodocstore.OpenCollectionWithIDFunc`]: https://godoc.org/gocloud.dev/docstore/mongodocstore#OpenCollectionWithIDFunc

### In-Memory Document Store {#mem}

The [`memdocstore`](https://godoc.org/gocloud.dev/docstore/memdocstore)
package implements an in-memory document store suitable for testing and
development.

URLs for the in-memory store have a `mem:` scheme. The URL host is used as the
the collection name, and the URL path is used as the name of the document field
to use as a primary key.

{{< goexample
"gocloud.dev/docstore/memdocstore.Example_openCollectionFromURL" >}}

Full details about acceptable URLs can be found under the API reference for
[`memdocstore.URLOpener`][].

[`memdocstore.URLOpener`]:  https://godoc.org/gocloud.dev/docstore/memdocstore#URLOpener

#### Mem Constructors {#mem-ctor}

The [`memdocstore.OpenCollection`][] constructor creates and opens a collection,
taking the name of the key field.

{{< goexample "gocloud.dev/docstore/memdocstore.ExampleOpenCollection" >}}

You can instead supply a function to construct the primary key from the document
contents with [`memdocstore.OpenCollectionWithKeyFunc`][]. This can be useful
for documents whose name is the combination of two or more fields.

{{< goexample
"gocloud.dev/docstore/memdocstore.ExampleOpenCollectionWithKeyFunc" >}}

[`memdocstore.OpenCollection`]: https://godoc.org/gocloud.dev/docstore/memdocstore#OpenCollection
[`memdocstore.OpenCollectionWithKeyFunc`]: https://godoc.org/gocloud.dev/docstore/memdocstore#OpenCollectionWithKeyFunc
