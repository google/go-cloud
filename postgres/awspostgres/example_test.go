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

package awspostgres_test

import (
	"context"
	"log"

	"gocloud.dev/postgres"
	_ "gocloud.dev/postgres/awspostgres"
)

func Example() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/postgres/awspostgres"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// Replace these with your actual settings.
	db, err := postgres.Open(ctx,
		"awspostgres://myrole:swordfish@example01.xyzzy.us-west-1.rds.amazonaws.com/testdb")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Use database in your program.
	_, _ = db.ExecContext(ctx, "CREATE TABLE foo (bar INT);")
}

func Example_iam() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/postgres/awspostgres"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// To use IAM authentication, omit the password from the URL.
	// The IAM user or role must be granted rds_iam permission in the database.
	db, err := postgres.Open(ctx,
		"awspostgres://iamuser@example01.xyzzy.us-west-1.rds.amazonaws.com:5432/testdb")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Use database in your program.
	if _, err := db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS foo (bar INT)"); err != nil {
		log.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO foo (bar) VALUES ($1)", 42); err != nil {
		log.Fatal(err)
	}
	var val int
	if err := db.QueryRowContext(ctx, "SELECT bar FROM foo LIMIT 1").Scan(&val); err != nil {
		log.Fatal(err)
	}
}
