// Copyright 2018 The Go Cloud Authors
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

package cloudpostgres_test

import (
	"context"

	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/gcp/cloudsql"
	"github.com/google/go-cloud/postgres/cloudpostgres"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func Example() {
	ctx := context.Background()
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		panic(err)
	}
	authClient := gcp.HTTPClient{Client: *oauth2.NewClient(ctx, creds.TokenSource)}
	db, err := cloudpostgres.Open(ctx, cloudsql.NewCertSource(&authClient), &cloudpostgres.Params{
		// Replace these with your actual settings.
		ProjectID: "example-project",
		Region:    "us-central1",
		Instance:  "my-instance01",
		User:      "myrole",
		Database:  "test",
	})
	if err != nil {
		panic(err)
	}

	// Use database in your program.
	db.Exec("CREATE TABLE foo (bar INT);")
}
