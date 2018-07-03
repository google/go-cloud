// Copyright 2018 Google LLC
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

package rdsmysql_test

import (
	"context"

	"github.com/google/go-cloud/mysql/rdsmysql"
)

func Example() {
	ctx := context.Background()
	db, cleanup, err := rdsmysql.Open(ctx, new(rdsmysql.CertFetcher), &rdsmysql.Params{
		// Replace these with your actual settings.
		Endpoint: "example01.xyzzy.us-west-1.rds.amazonaws.com",
		User:     "myrole",
		Password: "swordfish",
		Database: "test",
	})
	if err != nil {
		panic(err)
	}
	defer cleanup()

	// Use database in your program.
	db.ExecContext(ctx, "CREATE TABLE foo (bar INT);")
}
