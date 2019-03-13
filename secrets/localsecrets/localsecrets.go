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
	return NewKeeper(ByteKey(u.Host)), nil
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
// to a [32]byte, cropping it if necessary.
func Base64Key(base64str string) ([32]byte, error) {
	var sk32 [32]byte
	key, err := base64.StdEncoding.DecodeString(base64str)
	if err != nil {
		return sk32, err
	}
	copy(sk32[:], key)
	return sk32, nil
}

// ByteKey takes a secret key as a string and converts it
// to a [32]byte, cropping it if necessary.
func ByteKey(sk string) [32]byte {
	var sk32 [32]byte
	copy(sk32[:], []byte(sk))
	return sk32
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
	var decryptNonce [nonceSize]byte
	copy(decryptNonce[:], message[:nonceSize])

	decrypted, ok := secretbox.Open(nil, message[nonceSize:], &decryptNonce, &k.secretKey)
	if !ok {
		return nil, errors.New("localsecrets: Decrypt failed")
	}
	return decrypted, nil
}

// ErrorAs implements driver.Keeper.ErrorAs.
func (k *keeper) ErrorAs(err error, i interface{}) bool {
	return false
}

// ErrorCode implements driver.ErrorCode.
func (k *keeper) ErrorCode(error) gcerrors.ErrorCode { return gcerrors.Unknown }
