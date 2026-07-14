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

package awspostgres

import (
	"context"
	"fmt"
	"io"
	"testing"

	"gocloud.dev/internal/testing/terraform"
	"gocloud.dev/postgres"
)

func closeWithErrorCheck(t testing.TB, c io.Closer) {
	t.Helper()
	if err := c.Close(); err != nil {
		t.Errorf("failed to Close: %v", err)
	}
}

func TestURLOpener(t *testing.T) {
	// This test will be skipped unless the project is set up with Terraform.
	// Before running go test, run in this directory:
	//
	// terraform init
	// terraform apply

	tfOut, err := terraform.ReadOutput(".")
	if err != nil || len(tfOut) == 0 {
		t.Skipf("Could not obtain harness info: %v", err)
	}
	endpoint, _ := tfOut["endpoint"].Value.(string)
	username, _ := tfOut["username"].Value.(string)
	password, _ := tfOut["password"].Value.(string)
	databaseName, _ := tfOut["database"].Value.(string)
	if endpoint == "" || username == "" || databaseName == "" {
		t.Fatalf("Missing one or more required Terraform outputs; got endpoint=%q username=%q database=%q", endpoint, username, databaseName)
	}

	tests := []struct {
		urlstr      string
		wantErr     bool
		wantPingErr bool
	}{
		// OK.
		{fmt.Sprintf("awspostgres://%s:%s@%s/%s", username, password, endpoint, databaseName), false, false},
		// Invalid URL parameters: db creation fails.
		{fmt.Sprintf("awspostgres://%s:%s@%s/%s?sslcert=foo", username, password, endpoint, databaseName), true, false},
		{fmt.Sprintf("awspostgres://%s:%s@%s/%s?sslkey=foo", username, password, endpoint, databaseName), true, false},
		{fmt.Sprintf("awspostgres://%s:%s@%s/%s?sslrootcert=foo", username, password, endpoint, databaseName), true, false},
		{fmt.Sprintf("awspostgres://%s:%s@%s/%s?sslmode=require", username, password, endpoint, databaseName), true, false},
		// Invalid connection info: db is created, but Ping fails.
		{fmt.Sprintf("awspostgres://%s:badpwd@%s/%s", username, endpoint, databaseName), false, true},
		{fmt.Sprintf("awspostgres://badusername:%s@%s/%s", password, endpoint, databaseName), false, true},
		{fmt.Sprintf("awspostgres://%s:%s@localhost:9999/%s", username, password, databaseName), false, true},
		{fmt.Sprintf("awspostgres://%s:%s@%s/wrongdbname", username, password, endpoint), false, true},
		{fmt.Sprintf("awspostgres://%s:%s@%s/%s?foo=bar", username, password, endpoint, databaseName), false, true},
	}
	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.urlstr, func(t *testing.T) {
			db, err := postgres.Open(ctx, test.urlstr)
			if err != nil != test.wantErr {
				t.Fatalf("got err %v, wanted error? %v", err, test.wantErr)
			}
			if err != nil {
				return
			}
			defer closeWithErrorCheck(t, db)
			err = db.Ping()
			if err != nil != test.wantPingErr {
				t.Errorf("ping got err %v, wanted error? %v", err, test.wantPingErr)
			}
		})
	}
}

func TestURLOpenerIAM(t *testing.T) {
	// This test will be skipped unless the project is set up with Terraform
	// and IAM authentication is configured.
	// Before running go test, run in this directory:
	//
	// terraform init
	// terraform apply

	tfOut, err := terraform.ReadOutput(".")
	if err != nil || len(tfOut) == 0 {
		t.Skipf("Could not obtain harness info: %v", err)
	}
	endpoint, _ := tfOut["endpoint"].Value.(string)
	iamUsername, _ := tfOut["iam_username"].Value.(string)
	databaseName, _ := tfOut["database"].Value.(string)
	roleARN, _ := tfOut["iam_role_arn"].Value.(string)
	if endpoint == "" || iamUsername == "" || databaseName == "" {
		t.Skipf("Missing IAM Terraform outputs; got endpoint=%q iam_username=%q database=%q", endpoint, iamUsername, databaseName)
	}

	ctx := context.Background()

	// Test IAM auth without role assumption.
	urlstr := fmt.Sprintf("awspostgres://%s@%s/%s", iamUsername, endpoint, databaseName)
	t.Run(urlstr, func(t *testing.T) {
		db, err := postgres.Open(ctx, urlstr)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer closeWithErrorCheck(t, db)
		if err := db.Ping(); err != nil {
			t.Errorf("Ping: %v", err)
		}
	})

	// Test IAM auth with role assumption.
	if roleARN != "" {
		urlstr := fmt.Sprintf("awspostgres://%s@%s/%s?aws_role_arn=%s", iamUsername, endpoint, databaseName, roleARN)
		t.Run(urlstr, func(t *testing.T) {
			db, err := postgres.Open(ctx, urlstr)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer closeWithErrorCheck(t, db)
			if err := db.Ping(); err != nil {
				t.Errorf("Ping: %v", err)
			}
		})
	}
}
