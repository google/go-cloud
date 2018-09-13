package azureblob

import (
	"context"
	"os"
	"testing"

	"github.com/google/go-cloud/blob"
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
	accountName := os.Getenv("ACCOUNT_NAME")
	accountKey := os.Getenv("ACCOUNT_KEY")
	s := Settings{
		AccountName:      accountName,
		AccountKey:       accountKey,
		PublicAccessType: PublicAccessBlob,
		SASToken:         "",
		Pipeline:         p,
	}

	return &harness{settings: s, closer: done}, nil
}

func (h *harness) MakeBucket(ctx context.Context) (*blob.Bucket, error) {
	return OpenBucket(ctx, &h.settings, bucketName)
}

func (h *harness) Close() {
	h.closer()
}
func TestConformance(t *testing.T) {
	// test requires the following environment variables
	// Account_Name is the Azure Storage Account Name
	// Account_Key is the Primary or Secondary Key
	os.Setenv("ACCOUNT_NAME", "gocloud")
	os.Setenv("ACCOUNT_KEY", "")

	drivertest.RunConformanceTests(t, newHarness, "../testdata")
}
