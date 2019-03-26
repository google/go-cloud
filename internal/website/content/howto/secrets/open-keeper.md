---
title: "Open a Secret Keeper"
date: 2019-03-21T17:43:54-07:00
draft: true
weight: 1
---

The first step in working with your secrets is establishing your
secret keeper provider. Every secret keeper provider is a little different, but the Go CDK
lets you interact with all of them using the [`*secrets.Keeper` type][].

[`*secrets.Keeper` type]: https://godoc.org/gocloud.dev/secrets#Keeper

## Constructors versus URL openers

If you know that your program is always going to use a particular secret
keeper provider or you need fine-grained control over the connection
settings, you should call the constructor function in the driver package
directly (like `awskms.NewKeeper`). However, if you want to change providers
based on configuration, you can use `secrets.OpenKeeper`, making sure you
["blank import"][] the driver package to link it in. This guide will show how
to use both forms for each secret keeper provider.

["blank import"]: https://golang.org/doc/effective_go.html#blank_import

## AWS Key Management Service

The Go CDK can use customer master keys from Amazon Web Service's [Key
Management Service][AWS KMS] (AWS KMS) to keep information secret. AWS KMS
URLs can use the key's ID, alias, or Amazon Resource Name (ARN) to identify
the key. You can specify the `region` query parameter to ensure your
application connects to the correct region, but otherwise
`secrets.OpenKeeper` will use the region found in the environment variable
`AWS_REGION` or your AWS CLI configuration.

```go
import (
    "gocloud.dev/secrets"
    _ "gocloud.dev/secrets/awskms"
)

// ...

// Use one of the following:

// 1. By ID.
keeperByID, err := secrets.OpenKeeper(ctx,
    "awskms://1234abcd-12ab-34cd-56ef-1234567890ab?region=us-east-1")
if err != nil {
    return err
}

// 2. By alias.
keeperByAlias, err := secrets.OpenKeeper(ctx,
    "awskms://alias/ExampleAlias?region=us-east-1")
if err != nil {
    return err
}

// 2. By ARN.
const arn = "arn:aws:kms:us-east-1:111122223333:key/" +
    "1234abcd-12ab-34bc-56ef-1234567890ab"
keeperByARN, err := secrets.OpenKeeper(ctx,
    "awskms://" + arn + "?region=us-east-1")
if err != nil {
    return err
}
```

[AWS KMS]: https://aws.amazon.com/kms/

## AWS Key Management Service Constructor

The [`awskms.NewKeeper`][] constructor opens a customer master key. You must
first create an [AWS session][] with the same region as your key and then
connect to KMS:

```go
import (
    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/aws/session"
    "gocloud.dev/secrets/awskms"
)

// ...

// Establish an AWS session.
// The region must match the region for "master-key-test".
sess, err := session.NewSession(&aws.Config{
    Region: aws.String("us-east-1"),
})
if err != nil {
    return err
}

// Connect to KMS.
kmsClient, err := awskms.Dial(sess)
if err != nil {
    return err
}

// Create a *secrets.Keeper.
bucket, err := awskms.NewKeeper(kmsClient, "alias/master-key-test", nil)
if err != nil {
    return err
}
```

[`awskms.NewKeeper`]: https://godoc.org/gocloud.dev/secrets/awskms#NewKeeper
[AWS session]: https://docs.aws.amazon.com/sdk-for-go/api/aws/session/

## Google Cloud Key Management Service

The Go CDK can use keys from Google Cloud Platform's [Key Management
Service][GCP KMS] (GCP KMS) to keep information secret. GCP KMS URLs are very
similar to [key resource IDs][]:

```go
import (
    "gocloud.dev/secrets"
    _ "gocloud.dev/secrets/gcpkms"
)

// ...

keeper, err := secrets.OpenKeeper(ctx,
    "gcpkms://projects/MYPROJECT/" +
    "locations/MYLOCATION/" +
    "keyRings/MYKEYRING/" +
    "cryptoKeys/MYKEY")
if err != nil {
    return err
}
```

[GCP KMS]: https://cloud.google.com/kms/
[key resource IDs]: https://cloud.google.com/kms/docs/object-hierarchy#key

### Google Cloud Key Management Service Constructor

The [`gcpkms.NewKeeper`][] constructor opens a GCP KMS key. You must first
obtain [GCP credentials][] and then create a gRPC connection to GCP KMS.

```go
import (
    "gocloud.dev/gcp"
    "gocloud.dev/secrets/gcpkms"
)

// ...

// Your GCP credentials.
// See https://cloud.google.com/docs/authentication/production
// for more info on alternatives.
creds, err := gcp.DefaultCredentials(ctx)
if err != nil {
    return err
}

// Create a KMS client.
kmsClient, err := gcpkms.Dial(ctx, gcp.CredentialsTokenSource(creds))
if err != nil {
    return err
}

// Create a *secrets.Keeper.
const keyID = gcpkms.KeyResourceID(
    "MYPROJECT", "MYLOCATION", "MYKEYRING", "MYKEY")
keeper, err := gcpkms.NewKeeper(client, keyID, nil)
if err != nil {
    return err
}
```

[GCP credentials]: https://cloud.google.com/docs/authentication/production
[`gcpkms.NewKeeper`]: https://godoc.org/gocloud.dev/secrets/gcpkms#NewKeeper

## HashiCorp Vault

The Go CDK can use the [transit secrets engine][] in [Vault][] to keep
information secret. Vault URLs only specify the key ID. The Vault server
endpoint and authentication token are specified using the environment
variables `VAULT_SERVER_URL` and `VAULT_SERVER_TOKEN`, respectively.

```go
import (
    "gocloud.dev/secrets"
    _ "gocloud.dev/secrets/vault"
)

// ...

keeper, err := secrets.OpenKeeper(ctx, "vault://my-key")
if err != nil {
    return err
}
```

[Vault]: https://www.vaultproject.io/
[transit secrets engine]: https://www.vaultproject.io/docs/secrets/transit/index.html

### HashiCorp Vault Constructor

The [`vault.NewKeeper`][] constructor opens a transit secrets engine key. You
must first connect to your Vault instance.

```go
import (
    "github.com/hashicorp/vault/api"
    "gocloud.dev/secrets/vault"
)

// ...

vaultClient, err := vault.Dial(ctx, &vault.Config{
    Token: "CLIENT_TOKEN",
    APIConfig: api.Config{
        Address: "http://127.0.0.1:8200",
    },
})
if err != nil {
    return err
}
keeper := vault.NewKeeper(vaultClient, "my-key", nil)
```

[`vault.NewKeeper`]: https://godoc.org/gocloud.dev/secrets/vault#NewKeeper

## Local Secrets

**TODO(light):** Document https://godoc.org/gocloud.dev/secrets/localsecrets

**Blocked on https://github.com/google/go-cloud/issues/1664**

## What's Next

Now that you have opened a secrets keeper, you can [encrypt and decrypt
data][] and [access secret configuration data][] using portable operations.

[access secret configuration data]: {{< ref "./runtimevar.md" >}}
[encrypt and decrypt data]: {{< ref "./crypt.md" >}}

