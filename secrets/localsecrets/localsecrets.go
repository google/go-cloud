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

// Package localsecrets provides a secrets implementation using a locally
// locally provided symmetric key.
// Use NewKeeper to construct a *secrets.Keeper.
//
// URLs
//
// For secrets.OpenKeeper, localsecrets registers for the schemes "stringkey"
// and "base64key".
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://godoc.org/gocloud.dev#hdr-URLs for background information.
//
// As
//
// localsecrets does not support any types for As.
package localsecrets // import "gocloud.dev/secrets/localsecrets"

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"

	"gocloud.dev/gcerrors"
	"gocloud.dev/secrets"
	"golang.org/x/crypto/nacl/secretbox"
)

func init() {
	secrets.DefaultURLMux().RegisterKeeper(SchemeString, &URLOpener{})
	secrets.DefaultURLMux().RegisterKeeper(SchemeBase64, &URLOpener{base64: true})
}

// SchemeString/SchemeBase64 are the URL schemes localsecrets registers its URLOpener under on secrets.DefaultMux.
// See the package documentation and/or URLOpener for details.
const (
	SchemeString = "stringkey"
	SchemeBase64 = "base64key"
)

// URLOpener opens localsecrets URLs like "stringkey://mykey" and "base64key://c2VjcmV0IGtleQ==".
//
// For stringkey, the first 32 bytes of the URL host are used as the symmetric
// key for encryption/decryption.
//
// For base64key, the URL host must be a base64 encoded string; the first 32
// bytes of the decoded bytes are used as the symmetric key for
// encryption/decryption.
//
// No query parameters are supported.
type URLOpener struct {
	base64 bool
}

// OpenKeeperURL opens Keeper URLs.
func (o *URLOpener) OpenKeeperURL(ctx context.Context, u *url.URL) (*secrets.Keeper, error) {
	for param := range u.Query() {
		return nil, fmt.Errorf("open keeper %v: invalid query parameter %q", u, param)
	}
	if o.base64 {
		sk, err := Base64Key(u.Host)
		if err != nil {
			return nil, fmt.Errorf("open keeper %v: base64 decode failed: %v", u, err)
		}
		return NewKeeper(sk), nil
	}
	sk, err := ByteKey(u.Host)
	if err != nil {
		return nil, err
	}
	return NewKeeper(sk), nil
}

// keeper holds a secret for use in symmetric encryption,
// and implements driver.Keeper.
type keeper struct {
	secretKey [32]byte // secretbox key size
}

// NewKeeper returns a *secrets.Keeper that uses the given symmetric
// key. See the package documentation for an example.
func NewKeeper(sk [32]byte) *secrets.Keeper {
	return secrets.NewKeeper(
		&keeper{secretKey: sk},
	)
}

// Base64Key takes a secret key as a base64 string and converts it
// to a [32]byte, erroring if the decoded data is not 32 bytes.
func Base64Key(base64str string) ([32]byte, error) {
	var sk32 [32]byte
	key, err := base64.StdEncoding.DecodeString(base64str)
	if err != nil {
		return sk32, err
	}
	keySize := len([]byte(key))
	if keySize != 32 {
		return sk32, fmt.Errorf("Base64Key: secret key material is %v bytes, want 32 bytes", keySize)
	}
	copy(sk32[:], key)
	return sk32, nil
}

// ByteKey takes a secret key as a string and converts it
// to a [32]byte, erroring if the string is not 32 bytes of data.
func ByteKey(sk string) ([32]byte, error) {
	var sk32 [32]byte
	skb := []byte(sk)
	keySize := len(skb)
	if keySize != 32 {
		return sk32, fmt.Errorf("ByteKey: secret key material is %v bytes, want 32 bytes", keySize)
	}
	copy(sk32[:], skb)
	return sk32, nil
}

// NewRandomKey will generate random secret key material suitable to be
// used as the secret key argument to NewKeeper.
func NewRandomKey() ([32]byte, error) {
	var sk32 [32]byte
	// Read random numbers into the passed slice until it's full.
	_, err := rand.Read(sk32[:])
	if err != nil {
		return sk32, err
	}
	return sk32, nil
}

const nonceSize = 24

// Encrypt encrypts a message using a per-message generated nonce and
// the secret held in the Keeper.
func (k *keeper) Encrypt(ctx context.Context, message []byte) ([]byte, error) {
	var nonce [nonceSize]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return nil, err
	}
	// secretbox.Seal appends the encrypted message to its first argument and returns
	// the result; using a slice on top of the nonce array for this "out" arg allows reading
	// the nonce out of the first nonceSize bytes when the message is decrypted.
	return secretbox.Seal(nonce[:], message, &nonce, &k.secretKey), nil
}

// Decrypt decryptes a message using a nonce that is read out of the first nonceSize bytes
// of the message and a secret held in the Keeper.
func (k *keeper) Decrypt(ctx context.Context, message []byte) ([]byte, error) {
	if len(message) < nonceSize {
		return nil, fmt.Errorf("localsecrets: invalid message length (%d, expected at least %d)", len(message), nonceSize)
	}
	var decryptNonce [nonceSize]byte
	copy(decryptNonce[:], message[:nonceSize])

	decrypted, ok := secretbox.Open(nil, message[nonceSize:], &decryptNonce, &k.secretKey)
	if !ok {
		return nil, errors.New("localsecrets: Decrypt failed")
	}
	return decrypted, nil
}

// Close implements driver.Keeper.Close.
func (k *keeper) Close() error { return nil }

// ErrorAs implements driver.Keeper.ErrorAs.
func (k *keeper) ErrorAs(err error, i interface{}) bool {
	return false
}

// ErrorCode implements driver.ErrorCode.
func (k *keeper) ErrorCode(error) gcerrors.ErrorCode { return gcerrors.Unknown }
