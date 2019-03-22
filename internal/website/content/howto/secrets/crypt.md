---
title: "Encrypt & Decrypt Data"
date: 2019-03-21T17:44:28-07:00
draft: true
weight: 2
---

Once you have [opened a secrets keeper][] for the secrets provider you want,
you can encrypt and decrypt small messages using the keeper.

[opened a secrets keeper]: {{< ref "./open-keeper.md" >}}

## Encrypting data

To encrypt data with a keeper, you call `Encrypt` with the byte slice you
want to encrypt.

```go
plainText := []byte("Secrets secrets...")
cipherText, err := keeper.Encrypt(ctx, plainText)
if err != nil {
    return err
}
```

## Decrypting data

To decrypt data with a keeper, you call `Decrypt` with the byte slice you
want to decrypt. This should be data that you obtained from a previous call
to `Encrypt` with a keeper that uses the same secret material (e.g. two AWS
KMS keepers created with the same customer master key ID). The `Decrypt`
method will return an error if the input data did not come from the same
secret keeper (**TODO(light)**: verify with @clausti and @shantuo that this
is correct).

```go
var cipherText []byte // obtained from elsewhere and random-looking
plainText, err := keeper.Decrypt(ctx, cipherText)
if err != nil {
    return err
}
```

## Large Messages

The secrets keeper API is designed to work with small messages (i.e. <10 KiB
in length.) Cloud key management services are high enough latency that using
them for encrypting or decrypting large amounts of data is prohibitively slow
(and in some cases not permitted). If you need your application to encrypt or
decrypt large amounts of data, your application should generate a key for the
encryption algorithm (16KiB chunks with [`secretbox`][] is a reasonable
approach), encrypt the key with the secret keeper, and then store the
encrypted key somewhere accessible to the application. When your application
needs to encrypt or decrypt a large message, it would first decrypt the key
from storage using the secret keeper then use the decrypted key to encrypt or
decrypt the message in your application's local address space.

[`secretbox`]: https://godoc.org/golang.org/x/crypto/nacl/secretbox
