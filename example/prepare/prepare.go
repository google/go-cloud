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

package main

import (
	"context"
	"flag"

	"golang.org/x/oauth2/google"
	sqladmin "google.golang.org/api/sqladmin/v1beta4"

	"github.com/vsekhar/go-cloud/log"
)

var ctx = context.Background()
var platform = flag.String("platform", "", "platform to prepare (one of AWS or GCP)")
var platforms = map[string]func(){
	"gcp": prepareGCP,
	"aws": prepareAWS,
}

const dbInstance = "go-cloud-demo-db-instance"
const dbName = "go-cloud-demo-db"

func main() {
	if *platform == "" {
		log.Print(ctx, "Please specify --platform=<platform>")
	}
	f, ok := platforms[*platform]
	if !ok {
		log.Fatalf(ctx, "unknown platform: %s", *platform)
	}
	f()
}

func prepareGCP() {
	creds, err := google.FindDefaultCredentials(ctx)
	if err != nil {
		log.Fatal(ctx, "failed to get default GCP credentials")
	}
	project := creds.ProjectID

	// Cloud SQL
	client, err := google.DefaultClient(ctx)
	if err != nil {
		log.Printf(ctx, "Failed to get client: %s", err)
	}
	sqladminSvc, err := sqladmin.New(client)
	dbSvc := sqladminSvc.Databases

}

func prepareAWS() {

}
