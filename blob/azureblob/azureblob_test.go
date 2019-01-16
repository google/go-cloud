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
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
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
			// Create concrete type.
			_, err = OpenBucket(ctx, p, test.accountName, test.containerName, nil)
			if (err != nil) != test.wantErr {
				t.Errorf("got err %v want error %v", err, test.wantErr)
			}
		})
	}
}

func TestOpenURL(t *testing.T) {

	const (
		accountName = "my-accoun-name"
		accountKey  = "aGVsbG8=" // must be base64 encoded string, this is "hello"
		sasToken    = "my-sas-token"
	)

	makeCredFile := func(name, content string) *os.File {
		f, err := ioutil.TempFile("", "key")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString(content); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		return f
	}

	keyFile := makeCredFile("key", fmt.Sprintf("{\"AccountName\": %q, \"AccountKey\": %q}", accountName, accountKey))
	defer os.Remove(keyFile.Name())
	badJSONFile := makeCredFile("badjson", "{")
	defer os.Remove(badJSONFile.Name())
	badKeyFile := makeCredFile("badkey", fmt.Sprintf("{\"AccountName\": %q, \"AccountKey\": \"not base 64\"}", accountName))
	defer os.Remove(badKeyFile.Name())
	sasFile := makeCredFile("sas", fmt.Sprintf("{\"AccountName\": %q, \"SASToken\": %q}", accountName, sasToken))
	defer os.Remove(sasFile.Name())

	tests := []struct {
		url      string
		wantName string
		wantErr  bool
		// If we use a key, we should get a non-nil *Credentials in Options.
		wantCreds bool
	}{
		{
			url:       "azblob://mybucket?cred_path=" + keyFile.Name(),
			wantName:  "mybucket",
			wantCreds: true,
		},
		{
			url:       "azblob://mybucket2?cred_path=" + keyFile.Name(),
			wantName:  "mybucket2",
			wantCreds: true,
		},
		{
			url:      "azblob://mybucket?cred_path=" + sasFile.Name(),
			wantName: "mybucket",
		},
		{
			url:      "azblob://mybucket2?cred_path=" + sasFile.Name(),
			wantName: "mybucket2",
		},
		{
			url:     "azblob://foo?cred_path=" + badJSONFile.Name(),
			wantErr: true,
		},
		{
			url:     "azblob://foo?cred_path=" + badKeyFile.Name(),
			wantErr: true,
		},
		{
			url:     "azblob://foo?cred_path=/path/does/not/exist",
			wantErr: true,
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.url, func(t *testing.T) {
			u, err := url.Parse(test.url)
			if err != nil {
				t.Fatal(err)
			}
			got, err := openURL(ctx, u)
			if (err != nil) != test.wantErr {
				t.Errorf("got err %v want error %v", err, test.wantErr)
			}
			if err != nil {
				return
			}
			gotB, ok := got.(*bucket)
			if !ok {
				t.Fatalf("got type %T want *bucket", got)
			}
			if gotB.name != test.wantName {
				t.Errorf("got bucket name %q want %q", gotB.name, test.wantName)
			}
			if gotCreds := (gotB.opts.Credential != nil); gotCreds != test.wantCreds {
				t.Errorf("got creds? %v want %v", gotCreds, test.wantCreds)
			}
		})
	}
}
