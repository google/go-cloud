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
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"time"
)

type EncrypterDecrypter struct {
	keyString []byte
	cipher    cipher.AEAD //encrypt and decrypt methods
}

func NewEncrypterDecrypter() (*EncrypterDecrypter, error) { //return driver.Decrypter once I pull in that branch
	ks := []byte(time.Now().String())
	enc := base64.StdEncoding
	dst := make([]byte, enc.EncodedLen(len(ks)))
	enc.Encode(dst, ks)

	cblock, err := aes.NewCipher(dst[:32])
	if err != nil {
		return nil, err
	}
	c, err := cipher.NewGCM(cblock)
	if err != nil {
		return nil, err
	}

	return &EncrypterDecrypter{keyString: dst[:32], cipher: c}, nil
}

func (d EncrypterDecrypter) Decrypt(ctx context.Context, cipherText []byte) ([]byte, error) {
	nonce := make([]byte, d.cipher.NonceSize())
	return d.cipher.Open(nil, nonce, cipherText, nil)
}

func (e EncrypterDecrypter) Encrypt(ctx context.Context, message []byte) ([]byte, error) {
	return nil, nil
}
