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

package hashivault_test

import (
	"context"
	"log"

	"github.com/hashicorp/vault/api"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/hashivault"
)

func ExampleOpenVariable() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// Get a client to use with the Vault API.
	client, err := hashivault.Dial(ctx, &hashivault.Config{
		Token: "CLIENT_TOKEN",
		APIConfig: api.Config{
			Address: "http://127.0.0.1:8200",
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	// Construct a *runtimevar.Variable that watches the secret.
	v, err := hashivault.OpenVariable(client, "myapp/config", runtimevar.StringDecoder, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer v.Close()
}

func Example_openVariableFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/runtimevar/hashivault"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// runtimevar.OpenVariable creates a *runtimevar.Variable from a URL.
	// The default opener connects to a Vault server based on the environment
	// variables VAULT_SERVER_URL/VAULT_ADDR and VAULT_SERVER_TOKEN/VAULT_TOKEN.
	v, err := runtimevar.OpenVariable(ctx, "hashivault://myapp/config?decoder=string")
	if err != nil {
		log.Fatal(err)
	}
	defer v.Close()
}
