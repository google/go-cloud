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
// limtations under the License.

package vault

import (
	"context"
	"os"
	"testing"

	"github.com/hashicorp/vault/api"
	"gocloud.dev/secrets/driver"
	"gocloud.dev/secrets/drivertest"
)

// These constants cpature values that were used druing the last --record.
// If you want to use --record mode,
// 1. Install Vault. (https://learn.hashicorp.com/vault/getting-started/install)
// 2. Run a dev Vault server: `vault server -dev`.
// 3. Set API address: `export VAULT_ADDR='http://127.0.0.1:8200'`
// 4. Copy the "Root Token" displayed and run:
// `export VAULT_DEV_ROOT_TOKEN_ID=<Root Token>`.
// 5. Enable the transit API: `vault secrets enable transit`.
// 6. Create a key ring: `vault write -f transit/keys/my-key`
const (
	keyID      = "my-key"
	apiAddress = "http://127.0.0.1:8200"
)

type harness struct {
	client *api.Client
	close  func()
}

func (h *harness) MakeDriver(ctx context.Context) (driver.Keeper, error) {
	return &keeper{
		keyID:  keyID,
		client: h.client,
	}, nil
}

func (h *harness) Close() {}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	c, err := api.NewClient(&api.Config{
		Address: apiAddress,
	})
	if err != nil {
		return nil, err
	}
	c.SetToken(os.Getenv("VAULT_DEV_ROOT_TOKEN_ID"))
	return &harness{
		client: c,
	}, nil
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness)
}
