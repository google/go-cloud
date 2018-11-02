package cloudmysql_test

import (
	"context"
	"testing"

	"github.com/google/go-cloud/mysql/cloudmysql"
)

func TestOpenWithDefaultParamsGivesNoError(t *testing.T) {
	ctx := context.Background()
	_, err := cloudmysql.Open(ctx, nil, &cloudmysql.Params{})
	if err != nil {
		t.Error(err.Error())
	}
}
