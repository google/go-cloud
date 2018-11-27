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

package localsecrets

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"golang.org/x/crypto/nacl/secretbox"
	"io"
	"time"
)

//
type Crypter struct {
	secretKey [32]byte
}

//
func NewCrypter() (*Crypter, error) { //return driver.Decrypter once I pull in that branch
	sk := []byte(time.Now().String())
	enc := base64.StdEncoding
	dst := make([]byte, enc.EncodedLen(len(sk)))
	enc.Encode(dst, sk)

	var dst32 [32]byte
	copy(dst32[:], dst)

	return &Crypter{secretKey: dst32}, nil
}

//
func (n Crypter) Encrypt(ctx context.Context, message []byte) ([]byte, error) {
	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return nil, err
	}
	// a slice beginning at nonce is used here as the destination for the encrypted message,
	// so that we can read the nonce out of the first 24 bytes when we decrypt it
	return secretbox.Seal(nonce[:], message, &nonce, &n.secretKey), nil
}

//
func (d Crypter) Decrypt(ctx context.Context, message []byte) ([]byte, error) {
	var decryptNonce [24]byte
	copy(decryptNonce[:], message[:24])

	decrypted, ok := secretbox.Open(nil, message[24:], &decryptNonce, &d.secretKey)
	if !ok {
		return nil, errors.New("it failed")
	}
	return decrypted, nil
}
