// Copyright 2018 The Go Cloud Authors
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
	"testing"

	cloudkms "cloud.google.com/go/kms/apiv1"
	"gocloud.dev/internal/secrets/driver"
	"gocloud.dev/internal/secrets/drivertest"
	"gocloud.dev/internal/testing/setup"
	"google.golang.org/api/option"
)

const (
	// These constants capture values that were used during the last --record.
	// If you want to use --record mode,
	// 1. Update projectID to your GCP project name (not number!)
	// 2. Enable the Cloud KMS API.
	// 3. Create a key ring and a key, change their name below accordingly.
	projectID = "pledged-solved-practically"
	location  = "global"
	keyRing   = "test"
	keyID     = "password"
)

type harness struct {
	client *cloudkms.KeyManagementClient
	close  func()
}

func (h *harness) MakeDriver(ctx context.Context) (driver.Crypter, error) {
	return &Crypter{
		keyID: &KeyID{
			ProjectID: projectID,
			Location:  location,
			KeyRing:   keyRing,
			KeyID:     keyID,
		},
		client: h.client,
	}, nil
}

func (h *harness) Close() {
	h.close()
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
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
	drivertest.RunConformanceTests(t, newHarness)
}
