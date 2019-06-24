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

// Read the documentation on Azure Database for MySql for more information.
// See https://docs.microsoft.com/en-us/azure/mysql/howto-configure-ssl.
// To run this test, create a MySQL instance using Azure Portal or Terraform.
// For Azure Portal, see https://docs.microsoft.com/en-us/azure/mysql/quickstart-create-mysql-server-database-using-azure-portal.
// For Terraform, see https://www.terraform.io/docs/providers/azurerm/r/mysql_database.html.
package azuremysql

import (
	"context"
	"fmt"
	"testing"

	"gocloud.dev/internal/testing/terraform"
	"gocloud.dev/mysql"
)

func TestURLOpener(t *testing.T) {
	// This test will be skipped unless the project is set up with Terraform.
	// Before running go test:
	//
	// 1. Install Azure CLI (az) (https://docs.microsoft.com/en-us/cli/azure/install-azure-cli-linux)
	// 2. Run "az login"
	// 3. terraform init
	// 4. terraform apply

	tfOut, err := terraform.ReadOutput(".")
	if err != nil || len(tfOut) == 0 {
		t.Skipf("Could not obtain harness info: %v", err)
	}

	serverName, _ := tfOut["servername"].Value.(string)
	username, _ := tfOut["username"].Value.(string)
	password, _ := tfOut["password"].Value.(string)
	databaseName, _ := tfOut["database"].Value.(string)

	ctx := context.Background()
	db, err := mysql.Open(ctx, fmt.Sprintf("azuremysql://%s:%s@%s/%s", username, password, serverName, databaseName))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Error("Ping: ", err)
	}
	if _, err = db.ExecContext(ctx, "CREATE TABLE tblTester (id INT NOT NULL, PRIMARY KEY(id))"); err != nil {
		t.Error("ExecContext: ", err)
	}
	if _, err = db.ExecContext(ctx, "DROP TABLE tblTester"); err != nil {
		t.Error("ExecContext: ", err)
	}
	if err := db.Close(); err != nil {
		t.Error("Close: ", err)
	}
}
