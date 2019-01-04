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
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/Azure/azure-pipeline-go/pipeline"
	"github.com/Azure/azure-storage-blob-go/azblob"
	"gocloud.dev/blob"
	"gocloud.dev/blob/driver"
	"gocloud.dev/blob/drivertest"
	"gocloud.dev/internal/testing/setup"
)

// Prerequisites for --record mode
// 1. Sign-in to your Azure Subscription at http://portal.azure.com.
//
// 2. Create a Storage Account.
//
// 3. Locate the Access Key (Primary or Secondary) under your Storage Account > Settings > Access Keys.
//
// 4. Create a file in JSON format with the AccountName and AccountKey values.
//
// Example: settings.json
//  {
//    "AccountName": "enter-your-storage-account-name",
//    "AccountKey": "enter-your-storage-account-key"
//  }
//
// 4. Create a container in your Storage Account > Blob. Update the bucketName
// constant to your container name.
//
// Here is a step-by-step walkthrough using the Azure Portal
// https://docs.microsoft.com/en-us/azure/storage/blobs/storage-quickstart-blobs-portal
//
// 5. Run the tests with -record and -settingsfile flags.

const (
	bucketName  = "go-cloud-bucket"
	accountName = "gocloudblobtests"
)

var pathToSettingsFile = flag.String("settingsfile", "", "path to .json file containing Azure Storage AccountKey and AccountName(required for --record)")

// TestSettings sets the Azure Storage Account name and Key for constructing the test harness.
type TestSettings struct {
	AccountName string
	AccountKey  string
	pipeline    pipeline.Pipeline
}

type harness struct {
	settings   TestSettings
	closer     func()
	httpClient *http.Client
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	s := &TestSettings{}

	if *setup.Record {
		// Fetch the AccountName and AccountKey settings from the setting file.
		if *pathToSettingsFile == "" {
			t.Fatalf("--settingsfile is required in --record mode.")
		}
		b, err := ioutil.ReadFile(*pathToSettingsFile)
		if err != nil {
			t.Fatalf("Couldn't find settings file at %v: %v", *pathToSettingsFile, err)
		}
		err = json.Unmarshal(b, s)
		if err != nil {
			t.Fatalf("Cannot load settings file %v: %v", *pathToSettingsFile, err)
		}
	} else {
		// In replay mode, the AccountName must match the name used for recording.
		s.AccountName = accountName
		s.AccountKey = "FAKE_KEY"
	}

	p, done, httpClient := setup.NewAzureTestPipeline(ctx, t, s.AccountName, s.AccountKey)
	s.pipeline = p

	return &harness{settings: *s, closer: done, httpClient: httpClient}, nil
}

func (h *harness) HTTPClient() *http.Client {
	return h.httpClient
}

func (h *harness) MakeDriver(ctx context.Context) (driver.Bucket, error) {
	serviceURL, _ := ServiceURLFromAccountKey(h.settings.AccountName, h.settings.AccountKey)
	serviceURLForRecorder := serviceURL.WithPipeline(h.settings.pipeline)

	creds, _ := azblob.NewSharedKeyCredential(h.settings.AccountName, h.settings.AccountKey)
	opts := Options{
		Credential: *creds,
	}
	return openBucket(ctx, &serviceURLForRecorder, bucketName, &opts), nil
}

func (h *harness) Close() {
	h.closer()
}

func TestConformance(t *testing.T) {
	// See setup instructions above for more details.
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyContentLanguage{}})
}

const language = "nl"

// verifyContentLanguage uses As to access the underlying Azure types and
// read/write the ContentLanguage field.
type verifyContentLanguage struct{}

func (verifyContentLanguage) Name() string {
	return "verify ContentLanguage can be written and read through As"
}

func (verifyContentLanguage) BucketCheck(b *blob.Bucket) error {
	var u *azblob.ContainerURL
	if !b.As(&u) {
		return errors.New("Bucket.As failed")
	}
	return nil
}

func (verifyContentLanguage) ErrorCheck(err error) error {
	var to azblob.StorageError
	if !blob.ErrorAs(err, &to) {
		return errors.New("Bucket.ErrorAs failed")
	}
	return nil
}

func (verifyContentLanguage) BeforeWrite(as func(interface{}) bool) error {
	var azOpts *azblob.UploadStreamToBlockBlobOptions
	if !as(&azOpts) {
		return errors.New("Writer.As failed")
	}
	azOpts.BlobHTTPHeaders.ContentLanguage = language
	return nil
}

func (verifyContentLanguage) BeforeList(as func(interface{}) bool) error {
	var azOpts *azblob.ListBlobsSegmentOptions
	if !as(&azOpts) {
		return errors.New("BeforeList.As failed")
	}
	return nil
}

func (verifyContentLanguage) AttributesCheck(attrs *blob.Attributes) error {
	var resp azblob.BlobGetPropertiesResponse
	if !attrs.As(&resp) {
		return errors.New("Attributes.As returned false")
	}
	if got := resp.ContentLanguage(); got != language {
		return fmt.Errorf("got %q want %q", got, language)
	}
	return nil
}

func (verifyContentLanguage) ReaderCheck(r *blob.Reader) error {
	var resp azblob.DownloadResponse
	if !r.As(&resp) {
		return errors.New("Reader.As returned false")
	}
	if got := resp.ContentLanguage(); got != language {
		return fmt.Errorf("got %q want %q", got, language)
	}
	return nil
}

func (verifyContentLanguage) ListObjectCheck(o *blob.ListObject) error {
	var item azblob.BlobItem
	if !o.As(&item) {
		return errors.New("ListObject.As returned false")
	}
	if got := *item.Properties.ContentLanguage; got != language {
		return fmt.Errorf("got %q want %q", got, language)
	}
	return nil
}
