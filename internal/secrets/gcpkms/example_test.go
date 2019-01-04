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

package gcpkms_test

import (
	"context"

	"gocloud.dev/internal/secrets/gcpkms"
)

func ExampleCrypter_Encrypt() {
	ctx := context.Background()
	client, done, err := gcpkms.Dial(ctx, nil)
	if err != nil {
		panic(err)
	}
	defer done()
	plaintext := []byte("Hello, Secrets!")

	crypter := gcpkms.NewCrypter(
		client,
		&gcpkms.KeyID{
			ProjectID: "pledged-solved-practically",
			Location:  "global",
			KeyRing:   "test",
			Key:       "password",
		},
	)
	encrypted, err := crypter.Encrypt(ctx, plaintext)
	if err != nil {
		panic(err)
	}
	// Store the encrypted secret.
	_ = encrypted
}

func ExampleCrypter_Decrypt() {
	ctx := context.Background()
	client, done, err := gcpkms.Dial(ctx, nil)
	if err != nil {
		panic(err)
	}
	defer done()

	// Get the secret to be decrypted from some kind of storage.
	var ciphertext []byte

	crypter := gcpkms.NewCrypter(
		client,
		&gcpkms.KeyID{
			ProjectID: "pledged-solved-practically",
			Location:  "global",
			KeyRing:   "test",
			Key:       "password",
		},
	)
	decrypted, err := crypter.Decrypt(ctx, ciphertext)
	if err != nil {
		panic(err)
	}
	// Use the decrypted secret.
	_ = decrypted
}
