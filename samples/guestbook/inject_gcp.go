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

//+build wireinject

package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	"github.com/google/wire"
	"gocloud.dev/blob"
	"gocloud.dev/blob/gcsblob"
	"gocloud.dev/gcp"
	"gocloud.dev/gcp/gcpcloud"
	"gocloud.dev/mysql/gcpmysql"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/gcpruntimeconfig"
	"gocloud.dev/server"
	pb "google.golang.org/genproto/googleapis/cloud/runtimeconfig/v1beta1"
)

// This file wires the generic interfaces up to Google Cloud Platform (GCP). It
// won't be directly included in the final binary, since it includes a Wire
// injector template function (setupGCP), but the declarations will be copied
// into wire_gen.go when Wire is run.

// setupGCP is a Wire injector function that sets up the application using GCP.
func setupGCP(ctx context.Context, flags *cliFlags) (*server.Server, func(), error) {
	// This will be filled in by Wire with providers from the provider sets in
	// wire.Build.
	wire.Build(
		gcpcloud.GCP,
		wire.Struct(new(gcpmysql.URLOpener), "CertSource"),
		applicationSet,
		gcpBucket,
		gcpMOTDVar,
		openGCPDatabase,
	)
	return nil, nil, nil
}

// gcpBucket is a Wire provider function that returns the GCS bucket based on
// the command-line flags.
func gcpBucket(ctx context.Context, flags *cliFlags, client *gcp.HTTPClient) (*blob.Bucket, func(), error) {
	b, err := gcsblob.OpenBucket(ctx, client, flags.bucket, nil)
	if err != nil {
		return nil, nil, err
	}
	return b, func() { b.Close() }, nil
}

// openGCPDatabase is a Wire provider function that connects to a GCP Cloud SQL
// MySQL database based on the command-line flags.
func openGCPDatabase(ctx context.Context, opener *gcpmysql.URLOpener, id gcp.ProjectID, flags *cliFlags) (*sql.DB, func(), error) {
	db, err := opener.OpenMySQLURL(ctx, &url.URL{
		Scheme: "gcpmysql",
		User:   url.UserPassword(flags.dbUser, flags.dbPassword),
		Host:   string(id),
		Path:   fmt.Sprintf("/%s/%s/%s", flags.cloudSQLRegion, flags.dbHost, flags.dbName),
	})
	if err != nil {
		return nil, nil, err
	}
	return db, func() { db.Close() }, nil
}

// gcpMOTDVar is a Wire provider function that returns the Message of the Day
// variable from Runtime Configurator.
func gcpMOTDVar(ctx context.Context, client pb.RuntimeConfigManagerClient, project gcp.ProjectID, flags *cliFlags) (*runtimevar.Variable, func(), error) {
	variableKey := gcpruntimeconfig.VariableKey(project, flags.runtimeConfigName, flags.motdVar)
	v, err := gcpruntimeconfig.OpenVariable(client, variableKey, runtimevar.StringDecoder, &gcpruntimeconfig.Options{
		WaitDuration: flags.motdVarWaitTime,
	})
	if err != nil {
		return nil, nil, err
	}
	return v, func() { v.Close() }, nil
}
