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

package azureblob

import (
	"context"
	"net/http"
	"os"
	"testing"

	"gocloud.dev/blob/driver"
	"gocloud.dev/blob/drivertest"
	"gocloud.dev/internal/testing/setup"
)

// Prerequisites for --record mode
// 1. Sign-in to your Azure Subscription and create a Storage Account
//    link to the Azure Portal: https://portal.azure.com
// 2. Locate the Access Key (Primary or Secondary) under your Storage Account > Settings > Access Keys
// 3. Set Environment Variable AZURE_STORAGE_ACCOUNT_NAME and AZURE_STORAGE_ACCOUNT_KEY below
// 4. Create a container in your Storage Account > Blob. Use the bucketName constant below as the container name.
// Here is a step-by-step walkthrough using the Azure Portal
// https://docs.microsoft.com/en-us/azure/storage/blobs/storage-quickstart-blobs-portal

const (
	bucketName = "go-cloud-bucket"
)

type harness struct {
	settings Settings
	closer   func()
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	p, done, accountName, accountKey := setup.NewAzureTestPipeline(ctx, t)
	s := &Settings{
		AccountName: accountName,
		AccountKey:  accountKey,
		SASToken:    "",
		Pipeline:    p,
	}

	return &harness{settings: *s, closer: done}, nil
}

func (h *harness) HTTPClient() *http.Client {
	return setup.AzureHTTPClient(nil)
}

func (h *harness) MakeDriver(ctx context.Context) (driver.Bucket, error) {
	return openBucketWithAccountKey(ctx, &h.settings, bucketName)
}

func (h *harness) Close() {
	h.closer()
}

func TestConformance(t *testing.T) {
	// See setup instructions above for more details.
	// AZURE_STORAGE_ACCOUNT_NAME is the Azure Storage Account Name
	os.Setenv("AZURE_STORAGE_ACCOUNT_NAME", "gocloud")
	// AZURE_STORAGE_ACCOUNT_KEY is the Primary or Secondary Key
	os.Setenv("AZURE_STORAGE_ACCOUNT_KEY", "")

	drivertest.RunConformanceTests(t, newHarness, nil)
}
