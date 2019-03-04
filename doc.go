// Copyright 2018 The Go Cloud Development Kit Authors
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

/*
Package cloud contains a library and tools for open cloud development in Go.

The Go Cloud Development Kit (Go CDK) allows application developers to
seamlessly deploy cloud applications on any combination of cloud providers.
It does this by providing stable, idiomatic interfaces for common uses like
storage and databases. Think `database/sql` for cloud products.

At the core of the Go CDK are common types implemented by cloud providers.
For example, objects of the blob.Bucket type can be created using
gcsblob.OpenBucket, s3blob.OpenBucket, or any other provider. Then, the
*blob.Bucket can be used throughout your application without worrying about
the underlying implementation.

The Go CDK works well with a code generator called Wire
(https://github.com/google/wire/blob/master/README.md). It creates
human-readable code that only imports the cloud SDKs for providers you use. This
allows the Go CDK to grow to support any number of cloud providers, without
increasing compile times or binary sizes, and avoiding any side effects from
`init()` functions.

For sample applications and a tutorial, see the samples directory
(https://github.com/google/go-cloud/tree/master/samples).


Escaping the abstraction

It is not feasible or desirable for APIs like blob.Bucket to encompass the full
functionality of every provider. Rather, we intend to provide a subset of the
most commonly used functionality. There will be cases where a developer wants
to access provider-specific functionality, which might consist of:

1. Top-level APIs. For example, blob does not expose a Copy, but some provider
might.

2. Data fields. For example, blob exposes a few attributes like ContentType
and Size, but S3 and GCS both have many more.

3. Errors. For example, S3 returns awserr.Error.

4. Options. Different providers may support different options for functionality.

As functions in the APIs provide the user a way to escape the Go CDK
abstraction to access provider-specific types. They might be used as an interim
solution until a feature request to the Go CDK is implemented. Or, the Go CDK
may choose not to support specific features, and the use of As will be
permanent. As an example, both S3 and GCS blobs have the concept of ACLs, but
it might be difficult to represent them in a generic way (although, we have not
tried).

Using As implies that the resulting code is no longer portable; the
provider-specific code will need to be ported in order to switch providers.
Therefore, it should be avoided if possible. For example:

	// The existing blob.Reader exposes some blob attributes, but not everything
	// that every provider exposes.
	type Reader struct {...}

	// As converts i to provider-specific types.
	func (r *Reader) As(i interface{}) bool {...}

	// User code would look like:
	r, _ := bucket.NewReader(ctx, "foo.txt", nil)
	var s3type s3.GetObjectOutput
	if r.As(&s3type) {
		... use s3type...
	}

Each provider implementation documents what type(s) it supports for each of the
As functions.

*/
package cloud // import "gocloud.dev"
