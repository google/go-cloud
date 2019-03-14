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

package azuremysql

import (
	"context"
	"strconv"
	"testing"

	"gocloud.dev/internal/testing/terraform"
)

func TestOpen(t *testing.T) {
	tfOut, err := terraform.ReadOutput(".")
	if err != nil {
		t.Skipf("Could not obtain harness info: %v", err)
	}

	serverName, _ := tfOut["serverName"].Value.(string)
	port, _ := tfOut["port"].Value.(string)
	username, _ := tfOut["username"].Value.(string)
	password, _ := tfOut["password"].Value.(string)
	databaseName, _ := tfOut["database"].Value.(string)
	if serverName == "" || port == "" || username == "" || databaseName == "" {
		t.Fatalf("Missing one or more required Terraform outputs; got servername=%q port=%q username=%q database=%q", serverName, port, username, databaseName)
	}
	portNum, err := strconv.ParseInt(port, 0, 32)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	acf := NewAzureCertFetcher("")
	db, _, err := Open(ctx, acf, &Params{
		ServerName: serverName,
		Port:       portNum,
		User:       username,
		Password:   password,
		Database:   databaseName,
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
