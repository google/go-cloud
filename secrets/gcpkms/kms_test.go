// Copyright 2018 The Go Cloud Development Kit Authors
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

package gcpkms

import (
	"context"
	"errors"
	"testing"

	cloudkms "cloud.google.com/go/kms/apiv1"
	"gocloud.dev/internal/testing/setup"
	"gocloud.dev/secrets"
	"gocloud.dev/secrets/driver"
	"gocloud.dev/secrets/drivertest"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"
	"google.golang.org/grpc/status"
)

// These constants capture values that were used during the last --record.
// If you want to use --record mode,
// 1. Update projectID to your GCP project name (not number!)
// 2. Enable the Cloud KMS API.
// 3. Create a key ring and a key, change their name below accordingly.
const (
	project  = "go-cloud-test-216917"
	location = "global"
	keyRing  = "test"
	keyID1   = "password"
	keyID2   = "password2"
)

type harness struct {
	client *cloudkms.KeyManagementClient
	close  func()
}

func (h *harness) MakeDriver(ctx context.Context) (driver.Keeper, driver.Keeper, error) {
	return &keeper{KeyResourceID(project, location, keyRing, keyID1), h.client, KeeperOptions{}},
		&keeper{KeyResourceID(project, location, keyRing, keyID2), h.client, KeeperOptions{}}, nil
}

func (h *harness) Close() {
	h.close()
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	t.Helper()

	conn, done := setup.NewGCPgRPCConn(ctx, t, endPoint, "secrets")
	client, err := cloudkms.NewKeyManagementClient(ctx, option.WithGRPCConn(conn))
	if err != nil {
		return nil, err
	}
	return &harness{
		client: client,
		close: func() {
			client.Close()
			done()
		},
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
	var s *status.Status
	if !k.ErrorAs(err, &s) {
		return errors.New("Keeper.ErrorAs failed")
	}
	return nil
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

	keeper := OpenKeeper(client, "", nil)
	defer keeper.Close()

	if _, err := keeper.Encrypt(ctx, []byte("test")); err == nil {
		t.Error("got nil, want rpc error")
	}
}

func TestAdditionalAuthenticatedData(t *testing.T) {
	ctx := context.Background()
	conn, done := setup.NewGCPgRPCConn(ctx, t, endPoint, "secrets")
	defer done()
	client, err := cloudkms.NewKeyManagementClient(ctx, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	opts := KeeperOptions{
		AdditionalAuthenticatedData: []byte("sample AAD"),
	}
	k1a := OpenKeeper(client, KeyResourceID(project, location, keyRing, keyID1), &opts)
	defer k1a.Close()
	k1b := OpenKeeper(client, KeyResourceID(project, location, keyRing, keyID1), &opts)
	defer k1b.Close()
	opts.AdditionalAuthenticatedData = []byte("different AAD")
	k2 := OpenKeeper(client, KeyResourceID(project, location, keyRing, keyID1), &opts)
	defer k2.Close()

	// Encrypt/Decrypt with an AAD should work.
	secret := []byte("a secret")
	encryptedSecret, err := k1a.Encrypt(ctx, secret)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}
	if got, err := k1b.Decrypt(ctx, encryptedSecret); err != nil {
		t.Fatalf("failed to decrypt: %v", err)
	} else if string(got) != string(secret) {
		t.Errorf("unexpected decrypt result: %v", string(got))
	}

	// Decrypting with a different AAD should fail.
	if _, err := k2.Decrypt(ctx, encryptedSecret); err == nil {
		t.Errorf("expected Decrypt with a different AAD to fail, but it did not")
	}
}

func TestOpenKeeper(t *testing.T) {
	cleanup := setup.FakeGCPDefaultCredentials(t)
	defer cleanup()

	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"gcpkms://projects/MYPROJECT/locations/MYLOCATION/keyRings/MYKEYRING/cryptoKeys/MYKEY", false},
		// Invalid additional_authenticated_data.
		{"gcpkms://projects/MYPROJECT/locations/MYLOCATION/keyRings/MYKEYRING/cryptoKeys/MYKEY?additional_authenticated_data=22", true},
		// OK, with valid additional_authenticated_data.
		{"gcpkms://projects/MYPROJECT/locations/MYLOCATION/keyRings/MYKEYRING/cryptoKeys/MYKEY?additional_authenticated_data=SGVsbG8sIOS4lueVjA==", false},
		// Invalid query parameter.
		{"gcpkms://projects/MYPROJECT/locations/MYLOCATION/keyRings/MYKEYRING/cryptoKeys/MYKEY?param=val", true},
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
