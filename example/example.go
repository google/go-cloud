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
	"io/ioutil"

	"github.com/vsekhar/go-cloud/config"
	"github.com/vsekhar/go-cloud/database/sql"
	"github.com/vsekhar/go-cloud/io"
	"github.com/vsekhar/go-cloud/log"
)

var configName = flag.String("config", "", "configuration service and name, e.g. 'local://config.json'")
var ctx = context.Background()

func main() {
	flag.Parse()

	// Load a configuration from specified config source
	log.Printf(ctx, "Using configuration: %s", *configName)
	cfg, _, err := config.Get(ctx, *configName)
	if err != nil {
		log.Fatal(ctx, err)
	}
	dbName, err := cfg.Get("go-cloud-demo-db")
	if err == config.NotFound {
		log.Fatalf(ctx, "no 'db' param found in config: %s", *configName)
	}
	if err != nil {
		log.Fatal(ctx, err)
	}

	// Open a database identified in the configuration
	log.Printf(ctx, "Opening database: %s", dbName)
	db, err := sql.Open(dbName)
	if err != nil {
		log.Fatal(ctx, err)
	}
	rows, err := db.Query("SELECT blob_name FROM blobs WHERE name = 'hellomessage' LIMIT 1;")
	if err != nil {
		log.Fatal(ctx, err)
	}
	defer rows.Close()
	var blobName string
	for rows.Next() {
		if err := rows.Scan(&blobName); err != nil {
			log.Fatal(ctx, err)
		}
	}

	// Open a blob identified in the database and print its contents
	b, err := io.NewBlob(ctx, blobName)
	if err != nil {
		log.Fatal(ctx, err)
	}
	r, err := b.NewReader(ctx)
	if err != nil {
		log.Fatal(ctx, err)
	}
	msg, err := ioutil.ReadAll(r)
	if err != nil {
		log.Fatal(ctx, err)
	}
	log.Print(ctx, string(msg))
}
