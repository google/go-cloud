// Copyright 2019 The Go Cloud Development Kit Authors
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
	"gocloud.dev/secrets"
	"gocloud.dev/secrets/vault"
)

func Example_encrypt() {

	// Get a client to use with the Vault API.
	ctx := context.Background()
	client, err := vault.Dial(ctx, &vault.Config{
		Token: "<Client (Root) Token>",
		APIConfig: api.Config{
			Address: "http://127.0.0.1:8200",
		},
	})

	// Construct a *secrets.Keeper.
	keeper := vault.NewKeeper(client, "my-key", nil)

	// Now we can use keeper to encrypt or decrypt.
	plaintext := []byte("Hello, Secrets!")
	ciphertext, err := keeper.Encrypt(ctx, plaintext)
	if err != nil {
		log.Fatal(err)
	}
	decrypted, err := keeper.Decrypt(ctx, ciphertext)
	_ = decrypted
}

func Example_openKeeper() {
	ctx := context.Background()

	// OpenKeeper creates a *secrets.Keeper from a URL.
	// The host+path are used as the keyID, and the "address" and "token"
	// query parameters specify which Vault server to dial, and the auth token
	// to use.
	k, err := secrets.OpenKeeper(ctx, "vault://MYKEY?address=http://MYVAULT.SERVER.COM:8080&token=MYTOKEN")
	_, _ = k, err
}
