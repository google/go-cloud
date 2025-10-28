// Copyright 2019-2025 The Go Cloud Development Kit Authors
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

package awsmysql_test

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/otel/attribute"
	"gocloud.dev/internal/testing/oteltest"
	"gocloud.dev/internal/testing/terraform"
	"gocloud.dev/mysql"
)

func TestOpenTelemetry(t *testing.T) {
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
	username, _ := tfOut["iam_db_username"].Value.(string)
	roleARN, _ := tfOut["iam_role_arn"].Value.(string)
	databaseName, _ := tfOut["database"].Value.(string)
	if endpoint == "" || username == "" || databaseName == "" {
		t.Fatalf("Missing one or more required Terraform outputs; got endpoint=%q iam_db_username=%q database=%q", endpoint, username, databaseName)
	}
	ctx := context.Background()

	// Setup the test exporter for both trace and metrics.
	te := oteltest.NewTestExporter(t, nil)
	defer te.Shutdown(ctx)

	// Open the database with otelsql.
	urlstr := fmt.Sprintf("awsmysql://%s@%s/%s?parseTime=true&aws_role_arn=%s",
		username, endpoint, databaseName, roleARN)
	t.Log("Connecting to:", urlstr)
	db, err := mysql.Open(ctx, urlstr)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	query := func() error {
		rows, err := db.QueryContext(ctx, `SELECT CURRENT_TIMESTAMP`)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		var currentTime time.Time
		for rows.Next() {
			err = rows.Scan(&currentTime)
			if err != nil {
				return err
			}
		}
		// Check for errors from iterating over rows
		if err = rows.Err(); err != nil {
			return err
		}
		slog.Info("Current time", "time", currentTime)
		return nil
	}
	if err = query(); err != nil {
		t.Error("QueryContext:", err)
	}

	spans := te.GetSpans().Snapshots()
	if !cmp.Equal(3, len(spans)) {
		t.Errorf("expected 3 spans, got %d: %v", len(spans), spans)
	}
	if !cmp.Equal("sql.connector.connect", spans[0].Name()) {
		t.Errorf("expected first span name to be sql.connector.connect, got %q", spans[0].Name())
	}
	if !cmp.Equal("sql.conn.query", spans[1].Name()) {
		t.Errorf("expected second span name to be sql.conn.query, got %q", spans[1].Name())
	} else {
		attrs := spans[1].Attributes()
		slog.Info("Span Attributes", "attributes", attrs)
		if !cmp.Equal(1, len(attrs)) {
			t.Errorf("expected 1 attribute, got %d: %v", len(attrs), attrs)
		}
		if !cmp.Equal(attribute.Key("db.statement"), attrs[0].Key) {
			t.Errorf("expected attribute key to be db.statement, got %q", attrs[0].Key)
		}
		if !cmp.Equal("SELECT CURRENT_TIMESTAMP", attrs[0].Value.AsString()) {
			t.Errorf("expected attribute value to be 'SELECT CURRENT_TIMESTAMP', got %q", attrs[0].Value.AsString())
		}
	}
	if !cmp.Equal("sql.rows", spans[2].Name()) {
		t.Errorf("expected second span name to be sql.rows, got %q", spans[2].Name())
	} else {
		attrs := spans[2].Attributes()
		slog.Info("Span Attributes", "attributes", attrs)
		if !cmp.Equal(0, len(attrs)) {
			t.Errorf("expected 0 attribute, got %d: %v", len(attrs), attrs)
		}
	}
}
