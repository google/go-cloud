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

// Package firedocstore provides an implementation of the docstore API for Google
// Cloud Firestore.
//
//
// Docstore types not supported by the Go firestore client, cloud.google.com/go/firestore:
// - unsigned integers: encoded is int64s
// - complex64/128: encoded as an array of two float32/64s.
// - arrays: encoded as Firestore array values
//
// Firestore types not supported by Docstore:
// - Document reference (a pointer to another Firestore document)
// TODO(jba): figure out how to support this
//
//
// Queries
//
// Firestore allows only one field in a query to be compared with an inequality
// operator (one other than "="). This driver selects the first field in a Where
// clause with an inequality to pass to Firestore and handles the rest locally. For
// example, if the query specifies the three clauses A > 1, B > 2 and A < 3, then
// A > 1 and A < 3 will be sent to Firestore, and the results will be filtered by
// B > 2 in this driver.
//
// Firestore requires a composite index if a query contains both an equality and an
// inequality comparison. This driver returns an error if the necessary index does
// not exist. You must create the index manually. See
// https://cloud.google.com/firestore/docs/query-data/indexing for details.
//
// See https://cloud.google.com/firestore/docs/query-data/queries for more information on Firestore queries.
package firedocstore // import "gocloud.dev/internal/docstore/firedocstore"
