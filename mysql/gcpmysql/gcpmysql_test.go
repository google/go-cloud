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
	"fmt"
	"net/url"
	"reflect"
	"testing"

	drvr "github.com/go-sql-driver/mysql"
	"gocloud.dev/internal/testing/terraform"
	"gocloud.dev/mysql"
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
		urlStr     string
		dialerName string
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
				urlStr:     "gcpmysql://user:password@my-project-id/us-central1/my-instance-id/my-db",
				dialerName: "gocloud.dev/mysql/gcpmysql/1",
			},
			want: func() *drvr.Config {
				cfg := drvr.NewConfig()
				cfg.AllowNativePasswords = true
				cfg.Net = "gocloud.dev/mysql/gcpmysql/1"
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
				urlStr:     "gcpmysql://user:password@my-project-id/us-central1/my-instance-id/my-db?parseTime=true",
				dialerName: "gocloud.dev/mysql/gcpmysql/1",
			},
			want: func() *drvr.Config {
				cfg := drvr.NewConfig()
				cfg.AllowNativePasswords = true
				cfg.ParseTime = true
				cfg.Net = "gocloud.dev/mysql/gcpmysql/1"
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
				urlStr:     "gcpmysql://user:password@my-project-id/us-central1/my-instance-id/my-db?columnsWithAlias=true&parseTime=true",
				dialerName: "gocloud.dev/mysql/gcpmysql/1",
			},
			want: func() *drvr.Config {
				cfg := drvr.NewConfig()
				cfg.AllowNativePasswords = true
				cfg.ColumnsWithAlias = true
				cfg.ParseTime = true
				cfg.Net = "gocloud.dev/mysql/gcpmysql/1"
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
				urlStr:     "gcpmysql://user:password@my-project-id/us-central1/my-db",
				dialerName: "gocloud.dev/mysql/gcpmysql/1",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "DNSParseError",
			args: args{
				urlStr:     "gcpmysql://user:password@my-project-id/us-central1/my-instance-id/my-db?parseTime=nope",
				dialerName: "gocloud.dev/mysql/gcpmysql/1",
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
			got, err := configFromURL(u, tt.args.dialerName)
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
