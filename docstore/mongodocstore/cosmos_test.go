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

package mongodocstore

import (
	"context"
	"os"
	"testing"

	"gocloud.dev/docstore/drivertest"
	"gocloud.dev/internal/testing/setup"
)

// Run conformance tests on Azure Cosmos.

// See https://docs.microsoft.com/en-us/azure/cosmos-db/connect-mongodb-account
// on how to get a MongoDB connection string for Azure Cosmos.
var cosmosConnString = os.Getenv("COSMOS_CONNECTION_STRING")

func TestConformanceCosmos(t *testing.T) {
	if !*setup.Record {
		t.Skip("replaying is not yet supported for Azure Cosmos")
	}
	if cosmosConnString == "" {
		t.Fatal("test harness requires COSMOS_CONNECTION_STRING environment variable to run")
	}

	ctx := context.Background()
	client, err := Dial(ctx, cosmosConnString)
	if err != nil {
		t.Fatalf("dialing to %s: %v", cosmosConnString, err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		t.Fatalf("connecting to %s: %v", cosmosConnString, err)
	}
	defer func() {
		// Cleanup any resource to avoid wastes.
		client.Database(dbName).Drop(ctx)
		client.Disconnect(ctx)
	}()

	newHarness := func(context.Context, *testing.T) (drivertest.Harness, error) {
		return &harness{client.Database(dbName), true}, nil
	}
	drivertest.RunConformanceTests(t, newHarness, codecTester{}, []drivertest.AsTest{verifyAs{}})
}
