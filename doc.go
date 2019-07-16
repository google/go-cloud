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

At the core of the Go CDK are common "portable types", implemented on top of
service-specific drivers for supported cloud services. For example,
objects of the blob.Bucket portable type can be created using
gcsblob.OpenBucket, s3blob.OpenBucket, or any other Go CDK driver. Then, the
blob.Bucket can be used throughout your application without worrying about the
underlying implementation.

The Go CDK works well with a code generator called Wire
(https://github.com/google/wire/blob/master/README.md). It creates
human-readable code that only imports the cloud SDKs for drivers you use. This
allows the Go CDK to grow to support any number of cloud services, without
increasing compile times or binary sizes, and avoiding any side effects from
`init()` functions.

For non-reference documentation, see https://gocloud.dev/

URLs

See https://gocloud.dev/concepts/urls/ for a discussion of URLs in the Go CDK.

As

See https://gocloud.dev/concepts/as/ for a discussion of how to write
service-specific code with the Go CDK.
*/
package cloud // import "gocloud.dev"
