---
title: "Secrets"
date: 2019-03-21T17:42:18-07:00
lastmod: 2019-07-29T12:00:00-07:00
showInSidenav: true
toc: true
---

The [`secrets` package][] provides access to key management services in a
portable way. This guide shows how to work with secrets in the Go CDK.

<!--more-->

Cloud applications frequently need to store sensitive information like web
API credentials or encryption keys in a medium that is not fully secure. For
example, an application that interacts with GitHub needs to store its OAuth2
client secret and use it when obtaining end-user credentials. If this
information was compromised, it could allow someone else to impersonate the
application. In order to keep such information secret and secure, you can
encrypt the data, but then you need to worry about rotating the encryption
keys and distributing them securely to all of your application servers.
Most Cloud providers include a key management service to perform these tasks,
usually with hardware-level security and audit logging.

The [`secrets` package][] supports encryption and decryption operations.

Subpackages contain driver implementations of secrets for various services,
including Cloud and on-prem solutions. You can develop your application
locally using [`localsecrets`][], then deploy it to multiple Cloud providers with
minimal initialization reconfiguration.

[`secrets` package]: https://godoc.org/gocloud.dev/secrets
[`localsecrets`]: https://godoc.org/gocloud.dev/secrets/localsecrets

## Opening a SecretsKeeper {#opening}

The first step in working with your secrets is
to instantiate a portable [`*secrets.Keeper`][] for your service.

The easiest way to do so is to use [`secrets.OpenKeeper`][] and a service-specific
URL pointing to the keeper, making sure you ["blank import"][] the driver
package to link it in.

```go
import (
	"gocloud.dev/secrets"
	_ "gocloud.dev/secrets/<driver>"
)
...
keeper, err := secrets.OpenKeeper(context.Background(), "<driver-url>")
if err != nil {
    return fmt.Errorf("could not open keeper: %v", err)
}
defer keeper.Close()
// keeper is a *secrets.Keeper; see usage below
...
``` 

See [Concepts: URLs][] for general background and the [guide below][]
for URL usage for each supported service.

Alternatively, if you need
fine-grained control over the connection settings, you can call the constructor
function in the driver package directly (like `awskms.OpenKeeper`).

```go
import "gocloud.dev/secrets/<driver>"
...
keeper, err := <driver>.OpenKeeper(...)
...
```

You may find the [`wire` package][] useful for managing your initialization code
when switching between different backing services.

See the [guide below][] for constructor usage for each supported service.

[`*secrets.Keeper`]: https://godoc.org/gocloud.dev/secrets#Keeper
[`secrets.OpenKeeper`]:
https://godoc.org/gocloud.dev/secrets#OpenKeeper
["blank import"]: https://golang.org/doc/effective_go.html#blank_import
[Concepts: URLs]: {{< ref "/concepts/urls.md" >}}
[guide below]: {{< ref "#services" >}}
[`wire` package]: http://github.com/google/wire

## Using a SecretsKeeper {#using}

Once you have [opened a secrets keeper][] for the secrets provider you want,
you can encrypt and decrypt small messages using the keeper.

[opened a secrets keeper]: {{< ref "#opening" >}}

### Encrypting Data {#encrypt}

To encrypt data with a keeper, you call `Encrypt` with the byte slice you
want to encrypt.

{{< goexample src="gocloud.dev/secrets.ExampleKeeper_Encrypt" imports="0" >}}

### Decrypting Data {#decrypt}

To decrypt data with a keeper, you call `Decrypt` with the byte slice you
want to decrypt. This should be data that you obtained from a previous call
to `Encrypt` with a keeper that uses the same secret material (e.g. two AWS
KMS keepers created with the same customer master key ID). The `Decrypt`
method will return an error if the input data is corrupted.

{{< goexample src="gocloud.dev/secrets.ExampleKeeper_Decrypt" imports="0" >}}

### Large Messages {#large-messages}

The secrets keeper API is designed to work with small messages (i.e. <10 KiB
in length.) Cloud key management services are high latency; using them for
encrypting or decrypting large amounts of data is prohibitively slow (and in
some providers not permitted). If you need your application to encrypt or
decrypt large amounts of data, you should:

1. Generate a key for the encryption algorithm (16KiB chunks with
   [`secretbox`][] is a reasonable approach).
2. Encrypt the key with secret keeper.
3. Store the encrypted key somewhere accessible to the application.

When your application needs to encrypt or decrypt a large message:

1. Decrypt the key from storage using the secret keeper
2. Use the decrypted key to encrypt or decrypt the message inside your
   application.

[`secretbox`]: https://godoc.org/golang.org/x/crypto/nacl/secretbox

### Keep Secrets in Configuration {#runtimevar}

Once you have [opened a secrets keeper][] for the secrets provider you want,
you can use a secrets keeper to access sensitive configuration stored in an
encrypted `runtimevar`.

First, you create a [`*runtimevar.Decoder`][] configured to use your secrets
keeper using [`runtimevar.DecryptDecode`][]. In this example, we assume the
data is a plain string, but the configuration could be a more structured
type.

{{< goexample src="gocloud.dev/runtimevar.ExampleDecryptDecode" imports="0" >}}

Then you can pass the decoder to the runtime configuration provider of your
choice. See the [Runtime Configuration How-To Guide][] for more on how to set up
runtime configuration.

[opened a secrets keeper]: {{< ref "#opening" >}}
[Runtime Configuration How-To Guide]: {{< ref "/howto/runtimevar/_index.md" >}}
[`*runtimevar.Decoder`]: https://godoc.org/gocloud.dev/runtimevar#Decoder
[`runtimevar.DecryptDecode`]: https://godoc.org/gocloud.dev/runtimevar#DecryptDecode

## Other Usage Samples

* [CLI Sample](https://github.com/google/go-cloud/tree/master/samples/gocdk-secrets)
* [Secrets package examples](https://godoc.org/gocloud.dev/secrets#example-package)

## Supported Services {#services}

### Google Cloud Key Management Service {#gcp}

The Go CDK can use keys from Google Cloud Platform's [Key Management
Service][GCP KMS] (GCP KMS) to keep information secret. GCP KMS URLs are
similar to [key resource IDs][].

[GCP KMS]: https://cloud.google.com/kms/
[key resource IDs]: https://cloud.google.com/kms/docs/object-hierarchy#key

`secrets.OpenKeeper` will use Application Default Credentials; if you have
authenticated via [`gcloud auth application-default login`][], it will use those credentials. See
[Application Default Credentials][GCP creds] to learn about authentication
alternatives, including using environment variables.

[GCP creds]: https://cloud.google.com/docs/authentication/production
[`gcloud auth application-default login`]: https://cloud.google.com/sdk/gcloud/reference/auth/application-default/login

{{< goexample "gocloud.dev/secrets/gcpkms.Example_openFromURL" >}}

#### GCP Constructor {#gcp-ctor}

The [`gcpkms.OpenKeeper`][] constructor opens a GCP KMS key. You must first
obtain [GCP credentials][GCP creds] and then create a gRPC connection to GCP KMS.

{{< goexample "gocloud.dev/secrets/gcpkms.ExampleOpenKeeper" >}}

[`gcpkms.OpenKeeper`]: https://godoc.org/gocloud.dev/secrets/gcpkms#OpenKeeper

### AWS Key Management Service {#aws}

The Go CDK can use customer master keys from Amazon Web Service's [Key
Management Service][AWS KMS] (AWS KMS) to keep information secret. AWS KMS
URLs can use the key's ID, alias, or Amazon Resource Name (ARN) to identify
the key. You should specify the `region` query parameter to ensure your
application connects to the correct region.

[AWS KMS]: https://aws.amazon.com/kms/

It will create an AWS Config based on the AWS SDK V2; see [AWS V2 Config][] to learn more.

[AWS V2 Config]: https://aws.github.io/aws-sdk-go-v2/docs/configuring-sdk/

{{< goexample "gocloud.dev/secrets/awskms.Example_openFromURL" >}}

#### AWS Constructor {#aws-ctor}

The [`awskms.OpenKeeper`][] constructor opens a customer master key. You must
first create an AWS Config with the same region as your key and then
connect to KMS:

{{< goexample "gocloud.dev/secrets/awskms.ExampleOpenKeeper" >}}

[`awskms.OpenKeeper`]: https://godoc.org/gocloud.dev/secrets/awskms#OpenKeeper

### Azure KeyVault {#azure}

The Go CDK can use keys from [Azure KeyVault][] to keep information secret.
`secrets.OpenKeeper` will use [default credentials from the environment][Azure
Environment Auth], unless you set the environment variable
`AZURE_KEYVAULT_AUTH_VIA_CLI` to `true`, in which case it will use
credentials from the `az` command line.

Azure KeyVault URLs are based on the [Azure Key object identifer][Azure Key ID]:

{{< goexample "gocloud.dev/secrets/azurekeyvault.Example_openFromURL" >}}

[Azure KeyVault]: https://azure.microsoft.com/en-us/services/key-vault/
[Azure Environment Auth]: https://docs.microsoft.com/en-us/go/azure/azure-sdk-go-authorization#use-environment-based-authentication
[Azure Key ID]: https://docs.microsoft.com/en-us/azure/key-vault/about-keys-secrets-and-certificates

#### Azure Constructor {#azure-ctor}

The [`azurekeyvault.OpenKeeper`][] constructor opens an Azure KeyVault key.

{{< goexample "gocloud.dev/secrets/azurekeyvault.ExampleOpenKeeper" >}}

[`azurekeyvault.OpenKeeper`]: https://godoc.org/gocloud.dev/secrets/azurekeyvault#OpenKeeper

### HashiCorp Vault {#vault}

The Go CDK can use the [transit secrets engine][] in [Vault][] to keep
information secret. Vault URLs only specify the key ID. The Vault server
endpoint and authentication token are specified using the environment
variables `VAULT_SERVER_URL` and `VAULT_SERVER_TOKEN`, respectively.

{{< goexample "gocloud.dev/secrets/hashivault.Example_openFromURL" >}}

[Vault]: https://www.vaultproject.io/
[transit secrets engine]: https://www.vaultproject.io/docs/secrets/transit/index.html

#### HashiCorp Vault Constructor {#vault-ctor}

The [`hashivault.OpenKeeper`][] constructor opens a transit secrets engine
key. You must first connect to your Vault instance.

{{< goexample "gocloud.dev/secrets/hashivault.ExampleOpenKeeper" >}}

[`hashivault.OpenKeeper`]: https://godoc.org/gocloud.dev/secrets/hashivault#OpenKeeper

### Local Secrets {#local}

The Go CDK can use local encryption for keeping secrets. Internally, it uses
the [NaCl secret box][] algorithm to perform encryption and authentication.

{{< goexample "gocloud.dev/secrets/localsecrets.Example_openFromURL" >}}

[NaCl secret box]: https://godoc.org/golang.org/x/crypto/nacl/secretbox

#### Local Secrets Constructor {#local-ctor}

The [`localsecrets.NewKeeper`][] constructor takes in its secret material as
a `[]byte`.

{{< goexample "gocloud.dev/secrets/localsecrets.ExampleNewKeeper" >}}

[`localsecrets.NewKeeper`]: https://godoc.org/gocloud.dev/secrets/localsecrets#NewKeeper

