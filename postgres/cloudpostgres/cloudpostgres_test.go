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

package cloudpostgres

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/gcp/cloudsql"
	"github.com/google/go-cloud/internal/testing/terraform"
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
	project, _ := tfOut["project"].Value.(string)
	region, _ := tfOut["region"].Value.(string)
	instance, _ := tfOut["instance"].Value.(string)
	username, _ := tfOut["username"].Value.(string)
	password, _ := tfOut["password"].Value.(string)
	databaseName, _ := tfOut["database"].Value.(string)
	if project == "" || region == "" || instance == "" || username == "" || databaseName == "" {
		t.Fatalf("Missing one or more required Terraform outputs; got project=%q region=%q instance=%q username=%q database=%q", project, region, instance, username, databaseName)
	}

	ctx := context.Background()
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client, err := gcp.NewHTTPClient(http.DefaultTransport, creds.TokenSource)
	if err != nil {
		t.Fatal(err)
	}
	certSource := cloudsql.NewCertSource(client)
	db, err := Open(ctx, certSource, &Params{
		ProjectID: project,
		Region:    region,
		Instance:  instance,
		Database:  databaseName,
		User:      username,
		Password:  password,
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

func TestOpenBadValue(t *testing.T) {
	// This test will be skipped unless the project is set up with Terraform.

	tfOut, err := terraform.ReadOutput(".")
	if err != nil {
		t.Skipf("Could not obtain harness info: %v", err)
	}
	project, _ := tfOut["project"].Value.(string)
	region, _ := tfOut["region"].Value.(string)
	instance, _ := tfOut["instance"].Value.(string)
	username, _ := tfOut["username"].Value.(string)
	password, _ := tfOut["password"].Value.(string)
	databaseName, _ := tfOut["database"].Value.(string)
	if project == "" || region == "" || instance == "" || username == "" || databaseName == "" {
		t.Fatalf("Missing one or more required Terraform outputs; got project=%q region=%q instance=%q username=%q database=%q", project, region, instance, username, databaseName)
	}

	ctx := context.Background()
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client, err := gcp.NewHTTPClient(http.DefaultTransport, creds.TokenSource)
	if err != nil {
		t.Fatal(err)
	}
	certSource := cloudsql.NewCertSource(client)

	tests := []struct {
		name, value string
	}{
		{"user", "foo"},
		{"password", "foo"},
		{"dbname", "foo"},
		{"host", "localhost"},
		{"port", "1234"},
		{"sslmode", "require"},
	}
	for _, test := range tests {
		t.Run(test.name+"="+test.value, func(t *testing.T) {
			db, err := Open(ctx, certSource, &Params{
				ProjectID: project,
				Region:    region,
				Instance:  instance,
				Database:  databaseName,
				User:      username,
				Password:  password,
				Values:    url.Values{test.name: {test.value}},
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
