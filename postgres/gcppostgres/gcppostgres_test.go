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
	"testing"

	"cloud.google.com/go/cloudsqlconn"
	"gocloud.dev/gcp/cloudsql"
	"gocloud.dev/internal/testing/terraform"
	"gocloud.dev/postgres"
	"golang.org/x/oauth2"
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
				db.Close()
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

// mockCertSource is a no-op proxy.CertSource for unit tests.
type mockCertSource struct{}

func (mockCertSource) Local(instance string) (tls.Certificate, error) {
	return tls.Certificate{}, nil
}
func (mockCertSource) Remote(instance string) (*x509.Certificate, string, string, string, error) {
	return nil, "", "", "", nil
}

func mustParseURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}

func TestParseIPType(t *testing.T) {
	tests := []struct {
		input   string
		want    cloudsql.IPType
		wantErr bool
	}{
		{"auto", cloudsql.IPTypeAuto, false},
		{"public", cloudsql.IPTypePublic, false},
		{"private", cloudsql.IPTypePrivate, false},
		{"psc", cloudsql.IPTypePSC, false},
		{"", cloudsql.IPTypeAuto, false},         // empty string defaults to auto
		{"PUBLIC", cloudsql.IPTypePublic, false}, // case-insensitive
		{"invalid", 0, true},
		{"public ", 0, true}, // whitespace is not trimmed
	}
	for _, tc := range tests {
		got, err := parseIPType(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("parseIPType(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("parseIPType(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestOpenPostgresURL_Errors(t *testing.T) {
	ctx := context.Background()
	base := "gcppostgres://user:pass@project/us-central1/instance/db"

	fakeTS := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "fake"})
	d, err := cloudsqlconn.NewDialer(ctx, cloudsqlconn.WithTokenSource(fakeTS))
	if err != nil {
		t.Fatalf("NewDialer: %v", err)
	}
	defer d.Close()

	tests := []struct {
		name string
		uo   *URLOpener
		url  string
	}{
		{
			name: "NilDialerAndCertSource",
			uo:   &URLOpener{},
			url:  base,
		},
		{
			name: "ForbiddenSSLMode",
			uo:   &URLOpener{CertSource: mockCertSource{}},
			url:  base + "?sslmode=require",
		},
		{
			name: "ForbiddenSSLCert",
			uo:   &URLOpener{CertSource: mockCertSource{}},
			url:  base + "?sslcert=/tmp/cert",
		},
		{
			// ip_type validation only runs on the Dialer path; legacy silently ignores it.
			name: "InvalidIPType",
			uo:   &URLOpener{Dialer: d},
			url:  base + "?ip_type=bogus",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, err := tc.uo.OpenPostgresURL(ctx, mustParseURL(tc.url))
			if err == nil {
				db.Close()
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestOpenPostgresURL_DialerPath(t *testing.T) {
	ctx := context.Background()
	fakeTS := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "fake"})
	d, err := cloudsqlconn.NewDialer(ctx, cloudsqlconn.WithTokenSource(fakeTS))
	if err != nil {
		t.Fatalf("NewDialer: %v", err)
	}
	defer d.Close()

	tests := []struct {
		name string
		url  string
	}{
		{"DefaultIPType", "gcppostgres://user:pass@project/us-central1/instance/db"},
		{"ExplicitAutoIPType", "gcppostgres://user:pass@project/us-central1/instance/db?ip_type=auto"},
		{"PublicIPType", "gcppostgres://user:pass@project/us-central1/instance/db?ip_type=public"},
		{"PrivateIPType", "gcppostgres://user:pass@project/us-central1/instance/db?ip_type=private"},
		{"PSCIPType", "gcppostgres://user:pass@project/us-central1/instance/db?ip_type=psc"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uo := &URLOpener{Dialer: d}
			db, err := uo.OpenPostgresURL(ctx, mustParseURL(tc.url))
			if err != nil {
				t.Fatalf("OpenPostgresURL(%q): %v", tc.url, err)
			}
			db.Close()
		})
	}
}

func TestOpenPostgresURL_LegacyPath(t *testing.T) {
	ctx := context.Background()
	uo := &URLOpener{CertSource: mockCertSource{}}
	db, err := uo.OpenPostgresURL(ctx, mustParseURL("gcppostgres://user:pass@project/us-central1/instance/db"))
	if err != nil {
		t.Fatalf("OpenPostgresURL: %v", err)
	}
	db.Close()
}

// TestOpenPostgresURL_DialerTakesPrecedence verifies that when both Dialer and
// CertSource are set, the Dialer path is taken. The two paths differ in how they
// handle ip_type: the Dialer path validates and returns an error for unknown values;
// the legacy CertSource path silently ignores ip_type. Passing ip_type=bogus
// therefore errors only if the Dialer path ran.
func TestOpenPostgresURL_DialerTakesPrecedence(t *testing.T) {
	ctx := context.Background()
	fakeTS := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "fake"})
	d, err := cloudsqlconn.NewDialer(ctx, cloudsqlconn.WithTokenSource(fakeTS))
	if err != nil {
		t.Fatalf("NewDialer: %v", err)
	}
	defer d.Close()

	uo := &URLOpener{Dialer: d, CertSource: mockCertSource{}}
	db, err := uo.OpenPostgresURL(ctx, mustParseURL("gcppostgres://user:pass@project/us-central1/instance/db?ip_type=bogus"))
	if err == nil {
		db.Close()
		t.Fatal("expected ip_type validation error from Dialer path; CertSource path would have silently ignored ip_type")
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
