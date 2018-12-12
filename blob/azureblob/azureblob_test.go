package azureblob

import (
	"context"
	"net/http"
	"os"
	"testing"
	
	"github.com/google/go-cloud/blob/driver"
	"github.com/google/go-cloud/blob/drivertest"
	"github.com/google/go-cloud/internal/testing/setup"
)

const (
	bucketName = "go-cloud-bucket"
)

type harness struct {
	settings Settings
	closer   func()
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	p, done := setup.NewAzureTestPipeline(ctx, t)
	accountName := os.Getenv("AZURE_STORAGE_ACCOUNT_NAME")
	accountKey := os.Getenv("AZURE_STORAGE_ACCOUNT_KEY")
	s := &Settings{
		AccountName:      accountName,
		AccountKey:       accountKey,
		PublicAccessType: PublicAccessBlob,
		SASToken:         "",
		Pipeline:         p,
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
	// test requires the following environment variables
	// AZURE_STORAGE_ACCOUNT_NAME is the Azure Storage Account Name
	// AZURE_STORAGE_ACCOUNT_KEY is the Primary or Secondary Key
	os.Setenv("AZURE_STORAGE_ACCOUNT_NAME", "gocloud")
	os.Setenv("AZURE_STORAGE_ACCOUNT_KEY", "")

	drivertest.RunConformanceTests(t, newHarness, nil)
}
