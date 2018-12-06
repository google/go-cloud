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

// Package driver defines interfaces to be implemented for providers of the
// secrets package.
package driver

import "context"

// Decrypter decrypts a cipher message into a plain text message.
type Decrypter interface {

	// Decrypt decrypts the ciphertext and returns the plaintext or an error.
	Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error)
}

// Encrypter encrypts a plain text message into a cipher message.
type Encrypter interface {

	// Encrypt encrypts the plaintext and returns the cipher message.
	Encrypt(ctx context.Context, plaintext []byte) ([]byte, error)
}

// Crypter defines an interface satisfied by implementing both
// Encrypterand Decrypter. This composed interface reflects the
// inherent tie between compatible pairs, ie to decrypt a message,
// Decrypter must be based on the same underlying credentials as
// the Encrypter which encrypted the message.
type Crypter interface {
	Encrypter
	Decrypter
}
