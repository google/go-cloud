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

package rdspostgres

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"gocloud.dev/aws/rds"
	"gocloud.dev/internal/testing/terraform"
	"gocloud.dev/postgres"
)

func TestOpen(t *testing.T) {
	// This test will be skipped unless the project is set up with Terraform.
	// Before running go test, run in this directory:
	//
	// terraform init
	// terraform apply

	tfOut, err := terraform.ReadOutput(".")
	if err != nil {
		t.Skipf("Could not obtain harness info: %v", err)
	}
	endpoint, _ := tfOut["endpoint"].Value.(string)
	username, _ := tfOut["username"].Value.(string)
	password, _ := tfOut["password"].Value.(string)
	databaseName, _ := tfOut["database"].Value.(string)
	if endpoint == "" || username == "" || databaseName == "" {
		t.Fatalf("Missing one or more required Terraform outputs; got endpoint=%q username=%q database=%q", endpoint, username, databaseName)
	}

	ctx := context.Background()
	cf := new(rds.CertFetcher)
	db, _, err := Open(ctx, cf, &Params{
		Endpoint: endpoint,
		User:     username,
		Password: password,
		Database: databaseName,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Error("Ping:", err)
	}
	if err := db.Close(); err != nil {
		t.Error("Close:", err)
	}
}

func TestOpenWithURL(t *testing.T) {
	// This test will be skipped unless the project is set up with Terraform.
	// Before running go test, run in this directory:
	//
	// terraform init
	// terraform apply

	tfOut, err := terraform.ReadOutput(".")
	if err != nil {
		t.Skipf("Could not obtain harness info: %v", err)
	}
	endpoint, _ := tfOut["endpoint"].Value.(string)
	username, _ := tfOut["username"].Value.(string)
	password, _ := tfOut["password"].Value.(string)
	databaseName, _ := tfOut["database"].Value.(string)
	if endpoint == "" || username == "" || databaseName == "" {
		t.Fatalf("Missing one or more required Terraform outputs; got endpoint=%q username=%q database=%q", endpoint, username, databaseName)
	}

	ctx := context.Background()

	tests := []struct {
		urlstr      string
		wantErr     bool
		wantPingErr bool
	}{
		// OK.
		{fmt.Sprintf("rdspostgres://%s:%s@%s/%s", username, password, endpoint, databaseName), false, false},
		// Invalid URL parameters: db creation fails.
		{fmt.Sprintf("rdspostgres://%s:%s@%s/%s?sslcert=foo", username, password, endpoint, databaseName), true, false},
		{fmt.Sprintf("rdspostgres://%s:%s@%s/%s?sslkey=foo", username, password, endpoint, databaseName), true, false},
		{fmt.Sprintf("rdspostgres://%s:%s@%s/%s?sslrootcert=foo", username, password, endpoint, databaseName), true, false},
		{fmt.Sprintf("rdspostgres://%s:%s@%s/%s?sslmode=require", username, password, endpoint, databaseName), true, false},
		// Invalid connection info: db is created, but Ping fails.
		{fmt.Sprintf("rdspostgres://%s:badpwd@%s/%s", username, endpoint, databaseName), false, true},
		{fmt.Sprintf("rdspostgres://badusername:%s@%s/%s", password, endpoint, databaseName), false, true},
		{fmt.Sprintf("rdspostgres://%s:%s@localhost:9999/%s", username, password, databaseName), false, true},
		{fmt.Sprintf("rdspostgres://%s:%s@%s/wrongdbname", username, password, endpoint), false, true},
		{fmt.Sprintf("rdspostgres://%s:%s@%s/%s?foo=bar", username, password, endpoint, databaseName), false, true},
	}
	for _, test := range tests {
		t.Run(test.urlstr, func(t *testing.T) {
			db, err := postgres.Open(ctx, test.urlstr)
			if err != nil != test.wantErr {
				t.Fatalf("got err %v, wanted error? %v", err, test.wantErr)
			}
			if err != nil {
				return
			}
			defer func() {
				if err := db.Close(); err != nil {
					t.Error("Close:", err)
				}
			}()
			err = db.Ping()
			if err != nil != test.wantPingErr {
				t.Errorf("ping got err %v, wanted error? %v", err, test.wantPingErr)
			}
		})
	}
}

func TestOpenBadValues(t *testing.T) {
	// This test will be skipped unless the project is set up with Terraform.

	tfOut, err := terraform.ReadOutput(".")
	if err != nil {
		t.Skipf("Could not obtain harness info: %v", err)
	}
	endpoint, _ := tfOut["endpoint"].Value.(string)
	username, _ := tfOut["username"].Value.(string)
	password, _ := tfOut["password"].Value.(string)
	databaseName, _ := tfOut["database"].Value.(string)
	if endpoint == "" || username == "" || databaseName == "" {
		t.Fatalf("Missing one or more required Terraform outputs; got endpoint=%q username=%q database=%q", endpoint, username, databaseName)
	}

	ctx := context.Background()
	cf := new(rds.CertFetcher)
	tests := []struct {
		name, value string
	}{
		{"sslmode", "require"},
		{"sslcert", "foo"},
		{"sslkey", "foo"},
		{"sslrootcert", "foo"},
	}
	for _, test := range tests {
		t.Run(test.name+"="+test.value, func(t *testing.T) {
			db, _, err := Open(ctx, cf, &Params{
				Endpoint: endpoint,
				User:     username,
				Password: password,
				Database: databaseName,
				Values:   url.Values{test.name: {test.value}},
			})
			if err == nil || !strings.Contains(err.Error(), test.name) {
				t.Errorf("error = %v; want to contain %q", err, test.name)
			}
			if db != nil {
				db.Close()
			}
		})
	}
}
