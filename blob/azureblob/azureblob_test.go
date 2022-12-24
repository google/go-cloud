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
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	azblobblob "github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/blob"
	"gocloud.dev/blob/driver"
	"gocloud.dev/blob/drivertest"
	"gocloud.dev/internal/testing/setup"
)

// Prerequisites for -record mode
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
	accountName = "gocloudblobtests"
)

type harness struct {
	clientFn   func(bucketName string) (*container.Client, error)
	closer     func()
	httpClient *http.Client
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	var key string
	if *setup.Record {
		name := os.Getenv("AZURE_STORAGE_ACCOUNT")
		if name != accountName {
			t.Fatalf("Please update the accountName constant to match your settings file so future records work (%q vs %q)", name, accountName)
		}
		key = os.Getenv("AZURE_STORAGE_KEY")
	} else {
		// In replay mode, we use fake credentials.
		key = base64.StdEncoding.EncodeToString([]byte("FAKECREDS"))
	}
	credential, err := azblob.NewSharedKeyCredential(accountName, key)
	if err != nil {
		return nil, err
	}
	httpClient, done := setup.NewAzureTestBlobClient(ctx, t)
	// Hack to work around the fact that SignedURLs for PUTs are not fully
	// portable; they require a "x-ms-blob-type" header. Intercept all
	// requests, and insert that header where needed.
	httpClient.Transport = &requestInterceptor{httpClient.Transport}
	clientOptions := container.ClientOptions{}
	clientOptions.Transport = httpClient
	serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net", accountName)
	clientFn := func(bucketName string) (*container.Client, error) {
		return container.NewClientWithSharedKeyCredential(serviceURL+"/"+bucketName, credential, &clientOptions)
	}
	return &harness{clientFn: clientFn, closer: done, httpClient: httpClient}, nil
}

// requestInterceptor implements a hack for the lack of portability for
// SignedURLs for PUT. It adds the required "x-ms-blob-type" header where
// Azure requires it.
type requestInterceptor struct {
	base http.RoundTripper
}

func (ri *requestInterceptor) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == http.MethodPut && strings.Contains(req.URL.Path, "blob-for-signing") {
		reqClone := *req
		reqClone.Header.Add("x-ms-blob-type", "BlockBlob")
		req = &reqClone
	}
	return ri.base.RoundTrip(req)
}

func (h *harness) HTTPClient() *http.Client {
	return h.httpClient
}

func (h *harness) MakeDriver(ctx context.Context) (driver.Bucket, error) {
	client, err := h.clientFn(bucketName)
	if err != nil {
		return nil, err
	}
	return openBucket(ctx, client, nil)
}

func (h *harness) MakeDriverForNonexistentBucket(ctx context.Context) (driver.Bucket, error) {
	client, err := h.clientFn("bucket-does-not-exist")
	if err != nil {
		return nil, err
	}
	return openBucket(ctx, client, nil)
}

func (h *harness) Close() {
	h.closer()
}

func TestConformance(t *testing.T) {
	// See setup instructions above for more details.
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyContentLanguage{}})
}

func BenchmarkAzureblob(b *testing.B) {
	name := os.Getenv("AZURE_STORAGE_ACCOUNT")
	key := os.Getenv("AZURE_STORAGE_KEY")
	credential, err := azblob.NewSharedKeyCredential(name, key)
	if err != nil {
		b.Fatal(err)
	}
	containerURL := fmt.Sprintf("https://%s.blob.core.windows.net/%s", accountName, bucketName)
	client, err := container.NewClientWithSharedKeyCredential(containerURL, credential, nil)
	if err != nil {
		b.Fatal(err)
	}
	bkt, err := OpenBucket(context.Background(), client, nil)
	if err != nil {
		b.Fatal(err)
	}
	drivertest.RunBenchmarks(b, bkt)
}

var language = "nl"

// verifyContentLanguage uses As to access the underlying Azure types and
// read/write the ContentLanguage field.
type verifyContentLanguage struct{}

func (verifyContentLanguage) Name() string {
	return "verify ContentLanguage can be written and read through As"
}

func (verifyContentLanguage) BucketCheck(b *blob.Bucket) error {
	var u *container.Client
	if !b.As(&u) {
		return errors.New("Bucket.As failed")
	}
	return nil
}

func (verifyContentLanguage) ErrorCheck(b *blob.Bucket, err error) error {
	return nil
}

func (verifyContentLanguage) BeforeRead(as func(interface{}) bool) error {
	var u *azblob.DownloadStreamOptions
	if !as(&u) {
		return fmt.Errorf("BeforeRead As failed to get %T", u)
	}
	return nil
}

func (verifyContentLanguage) BeforeWrite(as func(interface{}) bool) error {
	var azOpts *azblob.UploadStreamOptions
	if !as(&azOpts) {
		return errors.New("Writer.As failed")
	}
	azOpts.HTTPHeaders.BlobContentLanguage = &language
	return nil
}

func (verifyContentLanguage) BeforeCopy(as func(interface{}) bool) error {
	var co *azblobblob.StartCopyFromURLOptions
	if !as(&co) {
		return errors.New("BeforeCopy.As failed")
	}
	return nil
}

func (verifyContentLanguage) BeforeList(as func(interface{}) bool) error {
	var azOpts *container.ListBlobsHierarchyOptions
	if !as(&azOpts) {
		return errors.New("BeforeList.As failed")
	}
	return nil
}

func (verifyContentLanguage) BeforeSign(as func(interface{}) bool) error {
	var azOpts *sas.BlobPermissions
	if !as(&azOpts) {
		return errors.New("BeforeSign.As failed")
	}
	return nil
}

func (verifyContentLanguage) AttributesCheck(attrs *blob.Attributes) error {
	var resp azblobblob.GetPropertiesResponse
	if !attrs.As(&resp) {
		return errors.New("Attributes.As returned false")
	}
	if got := *resp.ContentLanguage; got != language {
		return fmt.Errorf("got %q want %q", got, language)
	}
	return nil
}

func (verifyContentLanguage) ReaderCheck(r *blob.Reader) error {
	var resp azblobblob.DownloadStreamResponse
	if !r.As(&resp) {
		return errors.New("Reader.As returned false")
	}
	if got := *resp.ContentLanguage; got != language {
		return fmt.Errorf("got %q want %q", got, language)
	}
	return nil
}

func (verifyContentLanguage) ListObjectCheck(o *blob.ListObject) error {
	if o.IsDir {
		var prefix container.BlobPrefix
		if !o.As(&prefix) {
			return errors.New("ListObject.As for dir returned false")
		}
		return nil
	}
	var item container.BlobItem
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
		description string
		nilClient   bool
		accountName string
		want        string
		wantErr     bool
	}{
		{
			description: "nil client results in error",
			nilClient:   true,
			accountName: "myaccount",
			wantErr:     true,
		},
		{
			description: "success",
			accountName: "myaccount",
			want:        "foo",
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			var client *container.Client
			var err error
			if !test.nilClient {
				client, err = container.NewClientWithNoCredential("", nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			// Create portable type.
			b, err := OpenBucket(ctx, client, nil)
			if b != nil {
				defer b.Close()
			}
			if (err != nil) != test.wantErr {
				t.Errorf("got err %v want error %v", err, test.wantErr)
			}
		})
	}
}

func TestOpenerFromEnv(t *testing.T) {
	tests := []struct {
		accountName      string
		accountKey       string
		sasToken         string
		connectionString string
		domain           string
		protocol         string
		isCDN            bool
		isLocalEmulator  bool

		want     *credInfoT
		wantOpts *ServiceURLOptions
	}{
		{
			// Shared key.
			accountName: "myaccount",
			accountKey:  "fakecreds",
			want: &credInfoT{
				CredType:    credTypeSharedKey,
				AccountName: "myaccount",
				AccountKey:  "fakecreds",
			},
			wantOpts: &ServiceURLOptions{
				AccountName: "myaccount",
			},
		},
		{
			// SAS Token.
			accountName: "myaccount",
			sasToken:    "a-sas-token",
			want: &credInfoT{
				CredType:    credTypeSASViaNone,
				AccountName: "myaccount",
			},
			wantOpts: &ServiceURLOptions{
				AccountName: "myaccount",
				SASToken:    "a-sas-token",
			},
		},
		{
			// Connection string.
			accountName:      "myaccount",
			connectionString: "a-connection-string",
			want: &credInfoT{
				CredType:         credTypeConnectionString,
				AccountName:      "myaccount",
				ConnectionString: "a-connection-string",
			},
			wantOpts: &ServiceURLOptions{
				AccountName: "myaccount",
			},
		},
		{
			// Default.
			accountName: "anotheraccount",
			want: &credInfoT{
				CredType:    credTypeDefault,
				AccountName: "anotheraccount",
			},
			wantOpts: &ServiceURLOptions{
				AccountName: "anotheraccount",
			},
		},
		{
			// Setting protocol and domain.
			accountName: "myaccount",
			protocol:    "http",
			domain:      "foo.bar.com",
			want: &credInfoT{
				CredType:    credTypeDefault,
				AccountName: "myaccount",
			},
			wantOpts: &ServiceURLOptions{
				AccountName:   "myaccount",
				Protocol:      "http",
				StorageDomain: "foo.bar.com",
			},
		},
		{
			// Local emulator.
			accountName:     "myaccount",
			isLocalEmulator: true,
			want: &credInfoT{
				CredType:    credTypeDefault,
				AccountName: "myaccount",
			},
			wantOpts: &ServiceURLOptions{
				AccountName:     "myaccount",
				IsLocalEmulator: true,
			},
		},
	}
	for _, test := range tests {
		os.Setenv("AZURE_STORAGE_ACCOUNT", test.accountName)
		os.Setenv("AZURE_STORAGE_KEY", test.accountKey)
		os.Setenv("AZURE_STORAGE_SAS_TOKEN", test.sasToken)
		os.Setenv("AZURE_STORAGE_CONNECTION_STRING", test.connectionString)
		os.Setenv("AZURE_STORAGE_DOMAIN", test.domain)
		os.Setenv("AZURE_STORAGE_PROTOCOL", test.protocol)
		if test.isCDN {
			os.Setenv("AZURE_STORAGE_IS_CDN", "true")
		} else {
			os.Setenv("AZURE_STORAGE_IS_CDN", "")
		}
		if test.isLocalEmulator {
			os.Setenv("AZURE_STORAGE_IS_LOCAL_EMULATOR", "true")
		} else {
			os.Setenv("AZURE_STORAGE_IS_LOCAL_EMULATOR", "")
		}

		got := newCredInfoFromEnv()
		if diff := cmp.Diff(got, test.want); diff != "" {
			t.Errorf("unexpected diff in credInfo: %s", diff)
		}
		gotOpts := NewDefaultServiceURLOptions()
		if diff := cmp.Diff(gotOpts, test.wantOpts); diff != "" {
			t.Errorf("unexpected diff in Options: %s", diff)
		}

	}
}

func TestNewServiceURL(t *testing.T) {
	tests := []struct {
		opts             ServiceURLOptions
		query            url.Values
		want             ServiceURL
		wantErrOverrides bool
		wantErrURL       bool
	}{
		{
			// Unknown query parameter.
			opts: ServiceURLOptions{
				AccountName: "myaccount",
			},
			query: url.Values{
				"foo": {"bar"},
			},
			wantErrOverrides: true,
		},
		{
			// Duplicate query parameter.
			opts: ServiceURLOptions{
				AccountName: "myaccount",
			},
			query: url.Values{
				"domain": {"blob.core.usgovcloudapi.net", "blob.core.windows.net"},
			},
			wantErrOverrides: true,
		},
		{
			// Missing account name.
			opts:       ServiceURLOptions{},
			wantErrURL: true,
		},
		{
			// Account name set in the query
			opts: ServiceURLOptions{},
			query: url.Values{
				"storage_account": {"testaccount"},
			},
			want: "https://testaccount.blob.core.windows.net",
		},
		{
			// Basic working case.
			opts: ServiceURLOptions{
				AccountName: "myaccount",
			},
			want: "https://myaccount.blob.core.windows.net",
		},
		{
			// SASToken.
			opts: ServiceURLOptions{
				AccountName: "myaccount",
				SASToken:    "my-sas-token",
			},
			want: "https://myaccount.blob.core.windows.net?my-sas-token",
		},
		{
			// Setting domain from ServiceURLOptions.
			opts: ServiceURLOptions{
				AccountName:   "myaccount",
				StorageDomain: "blob.core.usgovcloudapi.net",
			},
			want: "https://myaccount.blob.core.usgovcloudapi.net",
		},
		{
			// Setting domain from the URL.
			opts: ServiceURLOptions{
				AccountName:   "myaccount",
				StorageDomain: "overridden",
			},
			query: url.Values{
				"domain": {"blob.core.usgovcloudapi.net"},
			},
			want: "https://myaccount.blob.core.usgovcloudapi.net",
		},
		{
			// Setting protocol from ServiceURLOptions.
			opts: ServiceURLOptions{
				AccountName: "myaccount",
				Protocol:    "http",
			},
			want: "http://myaccount.blob.core.windows.net",
		},
		{
			// Setting protocol from the URL.
			opts: ServiceURLOptions{
				AccountName: "myaccount",
				Protocol:    "https",
			},
			query: url.Values{
				"protocol": {"http"},
			},
			want: "http://myaccount.blob.core.windows.net",
		},
		{
			// Setting IsCDN from ServiceURLOptions.
			opts: ServiceURLOptions{
				AccountName: "myaccount",
				IsCDN:       true,
			},
			want: "https://blob.core.windows.net",
		},
		{
			// Setting IsCDN from the URL.
			opts: ServiceURLOptions{
				AccountName: "myaccount",
			},
			query: url.Values{
				"cdn": {"true"},
			},
			want: "https://blob.core.windows.net",
		},
		{
			// Local emulator, implicit from domain.
			opts: ServiceURLOptions{
				AccountName:   "myaccount",
				Protocol:      "http",
				StorageDomain: "localhost:10001",
			},
			want: "http://localhost:10001/myaccount",
		},
		{
			// Local emulator, implicit from domain through URL parameter.
			opts: ServiceURLOptions{
				AccountName: "myaccount",
			},
			query: url.Values{
				"protocol": {"http"},
				"domain":   {"127.0.0.1:10001"},
			},
			want: "http://127.0.0.1:10001/myaccount",
		},
		{
			// Local emulator, explicit through ServiceURLOptions.
			opts: ServiceURLOptions{
				AccountName:     "myaccount",
				StorageDomain:   "mylocalemulator",
				IsLocalEmulator: true,
			},
			want: "https://mylocalemulator/myaccount",
		},
		{
			// Local emulator, explicit through URL parameter.
			opts: ServiceURLOptions{
				AccountName:   "myaccount",
				StorageDomain: "mylocalemulator",
			},
			query: url.Values{
				"localemu": {"true"},
			},
			want: "https://mylocalemulator/myaccount",
		},
	}

	for _, test := range tests {
		opts, err := test.opts.withOverrides(test.query)
		if (err != nil) != test.wantErrOverrides {
			t.Fatalf("withOverrides got err %v want error %v", err, test.wantErrOverrides)
		}
		if err != nil {
			continue
		}
		got, err := NewServiceURL(opts)
		if (err != nil) != test.wantErrURL {
			t.Errorf("NewServiceURL got err %v want error %v", err, test.wantErrURL)
		}
		if got != test.want {
			t.Errorf("got %q want %q", got, test.want)
		}
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
		// With storage domain.
		{"azblob://mybucket?domain=blob.core.usgovcloudapi.net", false},
		// With duplicate storage domain.
		{"azblob://mybucket?domain=blob.core.usgovcloudapi.net&domain=blob.core.windows.net", true},
		// With protocol.
		{"azblob://mybucket?protocol=http", false},
		// With invalid protocol.
		{"azblob://mybucket?protocol=ftp", true},
		// With Account.
		{"azblob://mybucket?storage_account=test", false},
		// With CDN.
		{"azblob://mybucket?cdn=true", false},
		// With invalid CDN.
		{"azblob://mybucket?cdn=42", true},
		// With local emulator.
		{"azblob://mybucket?localemu=true", false},
		// With invalid local emulator.
		{"azblob://mybucket?localemu=42", true},
		// Invalid parameter.
		{"azblob://mybucket?param=value", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		b, err := blob.OpenBucket(ctx, test.URL)
		if b != nil {
			defer b.Close()
		}
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
	}
}
