package cloudmysql_test

import (
	"context"
	"testing"

	"github.com/google/go-cloud/mysql/cloudmysql"
	"github.com/opencensus-integrations/ocsql"
)

func TestOpenWithDefaultParamsGivesNoError(t *testing.T) {
	ctx := context.Background()
	params := &cloudmysql.Params{
		TraceOpts: ocsql.WithAllTraceOptions(),
	}
	_, err := cloudmysql.Open(ctx, nil, params)
	if err != nil {
		t.Error(err.Error())
	}
}
