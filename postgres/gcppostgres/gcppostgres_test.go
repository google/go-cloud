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

package gcppostgres

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"gocloud.dev/internal/testing/terraform"
	"gocloud.dev/postgres"
)

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
	project, _ := tfOut["project"].Value.(string)
	region, _ := tfOut["region"].Value.(string)
	instance, _ := tfOut["instance"].Value.(string)
	username, _ := tfOut["username"].Value.(string)
	password, _ := tfOut["password"].Value.(string)
	databaseName, _ := tfOut["database"].Value.(string)
	userEmail, _ := tfOut["user_email"].Value.(string)
	if project == "" || region == "" || instance == "" || username == "" || databaseName == "" || userEmail == "" {
		t.Fatalf("Missing one or more required Terraform outputs; got project=%q region=%q instance=%q username=%q database=%q userEmail=%q", project, region, instance, username, databaseName, userEmail)
	}
	tests := []struct {
		name    string
		urlstr  string
		wantErr bool
	}{
		{
			name:   "SuccessIam",
			urlstr: fmt.Sprintf("gcppostgres://%s@%s/%s/%s/%s", userEmail, project, region, instance, databaseName),
		},
		{
			name:   "SuccessBuiltin",
			urlstr: fmt.Sprintf("gcppostgres://%s:%s@%s/%s/%s/%s", username, password, project, region, instance, databaseName),
		},
		{
			name:    "SSLModeForbidden",
			urlstr:  fmt.Sprintf("gcppostgres://%s:%s@%s/%s/%s/%s?sslmode=require", username, password, project, region, instance, databaseName),
			wantErr: true,
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, err := postgres.Open(ctx, test.urlstr)
			if err != nil {
				t.Log(err)
				if !test.wantErr {
					t.Fail()
				}
				return
			}
			if test.wantErr {
				_ = db.Close()
				t.Fatal("Open succeeded; want error")
			}
			if err := db.Ping(); err != nil {
				t.Error("Ping:", err)
			}
			if err := db.Close(); err != nil {
				t.Error("Close:", err)
			}
		})
	}
}

// stubCertSource is a minimal proxy.CertSource used only to reach
// URLOpener.OpenPostgresURL's error paths without needing real GCP
// credentials or network access.
type stubCertSource struct{}

func (stubCertSource) Local(instance string) (tls.Certificate, error) {
	return tls.Certificate{}, fmt.Errorf("stub: not implemented")
}

func (stubCertSource) Remote(instance string) (cert *x509.Certificate, addr, name, version string, err error) {
	return nil, "", "", "", fmt.Errorf("stub: not implemented")
}

func TestOpenPostgresURLDoesNotLeakPassword(t *testing.T) {
	const password = "S3cr3tDBPassw0rd"
	// Missing the dbname path segment, which fails instanceFromURL.
	u, err := url.Parse(fmt.Sprintf("gcppostgres://dbuser:%s@myproject/us-central1/mydb", password))
	if err != nil {
		t.Fatalf("failed to parse URL: %v", err)
	}
	opener := &URLOpener{CertSource: stubCertSource{}}
	_, err = opener.OpenPostgresURL(context.Background(), u)
	if err == nil {
		t.Fatal("OpenPostgresURL: got nil error, want error for malformed URL")
	}
	if strings.Contains(err.Error(), password) {
		t.Errorf("OpenPostgresURL error contains the raw password: %q", err.Error())
	}
}

func TestInstanceFromURL(t *testing.T) {
	tests := []struct {
		name         string
		urlString    string
		wantInstance string
		wantDatabase string
		wantErr      bool
	}{
		{
			name:         "AllValuesSpecified",
			urlString:    "gcppostgres://username:password@my-project-id/us-central1/my-instance-id/my-db?foo=bar&baz=quux",
			wantInstance: "my-project-id:us-central1:my-instance-id",
			wantDatabase: "my-db",
		},
		{
			name:         "OptionalValuesOmitted",
			urlString:    "gcppostgres://my-project-id/us-central1/my-instance-id/my-db",
			wantInstance: "my-project-id:us-central1:my-instance-id",
			wantDatabase: "my-db",
		},
		{
			name:      "DatabaseNameEmpty",
			urlString: "gcppostgres://my-project-id/us-central1/my-instance-id/",
			wantErr:   true,
		},
		{
			name:      "InstanceEmpty",
			urlString: "gcppostgres://my-project-id/us-central1//my-db",
			wantErr:   true,
		},
		{
			name:      "RegionEmpty",
			urlString: "gcppostgres://my-project-id//my-instance-id/my-db",
			wantErr:   true,
		},
		{
			name:      "ProjectEmpty",
			urlString: "gcppostgres:///us-central1/my-instance-id/my-db",
			wantErr:   true,
		},
		{
			name:         "DatabaseNameWithSlashes",
			urlString:    "gcppostgres://my-project-id/us-central1/my-instance-id/foo/bar/baz",
			wantInstance: "my-project-id:us-central1:my-instance-id",
			wantDatabase: "foo/bar/baz",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			u, err := url.Parse(test.urlString)
			if err != nil {
				t.Fatalf("failed to parse URL %q: %v", test.urlString, err)
			}
			instance, database, err := instanceFromURL(u)
			if err != nil {
				t.Logf("instanceFromURL(url.Parse(%q)): %v", u, err)
				if !test.wantErr {
					t.Fail()
				}
				return
			}
			if test.wantErr {
				t.Fatalf("instanceFromURL(url.Parse(%q)) = %q, %q, <nil>; want error", test.urlString, instance, database)
			}
			if instance != test.wantInstance || database != test.wantDatabase {
				t.Errorf("instanceFromURL(url.Parse(%q)) = %q, %q, <nil>; want %q, %q, <nil>", test.urlString, instance, database, test.wantInstance, test.wantDatabase)
			}
		})
	}
}
