// Copyright 2018 The Go Cloud Authors
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

The Go Cloud Project allows application developers to seamlessly deploy cloud
applications on any combination of cloud providers. It does this by providing
stable, idiomatic interfaces for common uses like storage and databases.
Think `database/sql` for cloud products.

At the core of the project are common types implemented by cloud providers.
For example, the blob.Bucket type can be created using
gcsblob.OpenBucket, s3blob.OpenBucket, or any other provider. Then,
the blob.Bucket can be used throughout your application without worrying
about the underlying implementation.

This project also provides a code generator called Wire
(https://github.com/google/go-cloud/blob/master/wire/README.md). It creates
human-readable code that only imports the cloud SDKs for providers you use. This
allows Go Cloud to grow to support any number of cloud providers, without
increasing compile times or binary sizes, and avoiding any side effects from
`init()` functions.

For sample applications and a tutorial, see the samples directory
(https://github.com/google/go-cloud/tree/master/samples).
*/
package cloud // import "github.com/google/go-cloud"
