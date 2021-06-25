// Copyright 2021 The Go Cloud Development Kit Authors
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

package azuresig_test

import (
	"context"
	"log"

	"gocloud.dev/signers"
	"gocloud.dev/signers/azuresig"
)

func ExampleOpenSigner() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.

	// Get a client to use with the Azure KeyVault API, using default
	// authorization from the environment.
	//
	// You can alternatively use DialUsingCLIAuth to use auth from the
	// "az" CLI.
	client, err := azuresig.Dial()
	if err != nil {
		log.Fatal(err)
	}

	// Construct a *signers.Signer.
	signer, err := azuresig.OpenSigner(client, "https://mykeyvaultname.vault.azure.net/keys/mykeyname", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer signer.Close()
}

func Example_openFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/signers/azuresig"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// The "azuresig" URL scheme is replaced with "https" to construct an Azure
	// Key Vault keyID, as described in https://docs.microsoft.com/en-us/azure/key-vault/about-keys-signers-and-certificates.
	// You can add an optional "/{key-version}" to the path to use a specific
	// version of the key; it defaults to the latest version.
	signer, err := signers.OpenSigner(ctx, "azuresig://mykeyvaultname.vault.azure.net/keys/mykeyname")
	if err != nil {
		log.Fatal(err)
	}
	defer signer.Close()
}
