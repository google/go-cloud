// Copyright 2019 The Go Cloud Authors
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

// Package secrets provides an easy and portable way to encrypt and decrypt
// messages.
//
// Subpackages contain distinct implementations of secrets for various
// providers, including Cloud and on-prem solutions. For example, "localsecrets"
// supports encryption/decryption using a locally provided key. Your application
// should import one of these provider-specific subpackages and use its exported
// function(s) to create a *Keeper; do not use the NewKeeper function in this
// package. For example:
//
//  keeper := localsecrets.NewKeeper(myKey)
//  encrypted, err := keeper.Encrypt(ctx.Background(), []byte("text"))
//  ...
//
// Then, write your application code using the *Keeper type. You can easily
// reconfigure your initialization code to choose a different provider.
// You can develop your application locally using localsecrets, or deploy it to
// multiple Cloud providers. You may find http://github.com/google/wire useful
// for managing your initialization code.
package secrets // import "gocloud.dev/secrets"

import (
	"context"

	"gocloud.dev/internal/trace"
	"gocloud.dev/secrets/driver"
)

// Keeper does encryption and decryption. To create a Keeper, use constructors
// found in provider-specific subpackages.
type Keeper struct {
	k driver.Keeper
}

// NewKeeper is intended for use by provider implementations.
var NewKeeper = newKeeper

// newKeeper creates a Keeper.
func newKeeper(k driver.Keeper) *Keeper {
	return &Keeper{k: k}
}

// Encrypt encrypts the plaintext and returns the cipher message.
func (k *Keeper) Encrypt(ctx context.Context, plaintext []byte) (ciphertext []byte, err error) {
	ctx = trace.StartSpan(ctx, "gocloud.dev/secrets.Encrypt")
	defer func() { trace.EndSpan(ctx, err) }()

	b, err := k.k.Encrypt(ctx, plaintext)
	if err != nil {
		return nil, wrapError(k, err)
	}
	return b, nil
}

// Decrypt decrypts the ciphertext and returns the plaintext.
func (k *Keeper) Decrypt(ctx context.Context, ciphertext []byte) (plaintext []byte, err error) {
	ctx = trace.StartSpan(ctx, "gocloud.dev/secrets.Decrypt")
	defer func() { trace.EndSpan(ctx, err) }()

	b, err := k.k.Decrypt(ctx, ciphertext)
	if err != nil {
		return nil, wrapError(k, err)
	}
	return b, nil
}

// wrappedError is used to wrap all errors returned by drivers so that users are
// not given access to provider-specific errors.
type wrappedError struct {
	err error
	k   driver.Keeper
}

func wrapError(k driver.Keeper, err error) error {
	if err == nil {
		return nil
	}
	return &wrappedError{k: k, err: err}
}

func (w *wrappedError) Error() string {
	return "secrets: " + w.err.Error()
}
