// Copyright 2018 The Go Cloud Authors
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

// Package localsecrets provides a way to encrypt and decrypt small messages
// without making network calls to a third party service.
package localsecrets // import "gocloud.dev/secrets/localsecrets"

import (
	"context"
	"crypto/rand"
	"errors"
	"io"

	"gocloud.dev/secrets"
	"golang.org/x/crypto/nacl/secretbox"
)

// keeper holds a secret for use in symmetric encryption,
// and implements driver.Keeper.
type keeper struct {
	secretKey [32]byte // secretbox key size
}

// NewKeeper takes a secret key and returns a Keeper.
func NewKeeper(sk [32]byte) *secrets.Keeper {
	return secrets.NewKeeper(
		&keeper{secretKey: sk},
	)
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
