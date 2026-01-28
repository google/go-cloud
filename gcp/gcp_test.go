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

package gcp_test

import (
	"context"
	"testing"

	"gocloud.dev/gcp"
	"gocloud.dev/internal/testing/setup"
	"golang.org/x/oauth2/google"
)

func TestNewHTTPClient(t *testing.T) {
	transport := gcp.DefaultTransport()
	_, err := gcp.NewHTTPClient(transport, nil)
	if err == nil {
		t.Error("got nil want error")
	}
	creds, err := setup.FakeGCPCredentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = gcp.NewHTTPClient(transport, gcp.CredentialsTokenSource(creds))
	if err != nil {
		t.Error(err)
	}
}

func TestCredentialsTokenSource(t *testing.T) {
	ts := gcp.CredentialsTokenSource(nil)
	if ts != nil {
		t.Error("got non-nil TokenSource from nil creds, want nil")
	}
	creds, err := setup.FakeGCPCredentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ts = gcp.CredentialsTokenSource(creds)
	if ts == nil {
		t.Error("got nil TokenSource from creds, want non-nil")
	}
}

func TestDefaultProjectID(t *testing.T) {
	_, err := gcp.DefaultProjectID(nil)
	if err == nil {
		t.Error("got nil error from nil creds, want error")
	}
	creds, err := setup.FakeGCPCredentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = gcp.DefaultProjectID(creds)
	if err != nil {
		t.Error(err)
	}
}

func TestDefaultCredentialsWithParams(t *testing.T) {
	cleanup := setup.FakeGCPDefaultCredentials(t)
	defer cleanup()

	ctx := context.Background()

	// Test with empty params (should use default scope and default universe domain)
	creds, err := gcp.DefaultCredentialsWithParams(ctx, google.CredentialsParams{})
	if err != nil {
		t.Fatalf("DefaultCredentialsWithParams with empty params failed: %v", err)
	}
	if creds == nil {
		t.Error("got nil credentials, want non-nil")
	}
	// Verify default universe domain (googleapis.com)
	gotDomain, err := creds.GetUniverseDomain()
	if err != nil {
		t.Fatalf("GetUniverseDomain failed: %v", err)
	}
	if gotDomain != "googleapis.com" {
		t.Errorf("got default universe domain %q, want %q", gotDomain, "googleapis.com")
	}

	// Test with universe domain parameter.
	// Note: The fake "authorized_user" credentials may not support universe domain
	// overrides, but we can verify the function accepts the parameter without error.
	creds, err = gcp.DefaultCredentialsWithParams(ctx, google.CredentialsParams{
		UniverseDomain: "example.com",
	})
	if err != nil {
		t.Fatalf("DefaultCredentialsWithParams with universe domain failed: %v", err)
	}
	if creds == nil {
		t.Error("got nil credentials, want non-nil")
	}
	// The universe domain may not be set for authorized_user credential type,
	// but the function should not error.
	_, err = creds.GetUniverseDomain()
	if err != nil {
		t.Fatalf("GetUniverseDomain failed: %v", err)
	}

	// Test with custom scopes
	creds, err = gcp.DefaultCredentialsWithParams(ctx, google.CredentialsParams{
		Scopes: []string{"https://www.googleapis.com/auth/devstorage.read_only"},
	})
	if err != nil {
		t.Fatalf("DefaultCredentialsWithParams with custom scopes failed: %v", err)
	}
	if creds == nil {
		t.Error("got nil credentials, want non-nil")
	}
}
