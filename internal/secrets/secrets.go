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

// Package secrets provides a set of portable APIs for message encryption and
// decryption.
package secrets // import "gocloud.dev/internal/secrets"

import (
	"context"

	"gocloud.dev/internal/secrets/driver"
)

// Crypter does encryption and decryption. To create a Crypter, use constructors
// found in provider-specific subpackages.
type Crypter struct {
	c driver.Crypter
}

// NewCrypter is intended for use by a specific provider implementation to
// create a Crypter.
func NewCrypter(c driver.Crypter, opts *CrypterOptions) *Crypter {
	return &Crypter{c: c}
}

// Encrypt encrypts the plaintext and returns the cipher message.
func (c *Crypter) Encrypt(ctx context.Context, plaintext []byte) ([]byte, error) {
	b, err := c.Encrypt(ctx, plaintext)
	if err != nil {
		return nil, wrapError(c, err)
	}
	return b, nil
}

// Decrypt decrypts the ciphertext and returns the plaintext or an error.
func (c *Crypter) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {
	b, err := c.Decrypt(ctx, ciphertext)
	if err != nil {
		return nil, wrapError(c, err)
	}
	return b, nil
}

// CrypterOptions controls Crypter behaviors.
// It is provided for future extensibility.
type CrypterOptions struct{}

// wrappedError is used to wrap all errors returned by drivers so that users are
// not given access to provider-specific errors.
type wrappedError struct {
	err error
	c   driver.Crypter
}

func wrapError(c driver.Crypter, err error) error {
	if err == nil {
		return nil
	}
	return &wrappedError{c: c, err: err}
}

func (w *wrappedError) Error() string {
	return "secrets: " + w.err.Error()
}
