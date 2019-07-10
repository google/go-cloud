---
title: "Encrypt & Decrypt Data"
date: 2019-03-21T17:44:28-07:00
weight: 2
toc: true
---

Once you have [opened a secrets keeper][] for the secrets provider you want,
you can encrypt and decrypt small messages using the keeper.

[opened a secrets keeper]: {{< ref "./open-keeper.md" >}}

<!--more-->

## Encrypting data {#encrypt}

To encrypt data with a keeper, you call `Encrypt` with the byte slice you
want to encrypt.

{{< goexample src="gocloud.dev/secrets.ExampleKeeper_Encrypt" imports="0" >}}

## Decrypting data {#decrypt}

To decrypt data with a keeper, you call `Decrypt` with the byte slice you
want to decrypt. This should be data that you obtained from a previous call
to `Encrypt` with a keeper that uses the same secret material (e.g. two AWS
KMS keepers created with the same customer master key ID). The `Decrypt`
method will return an error if the input data is corrupted.

{{< goexample src="gocloud.dev/secrets.ExampleKeeper_Decrypt" imports="0" >}}

## Large Messages

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
