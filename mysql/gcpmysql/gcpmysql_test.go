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

package gcpmysql

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"
	"reflect"
	"testing"

	"cloud.google.com/go/cloudsqlconn"
	drvr "github.com/go-sql-driver/mysql"
	"gocloud.dev/gcp/cloudsql"
	"gocloud.dev/internal/testing/terraform"
	"gocloud.dev/mysql"
	"golang.org/x/oauth2"
)

func TestOpen(t *testing.T) {
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
	if project == "" || region == "" || instance == "" || username == "" || databaseName == "" {
		t.Fatalf("Missing one or more required Terraform outputs; got project=%q region=%q instance=%q username=%q database=%q", project, region, instance, username, databaseName)
	}

	ctx := context.Background()
	urlstr := fmt.Sprintf("gcpmysql://%s:%s@%s/%s/%s/%s", username, password, project, region, instance, databaseName)
	t.Log("Connecting to", urlstr)
	db, err := mysql.Open(ctx, urlstr)
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

func TestOpenMySQLURL_Errors(t *testing.T) {
	ctx := context.Background()
	base := "gcpmysql://user:pass@project/us-central1/instance/db"

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
			// ip_type validation only runs on the Dialer path; legacy silently ignores it.
			name: "InvalidIPType",
			uo:   &URLOpener{Dialer: d},
			url:  base + "?ip_type=bogus",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, err := tc.uo.OpenMySQLURL(ctx, mustParseURL(tc.url))
			if err == nil {
				db.Close()
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestOpenMySQLURL_DialerPath(t *testing.T) {
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
		{"DefaultIPType", "gcpmysql://user:pass@project/us-central1/instance/db"},
		{"ExplicitAutoIPType", "gcpmysql://user:pass@project/us-central1/instance/db?ip_type=auto"},
		{"PublicIPType", "gcpmysql://user:pass@project/us-central1/instance/db?ip_type=public"},
		{"PrivateIPType", "gcpmysql://user:pass@project/us-central1/instance/db?ip_type=private"},
		{"PSCIPType", "gcpmysql://user:pass@project/us-central1/instance/db?ip_type=psc"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uo := &URLOpener{Dialer: d}
			db, err := uo.OpenMySQLURL(ctx, mustParseURL(tc.url))
			if err != nil {
				t.Fatalf("OpenMySQLURL(%q): %v", tc.url, err)
			}
			db.Close()
		})
	}
}

func TestOpenMySQLURL_LegacyPath(t *testing.T) {
	ctx := context.Background()
	uo := &URLOpener{CertSource: mockCertSource{}}
	db, err := uo.OpenMySQLURL(ctx, mustParseURL("gcpmysql://user:pass@project/us-central1/instance/db"))
	if err != nil {
		t.Fatalf("OpenMySQLURL: %v", err)
	}
	db.Close()
}

// TestOpenMySQLURL_DialerTakesPrecedence verifies that when both Dialer and
// CertSource are set, the Dialer path is taken. The two paths differ in how they
// handle ip_type: the Dialer path validates and returns an error for unknown values;
// the legacy CertSource path silently ignores ip_type. Passing ip_type=bogus
// therefore errors only if the Dialer path ran.
func TestOpenMySQLURL_DialerTakesPrecedence(t *testing.T) {
	ctx := context.Background()
	fakeTS := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "fake"})
	d, err := cloudsqlconn.NewDialer(ctx, cloudsqlconn.WithTokenSource(fakeTS))
	if err != nil {
		t.Fatalf("NewDialer: %v", err)
	}
	defer d.Close()

	uo := &URLOpener{Dialer: d, CertSource: mockCertSource{}}
	db, err := uo.OpenMySQLURL(ctx, mustParseURL("gcpmysql://user:pass@project/us-central1/instance/db?ip_type=bogus"))
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
			urlString:    "gcpmysql://username:password@my-project-id/us-central1/my-instance-id/my-db?foo=bar&baz=quux",
			wantInstance: "my-project-id:us-central1:my-instance-id",
			wantDatabase: "my-db",
		},
		{
			name:         "OptionalValuesOmitted",
			urlString:    "gcpmysql://my-project-id/us-central1/my-instance-id/my-db",
			wantInstance: "my-project-id:us-central1:my-instance-id",
			wantDatabase: "my-db",
		},
		{
			name:      "DatabaseNameEmpty",
			urlString: "gcpmysql://my-project-id/us-central1/my-instance-id/",
			wantErr:   true,
		},
		{
			name:      "InstanceEmpty",
			urlString: "gcpmysql://my-project-id/us-central1//my-db",
			wantErr:   true,
		},
		{
			name:      "RegionEmpty",
			urlString: "gcpmysql://my-project-id//my-instance-id/my-db",
			wantErr:   true,
		},
		{
			name:      "ProjectEmpty",
			urlString: "gcpmysql:///us-central1/my-instance-id/my-db",
			wantErr:   true,
		},
		{
			name:         "DatabaseNameWithSlashes",
			urlString:    "gcpmysql://my-project-id/us-central1/my-instance-id/foo/bar/baz",
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

func Test_configFromURL(t *testing.T) {
	type args struct {
		urlStr string
	}
	tests := []struct {
		name    string
		args    args
		want    *drvr.Config
		wantErr bool
	}{
		{
			name: "ConfigWithNoOptions",
			args: args{
				urlStr: "gcpmysql://user:password@my-project-id/us-central1/my-instance-id/my-db",
			},
			want: func() *drvr.Config {
				cfg := drvr.NewConfig()
				cfg.AllowNativePasswords = true
				cfg.Net = "tcp"
				cfg.Addr = "my-project-id:us-central1:my-instance-id"
				cfg.User = "user"
				cfg.Passwd = "password"
				cfg.DBName = "my-db"
				return cfg
			}(),
			wantErr: false,
		},
		{
			name: "ConfigWithSignalOptions",
			args: args{
				urlStr: "gcpmysql://user:password@my-project-id/us-central1/my-instance-id/my-db?parseTime=true",
			},
			want: func() *drvr.Config {
				cfg := drvr.NewConfig()
				cfg.AllowNativePasswords = true
				cfg.ParseTime = true
				cfg.Net = "tcp"
				cfg.Addr = "my-project-id:us-central1:my-instance-id"
				cfg.User = "user"
				cfg.Passwd = "password"
				cfg.DBName = "my-db"
				return cfg
			}(),
			wantErr: false,
		},
		{
			name: "ConfigWithMultipleOptions",
			args: args{
				urlStr: "gcpmysql://user:password@my-project-id/us-central1/my-instance-id/my-db?columnsWithAlias=true&parseTime=true",
			},
			want: func() *drvr.Config {
				cfg := drvr.NewConfig()
				cfg.AllowNativePasswords = true
				cfg.ColumnsWithAlias = true
				cfg.ParseTime = true
				cfg.Net = "tcp"
				cfg.Addr = "my-project-id:us-central1:my-instance-id"
				cfg.User = "user"
				cfg.Passwd = "password"
				cfg.DBName = "my-db"
				return cfg
			}(),
			wantErr: false,
		},
		{
			name: "InstanceFromURLError",
			args: args{
				urlStr: "gcpmysql://user:password@my-project-id/us-central1/my-db",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "DNSParseError",
			args: args{
				urlStr: "gcpmysql://user:password@my-project-id/us-central1/my-instance-id/my-db?parseTime=nope",
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.args.urlStr)
			if err != nil {
				t.Fatalf("failed to parse URL %q: %v", tt.args.urlStr, err)
			}
			got, err := configFromURL(u)
			if (err != nil) != tt.wantErr {
				t.Errorf("configFromURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("configFromURL() = %v, want %v", got, tt.want)
			}
		})
	}
}
