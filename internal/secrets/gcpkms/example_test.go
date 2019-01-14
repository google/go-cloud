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

	// Get a client to use with the KMS API.
	client, done, err := gcpkms.Dial(ctx, nil)
	if err != nil {
		panic(err)
	}
	// Make sure to close the connection when done.
	defer done()

	plaintext := []byte("Hello, Secrets!")

	crypter := gcpkms.NewCrypter(
		client,
		// Get the key resource ID.
		// See https://cloud.google.com/kms/docs/object-hierarchy#key for more
		// information.
		&gcpkms.KeyID{
			ProjectID: "project-id",
			Location:  "global",
			KeyRing:   "test",
			Key:       "key-name",
		},
		nil,
	)

	// Makes the request to the KMS API to encrypt the plain text into a binary.
	encrypted, err := crypter.Encrypt(ctx, plaintext)
	if err != nil {
		panic(err)
	}
	// Store the encrypted secret.
	_ = encrypted
}

func ExampleCrypter_Decrypt() {
	ctx := context.Background()

	// Get a client to use with the KMS API.
	client, done, err := gcpkms.Dial(ctx, nil)
	if err != nil {
		panic(err)
	}
	// Make sure to close the connection when done.
	defer done()

	// Get the secret to be decrypted from some kind of storage.
	var ciphertext []byte

	crypter := gcpkms.NewCrypter(
		client,
		// Get the key resource ID.
		// See https://cloud.google.com/kms/docs/object-hierarchy#key for more
		// information.
		&gcpkms.KeyID{
			ProjectID: "project-id",
			Location:  "global",
			KeyRing:   "test",
			Key:       "key-name",
		},
		nil,
	)

	// Makes the request to the KMS API to decrypt the binary into plain text.
	decrypted, err := crypter.Decrypt(ctx, ciphertext)
	if err != nil {
		panic(err)
	}
	// Use the decrypted secret.
	_ = decrypted
}
