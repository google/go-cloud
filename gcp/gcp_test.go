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
