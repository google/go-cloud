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

package gcpsig

import (
	"context"
	"testing"

	kms "cloud.google.com/go/kms/apiv1"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"

	"gocloud.dev/internal/testing/setup"
	"gocloud.dev/signers"
	"gocloud.dev/signers/driver"
	"gocloud.dev/signers/drivertest"
	"gocloud.dev/signers/internal"
)

// These constants capture values that were used during the last --record.
// If you want to use --record mode,
// 1. Update projectID to your GCP project name (not number!)
// 2. Enable the Cloud KMS API.
// 3. Create a key ring and a key, change their name below accordingly.
const (
	project     = "go-cloud-test-317923"
	location    = "global"
	keyRing     = "test"
	RsaSha256a  = "RsaSha256a"
	RsaSha256b  = "RsaSha256b"
	RsaSha512a  = "RsaSha512a"
	RsaSha512b  = "RsaSha512b"
	EcSha256a   = "EcSha256a"
	EcSha256b   = "EcSha256b"
	EcSha384a   = "EcSha384a"
	EcSha384b   = "EcSha384b"
	keyVersion1 = "1"
	keyVersion2 = "1"
)

type harness struct {
	keyA     string
	keyB     string
	digester internal.Digester
	client   *kms.KeyManagementClient
	close    func()
}

func (h *harness) MakeDriver(_ context.Context) (driver.Signer, driver.Signer, internal.Digester, error) {
	return &signer{nil, h.digester, KeyResourceID(project, location, keyRing, h.keyA, keyVersion1), h.client},
		&signer{nil, h.digester, KeyResourceID(project, location, keyRing, h.keyB, keyVersion2), h.client},
		h.digester,
		nil
}

func (h *harness) Close() {
	h.close()
}

func generateHarness(keyA string, keyB string, digester internal.Digester) drivertest.HarnessMaker {
	return func(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
		conn, done := setup.NewGCPgRPCConn(ctx, t, endPoint, "signers")
		client, err := kms.NewKeyManagementClient(ctx, option.WithGRPCConn(conn))
		if err != nil {
			return nil, err
		}
		return &harness{
			keyA:     keyA,
			keyB:     keyB,
			digester: digester,
			client:   client,
			close: func() {
				client.Close()
				done()
			},
		}, nil
	}
}

func TestConformance(t *testing.T) {
	t.Run("RsaSha256", func(t *testing.T) {
		drivertest.RunConformanceTests(t, generateHarness(RsaSha256a, RsaSha256b, &internal.SHA256{}))
	})
	t.Run("RsaSha512", func(t *testing.T) {
		drivertest.RunConformanceTests(t, generateHarness(RsaSha512a, RsaSha512b, &internal.SHA512{}))
	})
	t.Run("EcSha256", func(t *testing.T) {
		drivertest.RunConformanceTests(t, generateHarness(EcSha256a, EcSha256b, &internal.SHA256{}))
	})
	t.Run("EcSha384", func(t *testing.T) {
		drivertest.RunConformanceTests(t, generateHarness(EcSha384a, EcSha384b, &internal.SHA384{}))
	})
}

// KMS-specific tests.

func TestNoConnectionError(t *testing.T) {
	ctx := context.Background()
	client, done, err := Dial(ctx, oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: "fake",
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer done()

	signer, err := OpenSigner(client, "SHA384", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer signer.Close()

	if _, err := signer.Sign(ctx, []byte("test")); err == nil {
		t.Error("got nil, want rpc error")
	}
}

func TestOpenSigner(t *testing.T) {
	cleanup := setup.FakeGCPDefaultCredentials(t)
	defer cleanup()

	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"gcpsig://SHA256/projects/MYPROJECT/locations/MYLOCATION/keyRings/MYKEYRING/cryptoKeys/MYKEY/cryptoKeyVersions/1", false},
		// Invalid query parameter.
		{"gcpsig://SHA256/projects/MYPROJECT/locations/MYLOCATION/keyRings/MYKEYRING/cryptoKeys/MYKEY/cryptoKeyVersions/1?param=val", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		signer, err := signers.OpenSigner(ctx, test.URL)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
		if err == nil {
			if err = signer.Close(); err != nil {
				t.Errorf("%s: got error during close: %v", test.URL, err)
			}
		}
	}
}
