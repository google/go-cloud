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

package vault_test

import (
	"context"
	"log"

	"github.com/hashicorp/vault/api"
	"gocloud.dev/secrets/vault"
)

func Example_encrypt() {
	ctx := context.Background()

	// Get a client to use with the Vault API.
	client, err := vault.Dial(ctx, &vault.Config{
		Token: "<Client (Root) Token>",
		APIConfig: &api.Config{
			Address: "http://127.0.0.1:8200",
		},
	})

	// Get the plaintext to be encrypted.
	plaintext := []byte("Hello, Secrets!")

	keeper := vault.NewKeeper(
		client,
		"my-key",
		nil,
	)

	// Makes the request to the Vault transit API to encrypt the plain text into a binary.
	encrypted, err := keeper.Encrypt(ctx, plaintext)
	if err != nil {
		log.Fatal(err)
	}
	// Store the encrypted secret.
	_ = encrypted
}

func Example_decrypt() {
	ctx := context.Background()

	// Get a client to use with the Vault API.
	client, err := vault.Dial(ctx, &vault.Config{
		Token: "<Client (Root) Token>",
		APIConfig: &api.Config{
			Address: "http://127.0.0.1:8200",
		},
	})

	// Get the secret to be decrypted from some kind of storage.
	ciphertext := []byte("vault:v1:RCWHyq64IWZo1s1YhCdjbe5DweJ9UNHKl0TK6NhpaGvv0tXs0QNpTuf8Sg==")

	keeper := vault.NewKeeper(
		client,
		"my-key",
		nil,
	)
	decrypted, err := keeper.Decrypt(ctx, ciphertext)
	if err != nil {
		log.Fatal(err)
	}
	// Use the decrypted secret.
	_ = decrypted
}
