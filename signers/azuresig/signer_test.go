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

package azuresig

import (
	"context"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/services/keyvault/v7.0/keyvault"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"

	"gocloud.dev/internal/testing/setup"
	"gocloud.dev/signers"
	"gocloud.dev/signers/driver"
	"gocloud.dev/signers/drivertest"
	"gocloud.dev/signers/internal"
)

// Prerequisites for --record mode
//
// 1. Sign-in to your Azure Subscription at http://portal.azure.com.
//
// 2. Create a KeyVault, see
// https://docs.microsoft.com/en-us/azure/key-vault/quick-create-portal.
//
// 3. Choose an authentication model. This test uses Service Principal, see
// https://docs.microsoft.com/en-us/rest/api/azure/index#register-your-client-application-with-azure-ad.
// For documentation on acceptable auth models, see
// https://docs.microsoft.com/en-us/azure/key-vault/key-vault-whatis.
//
// 4. Set your environment variables depending on the auth model selection.
// Modify helper initEnv() as needed.
// For Service Principal, please set the following, see
// https://docs.microsoft.com/en-us/go/azure/azure-sdk-go-authorization.
//
// - AZURE_TENANT_ID: Go to "Azure Active Directory", then "Properties". The
//     "Directory ID" property is your AZURE_TENANT_ID.
// - AZURE_CLIENT_ID: Go to "Azure Active Directory", then "App Registrations",
//     then "View all applications". The "Application ID" column shows your
//     AZURE_CLIENT_ID.
// - AZURE_CLIENT_SECRET: Click on the application from the previous step,
//     then "Settings" and then "Keys". Create a key and use it as your
//     AZURE_CLIENT_SECRET. Make sure to save the value as it's hidden after
//     the initial creation.
// - AZURE_ENVIRONMENT: (optional).
// - AZURE_AD_RESOURCE: (optional).
//
// 5. Create/Import a Key. This can be done in the Azure Portal under "Key vaults".
//
// 6. Update constants below to match your Azure KeyVault settings.

const (
	keyID1 = "https://gcloud.vault.azure.net/keys/rick"
	keyID2 = "https://gcloud.vault.azure.net/keys/morty"
)

type harness struct {
	client *keyvault.BaseClient
	close  func()
}

func (h *harness) MakeDriver(_ context.Context) (driver.Signer, driver.Signer, internal.Digester, error) {
	keeper1, err := openSigner(h.client, keyID1, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	keeper2, err := openSigner(h.client, keyID2, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	return keeper1, keeper2, &internal.SHA512{}, nil
}

func (h *harness) Close() {
	h.close()
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	// Use initEnv to setup your environment variables.
	var client *keyvault.BaseClient
	if *setup.Record {
		initEnv()
		var err error
		client, err = Dial()
		if err != nil {
			return nil, err
		}
	} else {
		// Use a null authorizer for replay mode.
		c := keyvault.NewWithoutDefaults()
		client = &c
		client.Authorizer = &autorest.NullAuthorizer{}
	}
	sender, done := setup.NewAzureKeyVaultTestClient(ctx, t)
	client.Sender = sender
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

	if os.Getenv("AZURE_TENANT_ID") == "" ||
		os.Getenv("AZURE_CLIENT_ID") == "" ||
		os.Getenv("AZURE_CLIENT_SECRET") == "" {
		log.Fatal("Missing environment for recording tests, set AZURE_TENANT_ID, AZURE_CLIENT_ID and AZURE_CLIENT_SECRET")
	}

	os.Setenv("AZURE_ENVIRONMENT", env.Name)

	vaultEndpoint := strings.TrimSuffix(env.KeyVaultEndpoint, "/")
	os.Setenv("AZURE_AD_RESOURCE", vaultEndpoint)
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness)
}

// Key Vault-specific tests.

func TestNoConnectionError(t *testing.T) {
	client := keyvault.NewWithoutDefaults()
	k, err := OpenSigner(&client, keyID1, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	if _, err := k.Sign(context.Background(), []byte("secrets")); err == nil {
		t.Error("Sign: got nil, want no connection error")
	}
}

// This test no longer works on MacOS, as OpenKeeper always fails with "MSI not available".

func TestOpenKeeper(t *testing.T) {
	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"azuresig://mykeyvault.vault.azure.net/keys/mykey/myversion", false},
		// No version -> OK.
		{"azuresig://mykeyvault.vault.azure.net/keys/mykey", false},
		// Setting algorithm query param -> OK.
		{"azuresig://mykeyvault.vault.azure.net/keys/mykey/myversion?algorithm=RSA-OAEP", false},
		// Invalid query parameter.
		{"azuresig://mykeyvault.vault.azure.net/keys/mykey/myversion?param=value", true},
		// Missing key vault name.
		{"azuresig:///vault.azure.net/keys/mykey/myversion", true},
		// Missing "keys".
		{"azuresig://mykeyvault.vault.azure.net/mykey/myversion", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		keeper, err := signers.OpenSigner(ctx, test.URL)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
		if err == nil {
			if err = keeper.Close(); err != nil {
				t.Errorf("%s: got error during close: %v", test.URL, err)
			}
		}
	}
}

func TestKeyIDRE(t *testing.T) {
	client := keyvault.NewWithoutDefaults()

	testCases := []struct {
		// input
		keyID string

		// output
		keyVaultURI string
		keyName     string
		keyVersion  string
	}{
		{
			keyID:       keyID1,
			keyVaultURI: "https://gcloud.vault.azure.net/",
			keyName:     "rick",
		},
		{
			keyID:       keyID2,
			keyVaultURI: "https://gcloud.vault.azure.net/",
			keyName:     "morty",
		},
		{
			keyID:       "https://mykeyvault.vault.azure.net/keys/mykey/myversion",
			keyVaultURI: "https://mykeyvault.vault.azure.net/",
			keyName:     "mykey",
			keyVersion:  "myversion",
		},
		{
			keyID:       "https://mykeyvault.vault.usgovcloudapi.net/keys/mykey/myversion",
			keyVaultURI: "https://mykeyvault.vault.usgovcloudapi.net/",
			keyName:     "mykey",
			keyVersion:  "myversion",
		},
		{
			keyID:       "https://mykeyvault.vault.region01.external.com/keys/mykey/myversion",
			keyVaultURI: "https://mykeyvault.vault.region01.external.com/",
			keyName:     "mykey",
			keyVersion:  "myversion",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.keyID, func(t *testing.T) {
			k, err := openSigner(&client, testCase.keyID, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer k.Close()

			if k.keyVaultURI != testCase.keyVaultURI {
				t.Errorf("got key vault URI %s, want key vault URI %s", k.keyVaultURI, testCase.keyVaultURI)
			}

			if k.keyName != testCase.keyName {
				t.Errorf("got key name %s, want key name %s", k.keyName, testCase.keyName)
			}

			if k.keyVersion != testCase.keyVersion {
				t.Errorf("got key version %s, want key version %s", k.keyVersion, testCase.keyVersion)
			}
		})
	}
}
