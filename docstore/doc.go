// Copyright 2019 The Go Cloud Development Kit Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package docstore provides a portable implementation of a document store.
// A document store is a service that stores data in recursively defined
// documents (like, but not necessarily the same as, JSON). Like other
// NoSQL databases, document stores are schemaless.
// See https://en.wikipedia.org/wiki/Document-oriented_database for more information.
//
// In docstore, documents are grouped into collections, and each document has a key
// that is unique in its collection. You can add, delete, modify and retrieve
// documents by key, and you can query a collection to retrieve documents that match
// certain criteria.
//
// Subpackages contain distinct implementations ("drivers") of docstore for various
// providers, including MongoDB, Google Cloud Firestore, and Amazon DynamoDB. You can
// use Azure Cosmos and Amazon DocumentDB via the MongoDB driver. There is also
// memdocstore, an in-process, in-memory implementation suitable for testing and
// development.
//
// Your application should import one of these provider-specific subpackages and use
// its exported functions to create a *Collection; do not use the NewCollection
// function in this package. For example:
//
//   coll, err := memdocstore.OpenCollection("SSN", nil)
//   if err != nil {
//     return fmt.Errorf("opening collection: %v", err)
//   }
//   defer coll.Close()
//   // coll is a *docstore.Collection
//
// Then, write your application code using the *Collection type. You can easily
// reconfigure your initialization code to choose a different provider.
// You can develop your application locally using memdocstore, or deploy it to
// multiple Cloud providers. You may find http://github.com/google/wire useful
// for managing your initialization code.
//
// Alternatively, you can construct a *Collection via a URL and the OpenCollection
// function in this package. See https://gocloud.dev/concepts/urls for more
// information.
//
//
// Documents
//
// Docstore allows you to represent documents as either map[string]interface{} or
// struct pointers. Using structs is recommended, because it enforces some structure
// on your data. Docstore mimics the encoding/json package in its treatment of
// structs: by default, a struct's exported fields are the fields of the document.
// You can alter this default mapping by using a struct tag beginning with
// "docstore:". Docstore struct tags support renaming, omitting fields
// unconditionally, or omitting them only when they are empty, exactly like
// encoding/json. Docstore also honors a "json" struct tag if there is no "docstore"
// tag on the field.
//
//
// Representing Data
//
// The value stored in a document field can be any of a wide range of types. All
// primitive types except for complex numbers are supported, as well as slices and
// maps (the map key type must be a string, an integer, or a type that implements
// encoding.TextMarshaler). In addition, any type that implements
// encoding.BinaryMarshaler or encoding.TextMarshaler is permitted. This set of types
// closely matches the encoding/json package.
//
// Times deserve special mention. Docstore can store and retrieve values of type
// time.Time, with two caveats. First, the timezone will not be preserved. Second,
// Docstore guarantees only that time.Time values are represented to millisecond
// precision. Many providers will do better, but if you need to be sure that times
// are stored with nanosecond precision, convert the time.Time to another type before
// storing, and re-create when you retrieve it. For instance, you could call the
// time's UnixNano method to get an int64, and get the original time back (in the
// local timezone) with the time.Unix function.
//
//
// Keys
//
// The key of a docstore document is some function of its contents, usually a field.
// Keys never appear alone in the docstore API, only as part of a document. For
// instance, to retrieve a document by key, you pass the Get method a document whose
// key field is populated, and docstore populates the rest of that argument with the
// stored document.
//
// When you open a collection using one of the subpackages' OpenCollection methods or
// a URL, you specify how to extract the key from a document.
// Usually, you provide the name of the key field, as in the example above:
//
//   coll, err := memdocstore.OpenBucket("SSN", nil)
//
// Here, the "SSN" field of the document is used as the key. Some providers let you
// supply a function to extract the key from the document, which can be useful if the
// key is composed of more than field.
//
//
// Actions
//
// Docstore supports six actions on documents:
//   - Get retrieves a document.
//   - Create creates a new document.
//   - Replace replaces an existing document.
//   - Put puts a document into a collection whether or not it is already present.
//   - Update applies a set of modifications to a document.
//   - Delete deletes a document.
//
// Each action acts atomically on a single document. You can execute actions
// individually--the Collection type has a method for each one--or you can group them
// into a list, like so:
//
//   err := coll.Actions().Put(doc1).Replace(doc2).Get(doc3).Do(ctx)
//
// When you use an action list, docstore will try to optimize the execution of the
// actions. For example, multiple Get actions may be combined into a single "batch
// get" RPC. For the most part, actions in a list execute in an undefined order
// (perhaps concurrently) and independently, but a Get and a write on the same
// document are ordered with respect to each other. See the documentation of
// ActionList for details.
//
//
// Revisions
//
// Docstore supports document revisions to distinguish different versions of a
// document and enable optimistic locking. By default, Docstore stores the revision
// in the field named "DocstoreRevision" (stored in the constant
// docstore.DefaultRevisionField). Providers give you the option of changing
// that field name.
//
// Docstore gives every document a revision when it is created, and changes that
// revision whenever the document is modified. If a struct doesn't have a revision
// field, then no revision logic is performed. Otherwise, whenever you pass a
// document with a revision to Put, Replace, Update or Delete, Docstore will return
// an error with code FailedPrecondition if the stored document's revision does not
// match the given document's. (See the gocloud.dev/gcerrors package for information
// about error codes.) If you call Get to retrieve a document, then later perform a
// write action with that same document, it will fail if the document was changed
// since the Get. Create, Put, Replace and Update all populate their argument
// document with the new revision of the document.
//
// Since different providers use different types for revisions, the type of the
// revision field is unspecified. When defining a struct for storing docstore data,
// define the field to be of type interface{}. For example,
//
//    type User {
//      Name             string
//      DocstoreRevision interface{}
//    }
//
//
// Queries
//
// Docstore supports querying within a collection. Call the Query method on
// Collection to obtain a Query value, then build your query by calling Query methods
// like Where, Limit and so on. Finally, call the Get method on the query to execute it.
// The result is an iterator, whose use is described below.
//
//     iter := coll.Query().Where("size", ">", 10).Limit(5).Get(ctx)
//
// The Where methods defines a filter condition, much like a WHERE clause in SQL.
// Conditions are of the form "field op value", where field is any document field
// path (including dot-separated paths), op is one of "=", ">", "<", ">=" or "<=",
// and value can be any value.
//
// You can make multiple Where calls. In some cases, parts of a Where clause may be
// processed on the client rather than natively by the provider, which may have
// performance implications for large result sets. See the provider-specific package
// documentation for details.
//
// Use the DocumentIterator returned from Query.Get by repeatedly calling its Next
// method until it returns io.EOF. Always call Stop when you are finished with an
// iterator. It is wise to use a defer statement for this.
//
//     iter := coll.Query().Where("size", ">", 10).Limit(5).Get(ctx)
//     defer iter.Stop()
//     for {
//         m := map[string]interface{}{}
//         err := iter.Next(ctx, m)
//         if err == io.EOF {
//             break
//         }
//         if err != nil {
//             return err
//         }
//         fmt.Println(m)
//     }
//
//
// Errors
//
// The errors returned from this package can be inspected in several ways:
//
// The Code function from gocloud.dev/gcerrors will return an error code, also
// defined in that package, when invoked on an error.
//
// The Collection.ErrorAs method can retrieve the driver error underlying the returned
// error.
// TODO(shantuo): implement ErrorAs.
//
//
// OpenCensus Integration
// TODO(jba): implement
package docstore // import "gocloud.dev/docstore"
