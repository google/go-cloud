// Copyright 2019 The Go Cloud Development Kit Authors
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

package mysql

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"gocloud.dev/internal/testing/terraform"
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
	endpoint, _ := tfOut["endpoint"].Value.(string)
	username, _ := tfOut["username"].Value.(string)
	password, _ := tfOut["password"].Value.(string)
	databaseName, _ := tfOut["database"].Value.(string)
	if endpoint == "" || username == "" || databaseName == "" {
		t.Fatalf("Missing one or more required Terraform outputs; got endpoint=%q username=%q database=%q", endpoint, username, databaseName)
	}

	ctx := context.Background()
	urlstr := fmt.Sprintf("%s://%s:%s@%s/%s", Scheme, username, password, endpoint, databaseName)
	t.Log("Connecting to:", urlstr)
	db, err := Open(ctx, urlstr)
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

func TestConfigFromURL(t *testing.T) {
	for _, host := range []string{
		"localhost",
		"tcp(localhost)",
	} {
		urlstr := "mysql://user:password@" + host + "/db?parseTime=true&interpolateParams=true"
		u, err := url.Parse(urlstr)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := ConfigFromURL(u)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := cfg.User, "user"; got != want {
			t.Errorf(`User = %q; want %q`, got, want)
		}
		if got, want := cfg.Passwd, "password"; got != want {
			t.Errorf(`Passwd = %q; want %q`, got, want)
		}
		if got, want := cfg.Net, "tcp"; got != want {
			t.Errorf(`Net = %q; want %q`, got, want)
		}
		if got, want := cfg.Addr, "localhost"; got != want {
			t.Errorf(`Addr = %q; want %q`, got, want)
		}
		if !cfg.ParseTime {
			t.Error("ParseTime = false; want true")
		}
		if !cfg.InterpolateParams {
			t.Error("InterpolateParams = false; want true")
		}
	}
}
