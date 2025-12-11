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

package hashivault

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	"github.com/hashicorp/vault/api"
	"gocloud.dev/internal/testing/setup"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
	"gocloud.dev/runtimevar/drivertest"
)

// To run these tests against a real Vault server, first run ./localvault.sh.
// Then wait a few seconds for the server to be ready.

const (
	apiAddress = "http://127.0.0.1:8200"
	testToken  = "faketoken"
)

type harness struct {
	client *api.Client
}

func (h *harness) MakeWatcher(ctx context.Context, name string, decoder *runtimevar.Decoder) (driver.Watcher, error) {
	return newWatcher(h.client, name, decoder, nil)
}

func (h *harness) CreateVariable(ctx context.Context, name string, val []byte) error {
	var data map[string]any
	if err := json.Unmarshal(val, &data); err != nil {
		data = map[string]any{"value": string(val)}
	}
	_, err := h.client.Logical().Write(path.Join("secret/data", name), map[string]any{
		"data": data,
	})
	return err
}

func (h *harness) UpdateVariable(ctx context.Context, name string, val []byte) error {
	return h.CreateVariable(ctx, name, val)
}

func (h *harness) DeleteVariable(ctx context.Context, name string) error {
	_, err := h.client.Logical().Delete(path.Join("secret/metadata", name))
	return err
}

func (h *harness) Close() {}

func (h *harness) Mutable() bool {
	return true
}

func newHarness(t *testing.T) (drivertest.Harness, error) {
	t.Helper()

	if !setup.HasDockerTestEnvironment() {
		t.Skip("Skipping Vault tests since the Vault server is not available")
	}

	ctx := context.Background()
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

	_, err = c.Sys().Health()
	if err != nil {
		return nil, fmt.Errorf("vault server not healthy (run runtimevar/hashivault/localvault.sh): %v", err)
	}

	return &harness{client: c}, nil
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyAs{}})
}

type verifyAs struct{}

func (verifyAs) Name() string {
	return "verify As"
}

func (verifyAs) SnapshotCheck(s *runtimevar.Snapshot) error {
	var secret *api.Secret
	if !s.As(&secret) {
		return errors.New("Snapshot.As failed for *api.Secret")
	}
	return nil
}

func (verifyAs) ErrorCheck(v *runtimevar.Variable, err error) error {
	var secErr *SecretError
	if !v.ErrorAs(err, &secErr) {
		return errors.New("ErrorAs expected to succeed with *SecretError")
	}
	if secErr.Code != 404 {
		return fmt.Errorf("expected SecretError code 404, got %d", secErr.Code)
	}
	return nil
}

func TestNoConfigError(t *testing.T) {
	if _, err := Dial(context.Background(), nil); err == nil {
		t.Error("got nil, want no auth Config provided")
	}
}

func TestEquivalentError(t *testing.T) {
	err1 := errors.New("error one")
	err2 := errors.New("error one")
	err3 := errors.New("error two")
	respErr404 := &api.ResponseError{StatusCode: 404}
	respErr403 := &api.ResponseError{StatusCode: 403}
	secErr404 := &SecretError{Code: 404, Message: "not found"}
	secErr404b := &SecretError{Code: 404, Message: "also not found"}
	secErr400 := &SecretError{Code: 400, Message: "bad request"}

	tests := []struct {
		Err1, Err2 error
		Want       bool
	}{
		{Err1: err1, Err2: err2, Want: true},
		{Err1: err1, Err2: err3, Want: false},
		{Err1: respErr404, Err2: respErr404, Want: true},
		{Err1: respErr404, Err2: respErr403, Want: false},
		{Err1: err1, Err2: respErr404, Want: false},
		{Err1: secErr404, Err2: secErr404b, Want: true},
		{Err1: secErr404, Err2: secErr400, Want: false},
		{Err1: secErr404, Err2: respErr404, Want: false}, // Different error types
	}

	for _, test := range tests {
		got := equivalentError(test.Err1, test.Err2)
		if got != test.Want {
			t.Errorf("%v vs %v: got %v want %v", test.Err1, test.Err2, got, test.Want)
		}
	}
}

func TestWatcherErrorCode(t *testing.T) {
	ctx := context.Background()
	client, err := Dial(ctx, &Config{
		Token: "fake",
		APIConfig: api.Config{
			Address: "http://localhost:8200",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	w, err := newWatcher(client, "test", runtimevar.StringDecoder, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	codes := []struct {
		Code int
		Want string
	}{
		{400, "InvalidArgument"},
		{403, "PermissionDenied"},
		{404, "NotFound"},
		{429, "ResourceExhausted"},
		{500, "Internal"},
		{502, "Internal"},
		{503, "ResourceExhausted"},
		{999, "Unknown"},
	}

	for _, test := range codes {
		err := &api.ResponseError{StatusCode: test.Code}
		code := w.ErrorCode(err)
		if code.String() != test.Want {
			t.Errorf("api.ResponseError StatusCode %d: got %v, want %v", test.Code, code, test.Want)
		}
	}

	for _, test := range codes {
		err := &SecretError{Code: test.Code, Message: "test"}
		code := w.ErrorCode(err)
		if code.String() != test.Want {
			t.Errorf("SecretError Code %d: got %v, want %v", test.Code, code, test.Want)
		}
	}
}

func TestEngineVersionPaths(t *testing.T) {
	ctx := context.Background()
	client, err := Dial(ctx, &Config{
		Token: "fake",
		APIConfig: api.Config{
			Address: "http://localhost:8200",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		EngineVersion int
		Mount         string
		SecretPath    string
		WantPath      string
	}{
		{2, "secret", "myapp/config", "secret/data/myapp/config"},
		{1, "secret", "myapp/config", "secret/myapp/config"},
		{2, "kv", "myapp/config", "kv/data/myapp/config"},
		{1, "kv", "myapp/config", "kv/myapp/config"},
		{0, "", "test", "secret/data/test"}, // defaults
	}

	for _, test := range tests {
		opts := &Options{
			EngineVersion: test.EngineVersion,
			Mount:         test.Mount,
		}
		w, err := newWatcher(client, test.SecretPath, runtimevar.StringDecoder, opts)
		if err != nil {
			t.Errorf("newWatcher failed: %v", err)
			continue
		}
		watcher := w.(*watcher)
		if watcher.path != test.WantPath {
			t.Errorf("EngineVersion=%d, Mount=%q, SecretPath=%q: got path %q, want %q",
				test.EngineVersion, test.Mount, test.SecretPath, watcher.path, test.WantPath)
		}
		w.Close()
	}
}

func TestInvalidEngineVersion(t *testing.T) {
	ctx := context.Background()
	client, err := Dial(ctx, &Config{
		Token: "fake",
		APIConfig: api.Config{
			Address: "http://localhost:8200",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = newWatcher(client, "test", runtimevar.StringDecoder, &Options{EngineVersion: 3})
	if err == nil {
		t.Error("expected error for invalid engine version")
	}
}

func fakeConnectionStringInEnv() func() {
	oldURLVal := os.Getenv("VAULT_SERVER_URL")
	oldTokenVal := os.Getenv("VAULT_SERVER_TOKEN")
	os.Setenv("VAULT_SERVER_URL", "http://myvaultserver")
	os.Setenv("VAULT_SERVER_TOKEN", "faketoken")
	return func() {
		os.Setenv("VAULT_SERVER_URL", oldURLVal)
		os.Setenv("VAULT_SERVER_TOKEN", oldTokenVal)
	}
}

func alternativeConnectionStringEnvVars() func() {
	oldURLVal := os.Getenv("VAULT_ADDR")
	oldTokenVal := os.Getenv("VAULT_TOKEN")
	os.Setenv("VAULT_ADDR", "http://myalternativevaultserver")
	os.Setenv("VAULT_TOKEN", "faketoken2")
	return func() {
		os.Setenv("VAULT_ADDR", oldURLVal)
		os.Setenv("VAULT_TOKEN", oldTokenVal)
	}
}

func unsetConnectionStringEnvVars() func() {
	oldURLVal := os.Getenv("VAULT_ADDR")
	oldTokenVal := os.Getenv("VAULT_TOKEN")
	oldServerURLVal := os.Getenv("VAULT_SERVER_URL")
	oldServerTokenVal := os.Getenv("VAULT_SERVER_TOKEN")
	os.Unsetenv("VAULT_ADDR")
	os.Unsetenv("VAULT_TOKEN")
	os.Unsetenv("VAULT_SERVER_URL")
	os.Unsetenv("VAULT_SERVER_TOKEN")
	return func() {
		os.Setenv("VAULT_ADDR", oldURLVal)
		os.Setenv("VAULT_SERVER_URL", oldServerURLVal)
		os.Setenv("VAULT_TOKEN", oldTokenVal)
		os.Setenv("VAULT_SERVER_TOKEN", oldServerTokenVal)
	}
}

func TestGetVaultConnectionDetails(t *testing.T) {
	t.Run("Test Current Env Vars", func(t *testing.T) {
		cleanup := fakeConnectionStringInEnv()
		defer cleanup()

		serverURL, err := getVaultURL()
		if err != nil {
			t.Errorf("got unexpected error: %v", err)
		}
		if serverURL != "http://myvaultserver" {
			t.Errorf("expected 'http://myvaultserver': got %q", serverURL)
		}

		vaultToken := getVaultToken()
		if vaultToken != "faketoken" {
			t.Errorf("expected 'faketoken': got %q", vaultToken)
		}
	})

	t.Run("Test Alternative Env Vars", func(t *testing.T) {
		cleanup := alternativeConnectionStringEnvVars()
		defer cleanup()

		serverURL, err := getVaultURL()
		if err != nil {
			t.Errorf("got unexpected error: %v", err)
		}
		if serverURL != "http://myalternativevaultserver" {
			t.Errorf("expected 'http://myalternativevaultserver': got %q", serverURL)
		}

		vaultToken := getVaultToken()
		if vaultToken != "faketoken2" {
			t.Errorf("expected 'faketoken2': got %q", vaultToken)
		}
	})

	t.Run("Test Unset Env Vars Throws Error", func(t *testing.T) {
		cleanup := unsetConnectionStringEnvVars()
		defer cleanup()

		serverURL, err := getVaultURL()
		if err == nil {
			t.Errorf("expected error but got a url: %s", serverURL)
		}

		vaultToken := getVaultToken()
		if vaultToken != "" {
			t.Errorf("expected '': got %q", vaultToken)
		}
	})
}

func TestOpenVariableURL(t *testing.T) {
	cleanup := fakeConnectionStringInEnv()
	defer cleanup()

	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"hashivault://myapp/config", false},
		// OK, setting decoder.
		{"hashivault://myapp/config?decoder=string", false},
		// OK, setting wait.
		{"hashivault://myapp/config?wait=1m", false},
		// OK, setting engine_version.
		{"hashivault://myapp/config?engine_version=1", false},
		{"hashivault://myapp/config?engine_version=2", false},
		// OK, setting mount.
		{"hashivault://myapp/config?mount=kv", false},
		// OK, setting all.
		{"hashivault://myapp/config?decoder=string&wait=1m&engine_version=2&mount=secret", false},
		// Invalid decoder.
		{"hashivault://myapp/config?decoder=notadecoder", true},
		// Invalid wait.
		{"hashivault://myapp/config?wait=xx", true},
		// Invalid engine_version.
		{"hashivault://myapp/config?engine_version=3", true},
		{"hashivault://myapp/config?engine_version=abc", true},
		// Invalid parameter.
		{"hashivault://myapp/config?param=value", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.URL, func(t *testing.T) {
			v, err := runtimevar.OpenVariable(ctx, test.URL)
			if (err != nil) != test.WantErr {
				t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
			}
			if err == nil {
				if err := v.Close(); err != nil {
					t.Errorf("%s: got error during close: %v", test.URL, err)
				}
			}
		})
	}
}
