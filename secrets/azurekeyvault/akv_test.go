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

package azurekeyvault

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/services/keyvault/v7.0/keyvault"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"gocloud.dev/internal/testing/setup"
	"gocloud.dev/secrets"
	"gocloud.dev/secrets/driver"
	"gocloud.dev/secrets/drivertest"
)

// Prerequisites for --record mode
//
// 1. Sign-in to your Azure Subscription at http://portal.azure.com.
//
// 2. Create a KeyVault, see https://docs.microsoft.com/en-us/azure/key-vault/quick-create-portal.
//
// 3. Choose an authentication model. This test uses Service Principal, see https://docs.microsoft.com/en-us/rest/api/azure/index#register-your-client-application-with-azure-ad.
// For documentation on acceptable auth models, see https://docs.microsoft.com/en-us/azure/key-vault/key-vault-whatis.
//
// 4. Set your environment variables depending on the auth model selection. Modify helper initEnv() as needed.
// For Service Principal, please set the following, see https://docs.microsoft.com/en-us/go/azure/azure-sdk-go-authorization.
// - AZURE_TENANT_ID
// - AZURE_CLIENT_ID
// - AZURE_CLIENT_SECRET
// - AZURE_ENVIRONMENT
// - AZURE_AD_RESOURCE to https://vault.azure.net
//
// 5. Create/Import a Key. This can be done in the Azure Portal or by code.
//
// 6. Update constants below to match your Azure KeyVault settings.

const (
	keyVaultName = "go-cloud"
	keyID1       = "test1"
	keyID2       = "test2"
	// Important: an empty key version will default to 'Current Version' in Azure Key Vault.
	// See link below for more information on versioning
	// https://docs.microsoft.com/en-us/azure/key-vault/about-keys-secrets-and-certificates
	keyVersion = ""
	algorithm  = string(keyvault.RSAOAEP256)
)

type harness struct {
	client *keyvault.BaseClient
	close  func()
}

func (h *harness) MakeDriver(ctx context.Context) (driver.Keeper, driver.Keeper, error) {
	keeper1 := keeper{
		client:       h.client,
		keyVaultName: keyVaultName,
		keyName:      keyID1,
		keyVersion:   keyVersion,
		options:      &KeeperOptions{Algorithm: algorithm},
	}

	keeper2 := keeper{
		client:       h.client,
		keyVaultName: keyVaultName,
		keyName:      keyID2,
		keyVersion:   keyVersion,
		options:      &KeeperOptions{Algorithm: algorithm},
	}

	return &keeper1, &keeper2, nil
}

func (h *harness) Close() {
	h.close()
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	// Use initEnv to setup your environment variables.
	if *setup.Record {
		initEnv()
	}

	done, sender := setup.NewAzureKeyVaultTestClient(ctx, t)
	client, err := Dial()
	if err != nil {
		return nil, err
	}
	client.Sender = sender

	// Use a null authorizer for replay mode.
	if !*setup.Record {
		na := &autorest.NullAuthorizer{}
		client.Authorizer = na
	}

	return &harness{
		client: client,
		close:  done,
	}, nil
}

func initEnv() {
	env, err := azure.EnvironmentFromName("AZUREPUBLICCLOUD")
	if err != nil {
		log.Fatalln(err)
	}

	// For Client Credentials authorization, set AZURE_TENANT_ID, AZURE_CLIENT_ID, AZURE_CLIENT_SECRET
	// For Client Certificate and Azure Managed Service Identity, see doc below for help
	// https://github.com/Azure/azure-sdk-for-go

	// os.Setenv("AZURE_TENANT_ID", "Specifies the Tenant to which to authenticate")
	// os.Setenv("AZURE_CLIENT_ID", "Specifies the app client ID to use")
	// os.Setenv("AZURE_CLIENT_SECRET", "Specifies the app secret to use")

	os.Setenv("AZURE_ENVIRONMENT", env.Name)

	vaultEndpoint := strings.TrimSuffix(env.KeyVaultEndpoint, "/")
	os.Setenv("AZURE_AD_RESOURCE", vaultEndpoint)
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyAs{}})
}

type verifyAs struct{}

func (v verifyAs) Name() string {
	return "verify As function"
}

func (v verifyAs) ErrorCheck(k *secrets.Keeper, err error) error {
	var e autorest.DetailedError
	if !k.ErrorAs(err, &e) {
		return errors.New("Keeper.ErrorAs failed")
	}
	return nil
}
