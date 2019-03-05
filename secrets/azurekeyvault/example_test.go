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

	"github.com/Azure/azure-sdk-for-go/services/keyvault/v7.0/keyvault"
	akv "gocloud.dev/secrets/azurekeyvault"
)

func Example() {
	// Get a client to use with the Azure KeyVault API.
	// See API docs for Authentication options.
	// https://github.com/Azure/azure-sdk-for-go
	client, err := akv.Dial()
	if err != nil {
		log.Fatal(err)
	}

	// Construct a *secrets.Keeper.
	// List of Parameters:
	// - client: *keyvault.BaseClient instance, see https://godoc.org/github.com/Azure/azure-sdk-for-go/services/keyvault/v7.0/keyvault#BaseClient
	// - keyVaultName: string representing the KeyVault name, see https://docs.microsoft.com/en-us/azure/key-vault/common-parameters-and-headers
	// - keyName: string representing the keyName, see https://docs.microsoft.com/en-us/rest/api/keyvault/encrypt/encrypt#uri-parameters
	// - keyVersion: string representing the keyVersion, see https://docs.microsoft.com/en-us/rest/api/keyvault/encrypt/encrypt#uri-parameters
	// - opts: *KeeperOptions with the desired Algorithm to use for operations. See this link for more info: https://docs.microsoft.com/en-us/rest/api/keyvault/encrypt/encrypt#jsonwebkeyencryptionalgorithm
	keeper := akv.NewKeeper(
		client,
		"replace with keyVaultName",
		"replace with keyName",
		"replace with keyVersion",
		&akv.KeeperOptions{
			Algorithm: string(keyvault.RSAOAEP256),
		},
	)

	// Now we can use keeper to encrypt or decrypt.
	ctx := context.Background()
	plaintext := []byte("Hello, Secrets!")
	ciphertext, err := keeper.Encrypt(ctx, plaintext)
	if err != nil {
		log.Fatal(err)
	}
	decrypted, err := keeper.Decrypt(ctx, ciphertext)
	_ = decrypted
}
