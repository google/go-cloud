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
	"encoding/json"
	"flag"
	"io/ioutil"
	"net/http"
	"testing"

	"gocloud.dev/blob/driver"
	"gocloud.dev/blob/drivertest"
	"gocloud.dev/internal/testing/setup"
)

// Prerequisites for --record mode
// 1. Sign-in to your Azure Subscription and create a Storage Account
//    Link to the Azure Portal: https://portal.azure.com
//
// 2. Locate the Access Key (Primary or Secondary) under your Storage Account > Settings > Access Keys
//
// 3. Create a file in JSON format with the AccountName and AccountKey values
// Example: settings.json
//	{
//		"AccountName": "enter-your-storage-account-name",
//		"AccountKey": "enter-your-storage-account-key",
//	}
//
// 4. Create a container in your Storage Account > Blob. Update the bucketName constant to your container name.
// Here is a step-by-step walkthrough using the Azure Portal
// https://docs.microsoft.com/en-us/azure/storage/blobs/storage-quickstart-blobs-portal
//
// 5. Run the tests with -record and -settingsfile flags
// Example:
// go.exe test -timeout 30s gocloud.dev/blob/azureblob -run ^TestConformance$ -v -record -settingsfile <path-to-settings.json>

const (
	bucketName = "go-cloud-bucket"
)

var pathToSettingsFile = flag.String("settingsfile", "", "path to .json file containing Azure Storage AccountKey and AccountName(required for --record)")

type harness struct {
	settings   Settings
	closer     func()
	httpClient *http.Client
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	s := &Settings{}

	if *setup.Record {
		// Fetch the AccountName and AccountKey settings from file
		if *pathToSettingsFile == "" {
			t.Fatalf("--settingsfile is required in --record mode.")
		} else {
			b, err := ioutil.ReadFile(*pathToSettingsFile)
			if err != nil {
				t.Fatalf("Couldn't find settings file at %v: %v", *pathToSettingsFile, err)
			}
			json.Unmarshal(b, s)
		}
	} else {
		// In replay mode, the AccountName must match the name used for recording.
		s.AccountName = "gocloud"
		s.AccountKey = "FAKE_KEY"
	}

	p, done, httpClient := setup.NewAzureTestPipeline(ctx, t, s.AccountName, s.AccountKey)
	s.Pipeline = p

	return &harness{settings: *s, closer: done, httpClient: httpClient}, nil
}

func (h *harness) HTTPClient() *http.Client {
	return h.httpClient
}

func (h *harness) MakeDriver(ctx context.Context) (driver.Bucket, error) {
	return openBucketWithAccountKey(ctx, &h.settings, bucketName)
}

func (h *harness) Close() {
	h.closer()
}

func TestConformance(t *testing.T) {
	// See setup instructions above for more details.
	drivertest.RunConformanceTests(t, newHarness, nil)
}
