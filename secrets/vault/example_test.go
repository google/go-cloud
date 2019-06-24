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

func ExampleOpenKeeper() {
	// This example is used in https://gocloud.dev/howto/secrets/open-keeper/#vault-ctor

	// import _ "gocloud.dev/secrets/vault"

	// Variables set up elsewhere:
	ctx := context.Background()

	// Get a client to use with the Vault API.
	client, err := vault.Dial(ctx, &vault.Config{
		Token: "CLIENT_TOKEN",
		APIConfig: api.Config{
			Address: "http://127.0.0.1:8200",
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	// Construct a *secrets.Keeper.
	keeper := vault.OpenKeeper(client, "my-key", nil)
	defer keeper.Close()
}

func Example_openFromURL() {
	// This example is used in https://gocloud.dev/howto/secrets/open-keeper/#vault

	// import _ "gocloud.dev/secrets/vault"

	// Variables set up elsewhere:
	ctx := context.Background()

	keeper, err := secrets.OpenKeeper(ctx, "vault://mykey")
	if err != nil {
		log.Fatal(err)
	}
	defer keeper.Close()
}
