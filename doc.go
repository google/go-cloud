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

At the core of the Go CDK are common "portable types" implemented by cloud
providers. For example, objects of the blob.Bucket portable type can be created
using gcsblob.OpenBucket, s3blob.OpenBucket, or any other provider. Then, the
blob.Bucket can be used throughout your application without worrying about
the underlying implementation.

The Go CDK works well with a code generator called Wire
(https://github.com/google/wire/blob/master/README.md). It creates
human-readable code that only imports the cloud SDKs for providers you use. This
allows the Go CDK to grow to support any number of cloud providers, without
increasing compile times or binary sizes, and avoiding any side effects from
`init()` functions.

For sample applications and a tutorial, see the samples directory
(https://github.com/google/go-cloud/tree/master/samples).


URLs

In addition to creating portable types via provider-specific constructors
(e.g., creating a blob.Bucket using s3blob.OpenBucket), many portable types
can also be created using a URL. The scheme of the URL specifies the provider,
and each provider implementation has code to convert the URL into the data
needed to call its constructor. For example, calling blob.OpenBucket with
s3blob://my-bucket will return a blob.Bucket created using s3blob.OpenBucket.

Each portable API package will document the types that it supports opening
by URL; for example, the blob package supports Buckets, while the pubsub
package supports Topics and Subscriptions. Each provider implementation will
document what scheme(s) it registers for, and what format of URL it expects.

Each portable type URL opener will accept URL schemes with an <api>+ prefix
(e.g., blob+file:///dir" instead of "file:///dir", as well as schemes with an
<api>+<type>+ prefix (e.g., blob+bucket+file:///dir).

Each portable API package should include an example using a URL, and
many providers will include provider-specific examples as well.


URL Muxes

Each portable type that's openable via URL will have a top-level function
you can call, like blob.OpenBucket. This top-level function uses a default
instance of an URLMux multiplexer to map schemes to a provider-specific
opener for the type. For example, blob has a BucketURLOpener interface that
providers implement and then register using RegisterBucket.

Many applications will work just fine using the default mux through the
top-level Open functions. However, if you want more control, you can create
your own URLMux and register the provider URLOpeners you need. Most providers
will export URLOpeners that give you more fine grained control over the
arguments needed by the constructor. In particular, portable types opened via
URL will often use default credentials from the environment. For example, the
AWS URL openers use the credentials saved by "aws login" (we don't want to
include credentials in the URL itself, since they are likely to be sensitive).

  - Instantiate the provider's URLOpener with the specific fields you need.
    For example, s3blob.URLOpener{ConfigProvider: myAWSProvider} using a
    ConfigProvider that holds explicit AWS credentials.
  - Create your own instance of the URLMux, e.g., mymux := new(blob.URLMux).
  - Register your custom URLOpener on your mux, e.g.,
    mymux.RegisterBucket(s3blob.Scheme, myS3URLOpener).
  - Now use your mux to open URLs, e.g. mymux.OpenBucket('s3://my-bucket').


Escaping the abstraction

It is not feasible or desirable for APIs like blob.Bucket to encompass the full
functionality of every provider. Rather, we intend to provide a subset of the
most commonly used functionality. There will be cases where a developer wants
to access provider-specific functionality, such as unexposed APIs or data
fields, errors or options. This can be accomplished using As functions.


As

As functions in the APIs provide the user a way to escape the Go CDK
abstraction to access provider-specific types. They might be used as an interim
solution until a feature request to the Go CDK is implemented. Or, the Go CDK
may choose not to support specific features, and the use of As will be
permanent.

Using As implies that the resulting code is no longer portable; the
provider-specific code will need to be ported in order to switch providers.
Therefore, it should be avoided if possible.

Each API will include examples demonstrating how to use its various As
functions, and each provider implementation will document what types it
supports for each.

Usage:

1. Declare a variable of the provider-specific type you want to access.

2. Pass a pointer to it to As.

3. If the type is supported, As will return true and copy the
provider-specific type into your variable. Otherwise, it will return false.

Provider-specific types that are intended to be mutable will be exposed
as a pointer to the underlying type.
*/
package cloud // import "gocloud.dev"
