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

package cloudpostgres

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gocloud.dev/gcp"
	"gocloud.dev/gcp/cloudsql"
	"gocloud.dev/internal/testing/terraform"
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
	db, _, err := Open(ctx, certSource, &Params{
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
			db, _, err := Open(ctx, certSource, &Params{
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

func TestParamsFromURL(t *testing.T) {
	tests := []struct {
		name      string
		urlString string
		want      *Params
		wantErr   bool
	}{
		{
			name:      "AllValuesSpecified",
			urlString: "cloudpostgres://username:password@my-project-id/us-central1/my-instance-id/my-db?foo=bar&baz=quux",
			want: &Params{
				ProjectID: "my-project-id",
				Region:    "us-central1",
				Instance:  "my-instance-id",
				Database:  "my-db",
				User:      "username",
				Password:  "password",
				Values: url.Values{
					"foo": {"bar"},
					"baz": {"quux"},
				},
			},
		},
		{
			name:      "OptionalValuesOmitted",
			urlString: "cloudpostgres://my-project-id/us-central1/my-instance-id/my-db",
			want: &Params{
				ProjectID: "my-project-id",
				Region:    "us-central1",
				Instance:  "my-instance-id",
				Database:  "my-db",
			},
		},
		{
			name:      "DatabaseNameEmpty",
			urlString: "cloudpostgres://my-project-id/us-central1/my-instance-id/",
			wantErr:   true,
		},
		{
			name:      "InstanceEmpty",
			urlString: "cloudpostgres://my-project-id/us-central1//my-db",
			wantErr:   true,
		},
		{
			name:      "RegionEmpty",
			urlString: "cloudpostgres://my-project-id//my-instance-id/my-db",
			wantErr:   true,
		},
		{
			name:      "ProjectEmpty",
			urlString: "cloudpostgres:///us-central1/my-instance-id/my-db",
			wantErr:   true,
		},
		{
			name:      "DatabaseNameWithSlashes",
			urlString: "cloudpostgres://my-project-id/us-central1/my-instance-id/foo/bar/baz",
			want: &Params{
				ProjectID: "my-project-id",
				Region:    "us-central1",
				Instance:  "my-instance-id",
				Database:  "foo/bar/baz",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			u, err := url.Parse(test.urlString)
			if err != nil {
				t.Fatalf("failed to parse URL %q: %v", test.urlString, err)
			}
			got, err := paramsFromURL(u)
			if err != nil {
				t.Logf("paramsFromURL(url.Parse(%q)): %v", u, err)
				if !test.wantErr {
					t.Fail()
				}
				return
			}
			if test.wantErr {
				t.Fatalf("paramsFromURL(url.Parse(%q)) = %#v; want error", test.urlString, got)
			}
			diff := cmp.Diff(test.want, got,
				cmpopts.IgnoreFields(Params{}, "TraceOpts"),
				cmpopts.EquateEmpty())
			if diff != "" {
				t.Errorf("paramsFromURL(url.Parse(%q)) (-want +got):\n%s", test.urlString, diff)
			}
		})
	}
}
