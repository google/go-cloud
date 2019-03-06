---
title: "Secrets"
---

Package `secrets` provides an easy and portable way to encrypt and decrypt
messages.

This package lets you do symmetric encryption and decryption within your
application layer. You can then store the secret data anywhere safely, for
example, using our blob, runtimevar packages etc. It is also integrated with
OpenCensus to get metrics and traces for `Encrypt` and `Decrypt`, and some
providers also have automatic audit logs for all key management related
activites.

Top-level package documentation: <https://godoc.org/gocloud.dev/secrets>

## Supported Providers

* [Google Cloud KMS](https://godoc.org/gocloud.dev/secrets/gcpkms)
* [AWS KMS](https://godoc.org/gocloud.dev/secrets/awskms)
* [Vault by HashiCorp](https://godoc.org/gocloud.dev/secrets/vault) - a
  platform-agnostic secrets engine
* [In-memory local secrets](https://godoc.org/gocloud.dev/secrets/localsecrets) -
  mainly useful for local testing

## Usage Samples

* [Tutorial sample](https://github.com/google/go-cloud/tree/master/samples/tutorial)
* [Secrets package examples](https://godoc.org/gocloud.dev/secrets#example-package)
