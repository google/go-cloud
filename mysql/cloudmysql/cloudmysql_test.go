package cloudmysql_test

import (
	"context"
	"testing"

	"github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/proxy"
	"github.com/google/go-cloud/mysql/cloudmysql"
)

func TestOpenWithDefaultParamsFails(t *testing.T) {
	_, err := cloudmysql.Open(context.Background(), proxy.CertSource{}, nil)
	if err == nil {
		t.Error("bogus call to cloudmysql.Open succeeded")
	}
}
