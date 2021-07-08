---
title: "Signers"
date: 2021-06-24T15:03:39-07:00
draft: true
showInSidenav: true
toc: true
---

The [`signers` package][] provides access to digest signing and verification
services in a portable way. This guide shows how to work with signatures in the 
Go CDK.

<!--more-->

Cloud applications frequently need to generate and verify digital signatures.

> A digital signature is a mathematical scheme for verifying the authenticity 
> of digital messages or documents. A valid digital signature, where the 
> prerequisites are satisfied, gives a recipient very strong reason to believe 
> that the message was created by a known sender (authentication), and that the 
> message was not altered in transit (integrity). ([wikipedia][wikipedia_signature])

Most Cloud providers include a key management service to perform these tasks,
usually with hardware-level security and audit logging.

The [`signers` package][] supports digest signing and verification operations.
This documentation assumes that the concepts of signing and verifying digital 
signatures of digests are understood.

Subpackages contain driver implementations of signers for various services,
including Cloud and on-prem solutions. You can develop your application
locally using [`localsigners`][], then deploy it to multiple Cloud providers 
with minimal initialization reconfiguration.

[wikipedia_signature]: https://en.wikipedia.org/wiki/Digital_signature
[`signers` package]: https://godoc.org/gocloud.dev/signers
[`localsigners`]: https://godoc.org/gocloud.dev/signers/localsigners

## Opening a Signers Signer {#opening}

The first step in working with your signers is
to instantiate a portable [`*signers.Signer`][] for your service.

The easiest way to do so is to use [`signers.OpenSigner`][] and a service-specific
URL pointing to the signer, making sure you ["blank import"][] the driver
package to link it in.

```go
import (
	"gocloud.dev/signers"
	_ "gocloud.dev/signers/<signers>"
)
...
signer, err := signers.OpenSigner(context.Background(), "<driver-url>")
if err != nil {
    return fmt.Errorf("could not open signer: %v", err)
}
defer signer.Close()
// signer is a *signers.Signer; see usage below
...
``` 

See [Concepts: URLs][] for general background and the [guide below][]
for URL usage for each supported service.

Alternatively, if you need
fine-grained control over the connection settings, you can call the constructor
function in the driver package directly (like `awssig.OpenSigner`).

```go
import "gocloud.dev/signers/<driver>"
...
signer, err := <driver>.OpenSigner(...)
...
```

You may find the [`wire` package][] useful for managing your initialization code
when switching between different backing services.

See the [guide below][] for constructor usage for each supported service.

[`*signers.Signer`]: https://godoc.org/gocloud.dev/signers#Signer
[`signers.OpenSigner`]:
https://godoc.org/gocloud.dev/signers#OpenSigner
["blank import"]: https://golang.org/doc/effective_go.html#blank_import
[Concepts: URLs]: {{< ref "/concepts/urls.md" >}}
[guide below]: {{< ref "#services" >}}
[`wire` package]: http://github.com/google/wire

## Using a Signers Signer {#using}

Once you have [opened a signer][] for the signers provider you want,
you can sign and verify digests using the signer.

[opened a signer]: {{< ref "#opening" >}}

### Signing Digests {#sign}

To sign digests with a signer, you call `Sign` with the digest you
want to obtain a signature of.

{{< goexample src="gocloud.dev/signers.ExampleSigner_Sign" imports="0" >}}

### Verifying Signatures {#verify}

To verify a signature with a signer, you call `Verify` with the digest and
its signature that you want to verify. This should be the signature that you 
obtained from a previous call to `Sign` with a signer that uses the same secret
material (e.g. two AWS KMS signers created with the same customer master key 
ID). The `Verify` method will return `true` if the signature can be verified,
`false` otherwise.  An error is returned if the digest is not in the proper 
form as required by the key or if there is a problem on the cloud side, e.g.
key does not exist, unauthorized key access, etc.

{{< goexample src="gocloud.dev/signers.ExampleSigner_Verify" imports="0" >}}

## Other Usage Samples

* [CLI Sample](https://github.com/google/go-cloud/tree/master/samples/gocdk-signers)
* [Signers package examples](https://godoc.org/gocloud.dev/signers#example-package)

## Supported Services {#services}

### Google Cloud Key Management Service {#gcp}

The Go CDK can use keys from Google Cloud Platform's [Key Management
Service][GCP KMS] (GCP KMS) to sign and verify digests. GCP KMS URLs are
similar to [key resource IDs][].

[GCP KMS]: https://cloud.google.com/kms/
[key resource IDs]: https://cloud.google.com/kms/docs/object-hierarchy#key

`signers.OpenSigner` will use Application Default Credentials; if you have
authenticated via [`gcloud auth application-default login`][], it will use those credentials. See
[Application Default Credentials][GCP creds] to learn about authentication
alternatives, including using environment variables.

> **Note**: the host component of the URL is used to specify the hashing algorithm
> used to obtain the digest.  It MUST be compatible with the key being used.  

[GCP creds]: https://cloud.google.com/docs/authentication/production
[`gcloud auth application-default login`]: https://cloud.google.com/sdk/gcloud/reference/auth/application-default/login

{{< goexample "gocloud.dev/signers/gcpsig.Example_openFromURL" >}}

#### GCP Constructor {#gcp-ctor}

The [`gcpkms.OpenSigner`][] constructor opens a GCP KMS key. You must first
obtain [GCP credentials][GCP creds] and then create a gRPC connection to GCP KMS.

{{< goexample "gocloud.dev/signers/gcpsig.ExampleOpenSigner" >}}

[`gcpkms.OpenSigner`]: https://godoc.org/gocloud.dev/signers/gcpsig#OpenSigner

### AWS Key Management Service {#aws}

The Go CDK can use customer master keys from Amazon Web Service's [Key
Management Service][AWS KMS] (AWS KMS) to sign and verify digests. AWS KMS
URLs can use the key's ID, alias, or Amazon Resource Name (ARN) to identify
the key. You should specify the `region` query parameter to ensure your
application connects to the correct region.

[AWS KMS]: https://aws.amazon.com/kms/

`signers.OpenSigner` will create a default AWS Session with the
`SharedConfigEnable` option enabled; if you have authenticated with the AWS CLI,
it will use those credentials. See [AWS Session][] to learn about authentication
alternatives, including using environment variables.

> **Note**: the host component of the URL specifies the signing algorithm to be
> used when signing and verifying the digest. _Choose an algorithm that is 
> compatible with the type and size of the specified asymmetric CMK_.

[AWS Session]: https://docs.aws.amazon.com/sdk-for-go/api/aws/session/

{{< goexample "gocloud.dev/signers/awssig.Example_openFromURL" >}}

#### AWS Constructor {#aws-ctor}

The [`awskms.OpenSigner`][] constructor opens a customer master key. You must
first create an [AWS session][] with the same region as your key and then
connect to KMS:

{{< goexample "gocloud.dev/signers/awssig.ExampleOpenSigner" >}}

[`awskms.OpenSigner`]: https://godoc.org/gocloud.dev/signers/awssig#OpenSigner
[AWS session]: https://docs.aws.amazon.com/sdk-for-go/api/aws/session/

### Azure KeyVault {#azure}

The Go CDK can use keys from [Azure KeyVault][] to sign and verify digests.
`signers.OpenSigner` will use [default credentials from the environment][Azure
Environment Auth], unless you set the environment variable
`AZURE_KEYVAULT_AUTH_VIA_CLI` to `true`, in which case it will use
credentials from the `az` command line.

Azure KeyVault URLs are based on the [Azure Key object identifer][Azure Key ID]:

{{< goexample "gocloud.dev/signers/azuresig.Example_openFromURL" >}}

[Azure KeyVault]: https://azure.microsoft.com/en-us/services/key-vault/
[Azure Environment Auth]: https://docs.microsoft.com/en-us/go/azure/azure-sdk-go-authorization#use-environment-based-authentication
[Azure Key ID]: https://docs.microsoft.com/en-us/azure/key-vault/about-keys-secrets-and-certificates

#### Azure Constructor {#azure-ctor}

The [`azuresig.OpenSigner`][] constructor opens an Azure KeyVault key.

{{< goexample "gocloud.dev/signers/azuresig.ExampleOpenSigner" >}}

[`azuresig.OpenSigner`]: https://godoc.org/gocloud.dev/signers/azuresig#OpenSigner

### Local Signers {#local}

The Go CDK can use any valid [`crypto.Hash`] and some pepper as a signers using 
local signers.

>In cryptography, a pepper is a secret added to an input such as a password 
> during hashing with a cryptographic hash function. This value differs from a 
> salt in that it is not stored alongside a password hash, but rather the pepper
> is kept separate in some other medium, such as a Hardware Security Module.
> Note that NIST never refers to this value as a pepper but rather as a secret
> salt. A pepper is similar in concept to a salt or an encryption key. It is 
> like a salt in that it is a randomized value that is added to a password hash, 
> and it is similar to an encryption key in that it should be kept secret.
>
> A pepper performs a comparable role to a salt or an encryption key, but while 
> a salt is not secret (merely unique) and can be stored alongside the hashed 
> output, a pepper is secret and must not be stored with the output. The hash 
> and salt are usually stored in a database, but a pepper must be stored 
> separately to prevent it from being obtained by the attacker in case of a 
> database breach. Where the salt only has to be long enough to be unique per 
> user, a pepper should be long enough to remain secret from brute force 
> attempts to discover it (NIST recommends at least 112 bits). ([wikipedia][wikipedia_pepper])

The implementation of `base64signer` requires that pepper be at least 32 bytes
long.

{{< goexample "gocloud.dev/signers/localsigners.Example_openFromURL" >}}

[`crypto.Hash`]: https://pkg.go.dev/crypto#Hash
[wikipedia_pepper]: https://en.wikipedia.org/wiki/Pepper_(cryptography)

#### Local Signers Constructor {#local-ctor}

The [`localsigners.NewSigner`][] constructor takes in its pepper as
a `[]byte`.

{{< goexample "gocloud.dev/signers/localsigners.ExampleNewSigner" >}}

[`localsigners.NewSigner`]: https://godoc.org/gocloud.dev/signers/localsigners#NewSigner

