---
title: "Open a Collection"
date: 2019-06-08T15:14:26-04:00
draft: true
weight: 1
---

The first step in using Docstore is connecting to your
document store provider. Every document store provider is a little different, but the Go CDK
lets you interact with all of them using the [`*docstore.Collection`][] type. 

While every provider has the concept of a primary key that uniquely
distinguishes a document in a collection, each one specifies that key in its own
way. To be portable, Docstore requires that the key be part of the document's
contents. When you open a collection using one of the functions described here,
you specify how to find the provider's primary key in the document.


[`*docstore.Collection`]: https://godoc.org/gocloud.dev/docstore#Collection

<!--more-->

## Constructors versus URL openers

If you know that your program is always going to use a particular 
provider or you need fine-grained control over the connection settings, you
should call the constructor function in the driver package directly (like
`mongodocstore.OpenCollection`). However, if you want to change providers based on
configuration, you can use `docstore.OpenCollection`, making sure you ["blank
import"][] the driver package to link it in. See the
[documentation on URLs][] for more details. This guide will show how to use
both forms for each document store provider.

["blank import"]: https://golang.org/doc/effective_go.html#blank_import
[documentation on URLs]: {{< ref "/concepts/urls.md" >}}

## DynamoDB {#dynamodb}

The
[`dynamodocstore`](https://godoc.org/gocloud.dev/docstore/dynamodocstore) 
package supports [Amazon DynamoDB](https://aws.amazon.com/dynamodb).
A Docstore collection corresponds to a DynamoDB table.

DynamoDB URLs provide the table, partition key field and optionally the sort key
field for the collection.

{{< goexample
"gocloud.dev/docstore/dynamodocstore.Example_openCollectionFromURL" >}}

Full details about acceptable URLs can be found under the API reference for
[`dynamodocstore.URLOpener`][].

### DynamoDB Constructor {#dynamodb-ctor}

The
[`dynamodocstore.OpenCollection`][] constructor opens a DynamoDB table as a Docstore collection. You must first
create an [AWS session][] with the same region as your collection:

{{< goexample "gocloud.dev/docstore/dynamodocstore.ExampleOpenCollection" >}}

[AWS session]: https://docs.aws.amazon.com/sdk-for-go/api/aws/session/
[`dynamodocstore.OpenCollection`]: https://godoc.org/gocloud.dev/docstore/dynamodocstore#OpenCollection
[`dynamodocstore.URLOpener`]: https://godoc.org/gocloud.dev/docstore/dynamodocstore#URLOpener

## Google Cloud Firestore {#firestore}

The [`firedocstore`](https://godoc.org/gocloud.dev/docstore/firedocstore)
package supports [Google Cloud
Firestore](https://https://cloud.google.com/firestore). Firestore documents
are uniquely named by paths that are not part of the document content. In
Docstore, these unique names are represented as part of the document. You must
supply a way to extract a document's name from its contents. This can be done by
specifying a document field that holds the name, or by providing a function to
extract the name from a document.

Firestore URLs provide the project and collection, as well as the field
that holds the document name. 

{{< goexample "gocloud.dev/docstore/firedocstore.Example_openCollectionFromURL" >}}

Full details about acceptable URLs can be found under the API reference for
[`firedocstore.URLOpener`][].

[`firedocstore.URLOpener`]:  https://godoc.org/gocloud.dev/docstore/firedocstore#URLOpener

### Firestore Constructors {#firestore-ctor}

The [`firedocstore.OpenCollection`][] constructor opens a Cloud Firestore collection
as a Docstore collection. You must first connect a Firestore client using
[`firedocstore.Dial`][] or the
[`cloud.google.com/go/firestore/apiv1`](https://godoc.org/cloud.google.com/go/firestore/apiv1)
package. In addition to a client, `OpenCollection` requires a Google Cloud project ID, the
path to the Firestore collection, and the name of the field that holds
the document name.

{{< goexample "gocloud.dev/docstore/firedocstore.ExampleOpenCollection" >}}

Instead of mapping the document name to a field, you can supply a function to construct the name
from the document contents with [`firedocstore.OpenCollectionWithNameFunc`][]. 
This can be useful for documents whose name is the combination of two or more fields. 

{{< goexample "gocloud.dev/docstore/firedocstore.ExampleOpenCollectionWithNameFunc" >}}


[`firedocstore.Dial`]: https://godoc.org/gocloud.dev/docstore/firedocstore#Dial
[`firedocstore.OpenCollection`]: https://godoc.org/gocloud.dev/docstore/firedocstore#OpenCollection
[`firedocstore.OpenCollectionWithNameFunc`]: https://godoc.org/gocloud.dev/docstore/firedocstore#OpenCollectionWithNameFunc

## MongoDB {#mongo}

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

[`mongodocstore.URLOpener`]:  https://godoc.org/gocloud.dev/docstore/mongodocstore#URLOpener


### MongoDB Constructors {#mongo-ctor}

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

[`mongodocstore.Dial`]: https://godoc.org/gocloud.dev/docstore/mongodocstore#Dial
[`mongodocstore.OpenCollection`]: https://godoc.org/gocloud.dev/docstore/mongodocstore#OpenCollection
[`mongodocstore.OpenCollectionWithIDFunc`]: https://godoc.org/gocloud.dev/docstore/mongodocstore#OpenCollectionWithIDFunc

## In-Memory Document Store {#mem}

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

[`memdocstore.URLOpener`]:  https://godoc.org/gocloud.dev/docstore/memdocstore#URLOpener

### Mem Constructors {#mem-ctor}

The [`memdocstore.OpenCollection`][] constructor creates and opens a collection,
taking the name of the key field.

{{< goexample "gocloud.dev/docstore/memdocstore.ExampleOpenCollection" >}}

You can instead supply a function to construct the primary key
from the document contents with [`memdocstore.OpenCollectionWithKeyFunc`][]. 
This can be useful for documents whose name is the combination of two or more fields. 

{{< goexample "gocloud.dev/docstore/memdocstore.ExampleOpenCollectionWithKeyFunc" >}}

[`memdocstore.OpenCollection`]: https://godoc.org/gocloud.dev/docstore/memdocstore#OpenCollection
[`memdocstore.OpenCollectionWithKeyFunc`]: https://godoc.org/gocloud.dev/docstore/memdocstore#OpenCollectionWithKeyFunc


## What's Next

Now that you have opened a collection, you can [perform actions on
documents]({{< ref "./actions.md" >}}) in the collection using portable operations.
