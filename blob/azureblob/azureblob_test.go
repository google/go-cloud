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

package azureblob

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
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
// 4. Set the environment variables AZURE_STORAGE_ACCOUNT, AZURE_STORAGE_KEY to
//    the storage account name and your access key.
//
// 5. Create a container in your Storage Account > Blob. Update the bucketName
// constant to your container name.
//
// Here is a step-by-step walkthrough using the Azure Portal
// https://docs.microsoft.com/en-us/azure/storage/blobs/storage-quickstart-blobs-portal
//
// 5. Run the tests with -record.

const (
	bucketName  = "go-cloud-bucket"
	accountName = AccountName("gocloudblobtests")
)

type harness struct {
	pipeline   pipeline.Pipeline
	credential *azblob.SharedKeyCredential
	closer     func()
	httpClient *http.Client
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	var key AccountKey
	if *setup.Record {
		name, err := DefaultAccountName()
		if err != nil {
			t.Fatal(err)
		}
		if name != accountName {
			t.Fatalf("Please update the accountName constant to match your settings file so future records work (%q vs %q)", name, accountName)
		}
		key, err = DefaultAccountKey()
		if err != nil {
			t.Fatal(err)
		}
	} else {
		// In replay mode, we use fake credentials.
		key = AccountKey(base64.StdEncoding.EncodeToString([]byte("FAKECREDS")))
	}
	credential, err := NewCredential(accountName, key)
	if err != nil {
		return nil, err
	}
	p, done, httpClient := setup.NewAzureTestPipeline(ctx, t, "blob", credential, string(accountName))
	return &harness{pipeline: p, credential: credential, closer: done, httpClient: httpClient}, nil
}

func (h *harness) HTTPClient() *http.Client {
	return h.httpClient
}

func (h *harness) MakeDriver(ctx context.Context) (driver.Bucket, error) {
	return openBucket(ctx, h.pipeline, accountName, bucketName, &Options{Credential: h.credential})
}

func (h *harness) Close() {
	h.closer()
}

func TestConformance(t *testing.T) {
	// See setup instructions above for more details.
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyContentLanguage{}})
}

func BenchmarkAzureblob(b *testing.B) {
	name, err := DefaultAccountName()
	if err != nil {
		b.Fatal(err)
	}
	key, err := DefaultAccountKey()
	if err != nil {
		b.Fatal(err)
	}
	credential, err := NewCredential(name, key)
	if err != nil {
		b.Fatal(err)
	}
	p := NewPipeline(credential, azblob.PipelineOptions{})
	bkt, err := OpenBucket(context.Background(), p, name, bucketName, nil)
	if err != nil {
		b.Fatal(err)
	}
	drivertest.RunBenchmarks(b, bkt)
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

func (verifyContentLanguage) ErrorCheck(b *blob.Bucket, err error) error {
	var to azblob.StorageError
	if !b.ErrorAs(err, &to) {
		return errors.New("Bucket.ErrorAs failed")
	}
	return nil
}

func (verifyContentLanguage) BeforeRead(as func(interface{}) bool) error {
	var u *azblob.BlockBlobURL
	if !as(&u) {
		return fmt.Errorf("BeforeRead As failed to get %T", u)
	}
	var ac *azblob.BlobAccessConditions
	if !as(&ac) {
		return fmt.Errorf("BeforeRead As failed to get %T", ac)
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

func (verifyContentLanguage) BeforeCopy(as func(interface{}) bool) error {
	var md azblob.Metadata
	if !as(&md) {
		return errors.New("BeforeCopy.As failed for Metadata")
	}

	var mac *azblob.ModifiedAccessConditions
	if !as(&mac) {
		return errors.New("BeforeCopy.As failed for ModifiedAccessConditions")
	}

	var bac *azblob.BlobAccessConditions
	if !as(&bac) {
		return errors.New("BeforeCopy.As failed for BlobAccessConditions")
	}
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
	if o.IsDir {
		var prefix azblob.BlobPrefix
		if !o.As(&prefix) {
			return errors.New("ListObject.As for directory returned false")
		}
		return nil
	}
	var item azblob.BlobItem
	if !o.As(&item) {
		return errors.New("ListObject.As for object returned false")
	}
	if got := *item.Properties.ContentLanguage; got != language {
		return fmt.Errorf("got %q want %q", got, language)
	}
	return nil
}

func TestOpenBucket(t *testing.T) {
	tests := []struct {
		description   string
		nilPipeline   bool
		accountName   AccountName
		containerName string
		want          string
		wantErr       bool
	}{
		{
			description:   "nil pipeline results in error",
			nilPipeline:   true,
			accountName:   "myaccount",
			containerName: "foo",
			wantErr:       true,
		},
		{
			description:   "empty account name results in error",
			containerName: "foo",
			wantErr:       true,
		},
		{
			description: "empty container name results in error",
			accountName: "myaccount",
			wantErr:     true,
		},
		{
			description:   "success",
			accountName:   "myaccount",
			containerName: "foo",
			want:          "foo",
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			var p pipeline.Pipeline
			if !test.nilPipeline {
				p = NewPipeline(azblob.NewAnonymousCredential(), azblob.PipelineOptions{})
			}
			// Create driver impl.
			drv, err := openBucket(ctx, p, test.accountName, test.containerName, nil)
			if (err != nil) != test.wantErr {
				t.Errorf("got err %v want error %v", err, test.wantErr)
			}
			if err == nil && drv != nil && drv.name != test.want {
				t.Errorf("got %q want %q", drv.name, test.want)
			}
			// Create portable type.
			_, err = OpenBucket(ctx, p, test.accountName, test.containerName, nil)
			if (err != nil) != test.wantErr {
				t.Errorf("got err %v want error %v", err, test.wantErr)
			}
		})
	}
}

func TestOpenerFromEnv(t *testing.T) {
	tests := []struct {
		name        string
		accountName AccountName
		accountKey  AccountKey
		sasToken    SASToken

		wantSharedCreds bool
		wantSASToken    SASToken
	}{
		{
			name:            "AccountKey",
			accountName:     "myaccount",
			accountKey:      AccountKey(base64.StdEncoding.EncodeToString([]byte("FAKECREDS"))),
			wantSharedCreds: true,
		},
		{
			name:            "SASToken",
			accountName:     "myaccount",
			sasToken:        "borkborkbork",
			wantSharedCreds: false,
			wantSASToken:    "borkborkbork",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			o, err := openerFromEnv(test.accountName, test.accountKey, test.sasToken)
			if err != nil {
				t.Fatal(err)
			}
			if o.AccountName != test.accountName {
				t.Errorf("AccountName = %q; want %q", o.AccountName, test.accountName)
			}
			if o.Pipeline == nil {
				t.Error("Pipeline = <nil>; want non-nil")
			}
			if o.Options.Credential == nil {
				if test.wantSharedCreds {
					t.Error("Options.Credential = <nil>; want non-nil")
				}
			} else {
				if !test.wantSharedCreds {
					t.Errorf("Options.Credential = %#v; want <nil>", o.Options.Credential)
				}
				if got := AccountName(o.Options.Credential.AccountName()); got != test.accountName {
					t.Errorf("Options.Credential.AccountName() = %q; want %q", got, test.accountName)
				}
			}
			if o.Options.SASToken != test.wantSASToken {
				t.Errorf("Options.SASToken = %q; want %q", o.Options.SASToken, test.wantSASToken)
			}
		})
	}
}

func TestOpenBucketFromURL(t *testing.T) {
	prevAccount := os.Getenv("AZURE_STORAGE_ACCOUNT")
	prevKey := os.Getenv("AZURE_STORAGE_KEY")
	os.Setenv("AZURE_STORAGE_ACCOUNT", "my-account")
	os.Setenv("AZURE_STORAGE_KEY", "bXlrZXk=") // mykey base64 encoded
	defer func() {
		os.Setenv("AZURE_STORAGE_ACCOUNT", prevAccount)
		os.Setenv("AZURE_STORAGE_KEY", prevKey)
	}()

	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"azblob://mybucket", false},
		// Invalid parameter.
		{"azblob://mybucket?param=value", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		_, err := blob.OpenBucket(ctx, test.URL)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
	}
}
