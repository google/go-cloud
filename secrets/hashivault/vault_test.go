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
// limtations under the License.

package hashivault

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/vault/api"
	"gocloud.dev/internal/testing/setup"
	"gocloud.dev/secrets"
	"gocloud.dev/secrets/driver"
	"gocloud.dev/secrets/drivertest"
)

// To run these tests against a real Vault server, first run ./localvault.sh.
// Then wait a few seconds for the server to be ready.

const (
	keyID1     = "test-secrets"
	keyID2     = "test-secrets2"
	apiAddress = "http://127.0.0.1:8200"
	testToken  = "faketoken"
)

// enableTransit checks and makes sure the Transit API is enabled only once.
var enableTransit sync.Once

type harness struct {
	client *api.Client
	close  func()
}

func (h *harness) MakeDriver(ctx context.Context) (driver.Keeper, driver.Keeper, error) {
	return newKeeper(h.client, keyID1, nil), newKeeper(h.client, keyID2, nil), nil
}

func (h *harness) Close() {}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	if !setup.HasDockerTestEnvironment() {
		t.Skip("Skipping Vault tests since the Vault server is not available")
	}
	c, err := Dial(ctx, &Config{
		Token: testToken,
		APIConfig: api.Config{
			Address: apiAddress,
		},
	})
	if err != nil {
		return nil, err
	}
	c.SetClientTimeout(3 * time.Second)
	// Enable the Transit Secrets Engine to use Vault as an Encryption as a Service.
	enableTransit.Do(func() {
		s, err := c.Logical().Read("sys/mounts")
		if err != nil {
			t.Fatal(err, "; run secrets/hashivault/localvault.sh to start a dev vault container")
		}
		if _, ok := s.Data["transit/"]; !ok {
			if _, err := c.Logical().Write("sys/mounts/transit", map[string]any{"type": "transit"}); err != nil {
				t.Fatal(err, "; run secrets/hashivault/localvault.sh to start a dev vault container")
			}
		}
	})

	return &harness{
		client: c,
	}, nil
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyAs{}})
}

type verifyAs struct{}

func (v verifyAs) Name() string {
	return "verify As function"
}

func (v verifyAs) ErrorCheck(k *secrets.Keeper, err error) error {
	var s string
	if k.ErrorAs(err, &s) {
		return errors.New("Keeper.ErrorAs expected to fail")
	}
	return nil
}

// Vault-specific tests.

func TestNoSessionProvidedError(t *testing.T) {
	if _, err := Dial(context.Background(), nil); err == nil {
		t.Error("got nil, want no auth Config provided")
	}
}

func TestNoConnectionError(t *testing.T) {
	ctx := context.Background()

	// Dial calls vault's NewClient method, which doesn't make the connection. Try
	// doing encryption which should fail by no connection.
	client, err := Dial(ctx, &Config{
		Token: "<Client (Root) Token>",
		APIConfig: api.Config{
			Address: apiAddress,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	keeper := OpenKeeper(client, "my-key", nil)
	defer keeper.Close()
	if _, err := keeper.Encrypt(ctx, []byte("test")); err == nil {
		t.Error("got nil, want connection refused")
	}
}

func TestOpenKeeper(t *testing.T) {
	t.Setenv("VAULT_SERVER_URL", "http://myvaultserver")
	t.Setenv("VAULT_SERVER_TOKEN", "faketoken")

	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"hashivault://mykey", false},
		// OK, setting engine.
		{"hashivault://mykey?engine=foo", false},
		// Invalid parameter.
		{"hashivault://mykey?param=value", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		keeper, err := secrets.OpenKeeper(ctx, test.URL)
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

func TestGetVaultConnectionDetails(t *testing.T) {
	t.Run("Test Current Env Vars", func(t *testing.T) {
		t.Setenv("VAULT_SERVER_URL", "http://myvaultserver")
		t.Setenv("VAULT_SERVER_TOKEN", "faketoken")

		serverUrl, err := getVaultURL()
		if err != nil {
			t.Errorf("got unexpected error: %v", err)
		}
		if serverUrl != "http://myvaultserver" {
			t.Errorf("expected 'http://myvaultserver': got %q", serverUrl)
		}

		vaultToken := getVaultToken()
		if vaultToken != "faketoken" {
			t.Errorf("export 'faketoken': got %q", vaultToken)
		}
	})

	t.Run("Test Alternative Env Vars", func(t *testing.T) {
		t.Setenv("VAULT_ADDR", "http://myalternativevaultserver")
		t.Setenv("VAULT_TOKEN", "faketoken2")

		serverUrl, err := getVaultURL()
		if err != nil {
			t.Errorf("got unexpected error: %v", err)
		}
		if serverUrl != "http://myalternativevaultserver" {
			t.Errorf("export '': got %q", serverUrl)
		}

		vaultToken := getVaultToken()
		if vaultToken != "faketoken2" {
			t.Errorf("export 'faketoken2': got %q", vaultToken)
		}
	})
	t.Run("Test Unset Env Vars Throws Error", func(t *testing.T) {
		t.Setenv("VAULT_ADDR", "")
		t.Setenv("VAULT_TOKEN", "")
		t.Setenv("VAULT_SERVER_URL", "")
		t.Setenv("VAULT_SERVER_TOKEN", "")

		serverUrl, err := getVaultURL()
		if err == nil {
			t.Errorf("expected error but got a url: %s", serverUrl)
		}

		vaultToken := getVaultToken()
		if vaultToken != "" {
			t.Errorf("export '': got %q", vaultToken)
		}
	})
}

func TestVaultPath(t *testing.T) {
	tests := []struct {
		prefix, keyID, want string
		wantErr             bool
	}{
		{"transit/decrypt", "mykey", "transit/decrypt/mykey", false},
		{"transit/decrypt", "tenant-a/subkey", "transit/decrypt/tenant-a/subkey", false},
		{"transit/decrypt", "tenant-a/../tenant-b", "transit/decrypt/tenant-b", false},
		{"transit/decrypt", "../../sys/health", "", true},
		{"transit/decrypt", "a/../../../auth/token/create", "", true},
		{"transit/decrypt", "..", "", true},
		{"transit/decrypt", "../decrypt-sibling", "", true},
	}
	for _, tc := range tests {
		got, err := vaultPath(tc.prefix, tc.keyID)
		if (err != nil) != tc.wantErr {
			t.Errorf("vaultPath(%q, %q) error = %v, wantErr %v", tc.prefix, tc.keyID, err, tc.wantErr)
			continue
		}
		if err == nil && got != tc.want {
			t.Errorf("vaultPath(%q, %q) = %q, want %q", tc.prefix, tc.keyID, got, tc.want)
		}
	}
}

func TestDecryptRejectsKeyIDThatEscapesEnginePrefix(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":{"plaintext":"aGVsbG8="}}`)
	}))
	defer srv.Close()

	client, err := api.NewClient(&api.Config{Address: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	// A legitimate keyID reaches the expected path.
	gotPath = ""
	k := newKeeper(client, "tenant-a-key", nil)
	if _, err := k.Decrypt(context.Background(), []byte("Y2lwaGVydGV4dA==")); err != nil {
		t.Fatalf("Decrypt with a normal keyID: %v", err)
	}
	if want := "/v1/transit/decrypt/tenant-a-key"; gotPath != want {
		t.Errorf("request path = %q, want %q", gotPath, want)
	}

	// A keyID crafted to escape the transit/decrypt prefix must be rejected
	// before any request is sent, not merely fail server-side.
	for _, keyID := range []string{"../../sys/health", "a/../../../auth/token/create"} {
		gotPath = ""
		k := newKeeper(client, keyID, nil)
		if _, err := k.Decrypt(context.Background(), []byte("Y2lwaGVydGV4dA==")); err == nil {
			t.Errorf("Decrypt with keyID %q: got no error, want rejection", keyID)
		}
		if gotPath != "" {
			t.Errorf("Decrypt with keyID %q: request reached the server at path %q; want no request sent", keyID, gotPath)
		}
	}
}

func TestEncryptRejectsKeyIDThatEscapesEnginePrefix(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":{"ciphertext":"vault:v1:abc"}}`)
	}))
	defer srv.Close()

	client, err := api.NewClient(&api.Config{Address: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	// A legitimate keyID reaches the expected path.
	gotPath = ""
	k := newKeeper(client, "tenant-a-key", nil)
	if _, err := k.Encrypt(context.Background(), []byte("hello")); err != nil {
		t.Fatalf("Encrypt with a normal keyID: %v", err)
	}
	if want := "/v1/transit/encrypt/tenant-a-key"; gotPath != want {
		t.Errorf("request path = %q, want %q", gotPath, want)
	}

	// A keyID crafted to escape the transit/encrypt prefix must be rejected
	// before any request is sent, not merely fail server-side.
	for _, keyID := range []string{"../../sys/health", "a/../../../auth/token/create"} {
		gotPath = ""
		k := newKeeper(client, keyID, nil)
		if _, err := k.Encrypt(context.Background(), []byte("hello")); err == nil {
			t.Errorf("Encrypt with keyID %q: got no error, want rejection", keyID)
		}
		if gotPath != "" {
			t.Errorf("Encrypt with keyID %q: request reached the server at path %q; want no request sent", keyID, gotPath)
		}
	}
}

func TestOpenKeeperRejectsKeyIDThatEscapesEnginePrefix(t *testing.T) {
	// End-to-end through secrets.OpenKeeper and the URL opener, not just the
	// driver.Keeper methods directly, to confirm a URL-supplied keyID is
	// covered too (e.g. "hashivault://../../sys/health").
	t.Setenv("VAULT_SERVER_URL", "http://myvaultserver")
	t.Setenv("VAULT_SERVER_TOKEN", "faketoken")

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":{"plaintext":"aGVsbG8="}}`)
	}))
	defer srv.Close()

	client, err := api.NewClient(&api.Config{Address: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	keeper := OpenKeeper(client, "../../sys/health", nil)
	defer keeper.Close()
	if _, err := keeper.Decrypt(context.Background(), []byte("Y2lwaGVydGV4dA==")); err == nil {
		t.Error("Decrypt via a keyID that escapes the engine prefix: got no error, want rejection")
	}
	if gotPath != "" {
		t.Errorf("request reached the server at path %q; want no request sent", gotPath)
	}
}
