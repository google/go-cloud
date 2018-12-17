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

//+build wireinject

package main

import (
	"context"

	awsclient "github.com/aws/aws-sdk-go/aws/client"
	"github.com/google/wire"
	"gocloud.dev/aws/awscloud"
	"gocloud.dev/blob"
	"gocloud.dev/blob/s3blob"
	"gocloud.dev/mysql/rdsmysql"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/paramstore"
)

// This file wires the generic interfaces up to Amazon Web Services (AWS). It
// won't be directly included in the final binary, since it includes a Wire
// injector template function (setupAWS), but the declarations will be copied
// into wire_gen.go when Wire is run.

// setupAWS is a Wire injector function that sets up the application using AWS.
func setupAWS(ctx context.Context, flags *cliFlags) (*application, func(), error) {
	// This will be filled in by Wire with providers from the provider sets in
	// wire.Build.
	wire.Build(
		awscloud.AWS,
		rdsmysql.Open,
		applicationSet,
		awsBucket,
		awsMOTDVar,
		awsSQLParams,
	)
	return nil, nil, nil
}

// awsBucket is a Wire provider function that returns the S3 bucket based on the
// command-line flags.
func awsBucket(ctx context.Context, cp awsclient.ConfigProvider, flags *cliFlags) (*blob.Bucket, error) {
	return s3blob.OpenBucket(ctx, cp, flags.bucket, nil)
}

// awsSQLParams is a Wire provider function that returns the RDS SQL connection
// parameters based on the command-line flags. Other providers inside
// awscloud.AWS use the parameters to construct a *sql.DB.
func awsSQLParams(flags *cliFlags) *rdsmysql.Params {
	return &rdsmysql.Params{
		Endpoint: flags.dbHost,
		Database: flags.dbName,
		User:     flags.dbUser,
		Password: flags.dbPassword,
	}
}

// awsMOTDVar is a Wire provider function that returns the Message of the Day
// variable from SSM Parameter Store.
func awsMOTDVar(ctx context.Context, sess awsclient.ConfigProvider, flags *cliFlags) (*runtimevar.Variable, error) {
	return paramstore.NewVariable(sess, flags.motdVar, runtimevar.StringDecoder, &paramstore.Options{
		WaitDuration: flags.motdVarWaitTime,
	})
}
