---
title: "Open a Secret Keeper"
date: 2019-03-21T17:43:54-07:00
weight: 1
---

The first step in working with your secrets is establishing your
secret keeper provider. Every secret keeper provider is a little different, but the Go CDK
lets you interact with all of them using the [`*secrets.Keeper` type][].

[`*secrets.Keeper` type]: https://godoc.org/gocloud.dev/secrets#Keeper

<!--more-->

## Constructors versus URL openers

If you know that your program is always going to use a particular secret
keeper provider or you need fine-grained control over the connection
settings, you should call the constructor function in the driver package
directly (like `awskms.OpenKeeper`). However, if you want to change providers
based on configuration, you can use `secrets.OpenKeeper`, making sure you
["blank import"][] the driver package to link it in. See the
[documentation on URLs][] for more details. This guide will show how to use
both forms for each secret keeper provider.

["blank import"]: https://golang.org/doc/effective_go.html#blank_import
[documentation on URLs]: {{< ref "/concepts/urls.md" >}}

## AWS Key Management Service {#aws}

The Go CDK can use customer master keys from Amazon Web Service's [Key
Management Service][AWS KMS] (AWS KMS) to keep information secret. AWS KMS
URLs can use the key's ID, alias, or Amazon Resource Name (ARN) to identify
the key. You can specify the `region` query parameter to ensure your
application connects to the correct region, but otherwise
`secrets.OpenKeeper` will use the region found in the environment variable
`AWS_REGION` or your AWS CLI configuration.

{{< goexample "gocloud.dev/secrets/awskms.Example_openFromURL" >}}

[AWS KMS]: https://aws.amazon.com/kms/

## AWS Key Management Service Constructor {#aws-ctor}

The [`awskms.OpenKeeper`][] constructor opens a customer master key. You must
first create an [AWS session][] with the same region as your key and then
connect to KMS:

{{< goexample "gocloud.dev/secrets/awskms.ExampleOpenKeeper" >}}

[`awskms.OpenKeeper`]: https://godoc.org/gocloud.dev/secrets/awskms#OpenKeeper
[AWS session]: https://docs.aws.amazon.com/sdk-for-go/api/aws/session/

## Google Cloud Key Management Service {#gcp}

The Go CDK can use keys from Google Cloud Platform's [Key Management
Service][GCP KMS] (GCP KMS) to keep information secret. `secrets.OpenKeeper`
will use [Application Default Credentials][GCP credentials]. GCP KMS URLs are
similar to [key resource IDs][]:

{{< goexample "gocloud.dev/secrets/gcpkms.Example_openFromURL" >}}

[GCP KMS]: https://cloud.google.com/kms/
[key resource IDs]: https://cloud.google.com/kms/docs/object-hierarchy#key

### Google Cloud Key Management Service Constructor {#gcp-ctor}

The [`gcpkms.OpenKeeper`][] constructor opens a GCP KMS key. You must first
obtain [GCP credentials][] and then create a gRPC connection to GCP KMS.

{{< goexample "gocloud.dev/secrets/gcpkms.ExampleOpenKeeper" >}}

[GCP credentials]: https://cloud.google.com/docs/authentication/production
[`gcpkms.OpenKeeper`]: https://godoc.org/gocloud.dev/secrets/gcpkms#OpenKeeper

## HashiCorp Vault {#vault}

The Go CDK can use the [transit secrets engine][] in [Vault][] to keep
information secret. Vault URLs only specify the key ID. The Vault server
endpoint and authentication token are specified using the environment
variables `VAULT_SERVER_URL` and `VAULT_SERVER_TOKEN`, respectively.

{{< goexample "gocloud.dev/secrets/hashivault.Example_openFromURL" >}}

[Vault]: https://www.vaultproject.io/
[transit secrets engine]: https://www.vaultproject.io/docs/secrets/transit/index.html

### HashiCorp Vault Constructor {#vault-ctor}

The [`hashivault.OpenKeeper`][] constructor opens a transit secrets engine
key. You must first connect to your Vault instance.

{{< goexample "gocloud.dev/secrets/hashivault.ExampleOpenKeeper" >}}

[`hashivault.OpenKeeper`]: https://godoc.org/gocloud.dev/secrets/hashivault#OpenKeeper

## Local Secrets {#local}

The Go CDK can use local encryption for keeping secrets. Internally, it uses
the [NaCl secret box][] algorithm to perform encryption and authentication.

{{< goexample "gocloud.dev/secrets/localsecrets.Example_openFromURL" >}}

[NaCl secret box]: https://godoc.org/golang.org/x/crypto/nacl/secretbox

### Local Secrets Constructor {#local-ctor}

The [`localsecrets.NewKeeper`][] constructor takes in its secret material as
a `[]byte`.

{{< goexample "gocloud.dev/secrets/localsecrets.ExampleNewKeeper" >}}

[`localsecrets.NewKeeper`]: https://godoc.org/gocloud.dev/secrets/localsecrets#NewKeeper

## What's Next

Now that you have opened a secrets keeper, you can [encrypt and decrypt
data][] using portable operations.

[encrypt and decrypt data]: {{< ref "./crypt.md" >}}

