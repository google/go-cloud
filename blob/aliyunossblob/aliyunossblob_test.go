package aliyunossblob

import (
	"context"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/drivertest"
	"testing"
)

// help url: https://www.alibabacloud.com/help/doc-detail/32147.htm?spm=a2c63.p38356.b99.208.639669a3WN9crs
const (
	bucketName = "go-cloud-bucket"
	endpoint   = "Endpoint"
	accessKeyID = "AccessKeyId"
	accessKeySecret = "AccessKeySecret"
)

type harness struct {
	client *oss.Client
	closer func()
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {

	client, err := oss.New(endpoint, accessKeyID, accessKeySecret)
	if err != nil {
		t.Fatal(err)
	}
	return &harness{client: client, closer: func() {

	}}, nil
}

func (h *harness) MakeBucket(ctx context.Context) (*blob.Bucket, error) {
	return OpenBucket(ctx, h.client, bucketName)
}

func (h *harness) Close() {
	h.closer()
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, "../testdata")
}
