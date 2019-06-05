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

package azurekeyvault_test

import (
	"context"
	"log"

	"gocloud.dev/secrets"
	akv "gocloud.dev/secrets/azurekeyvault"
)

func ExampleOpenKeeper() {
	// Get a client to use with the Azure KeyVault API.
	// See API docs for Authentication options.
	// https://github.com/Azure/azure-sdk-for-go
	client, err := akv.Dial()
	if err != nil {
		log.Fatal(err)
	}

	// Construct a *secrets.Keeper.
	keeper, err := akv.OpenKeeper(client, "https://mykeyvaultname.vault.azure.net/keys/mykeyname", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer keeper.Close()

	// Now we can use keeper to encrypt or decrypt.
	ctx := context.Background()
	plaintext := []byte("Hello, Secrets!")
	ciphertext, err := keeper.Encrypt(ctx, plaintext)
	if err != nil {
		log.Fatal(err)
	}
	decrypted, err := keeper.Decrypt(ctx, ciphertext)
	if err != nil {
		log.Fatal(err)
	}
	_ = decrypted
}

func Example_openFromURL() {
	ctx := context.Background()

	// secrets.OpenKeeper creates a *secrets.Keeper from a URL.
	// The "azurekeyvault" URL scheme is replaced with "https" to construct an Azure
	// Key Vault keyID, as described in https://docs.microsoft.com/en-us/azure/key-vault/about-keys-secrets-and-certificates.
	// You can add an optional "/{key-version}" to the path to use a specific
	// version of the key; it defaults to the latest version.
	keeper, err := secrets.OpenKeeper(ctx, "azurekeyvault://mykeyvaultname.vault.azure.net/keys/mykeyname")
	if err != nil {
		log.Fatal(err)
	}
	defer keeper.Close()
}
