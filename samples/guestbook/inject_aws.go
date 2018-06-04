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

//+build wireinject

package main

import (
	"context"
	"time"

	awsclient "github.com/aws/aws-sdk-go/aws/client"
	"github.com/google/go-cloud/aws/awscloud"
	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/s3blob"
	"github.com/google/go-cloud/mysql/rdsmysql"
	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/paramstore"
	"github.com/google/go-cloud/wire"
)

// Configure these to your AWS project.
const (
	s3BucketName = "foo-bucket"

	rdsEndpoint = "myinstance.xyzzy.us-west-1.rds.amazonaws.com"
	rdsPassword = "xyzzy"

	paramstoreVar = "/guestbook/motd"
)

func setupAWS(ctx context.Context) (*app, func(), error) {
	panic(wire.Build(
		awscloud.AWS,
		appSet,
		awsBucket,
		awsMOTDVar,
		awsSQLParams,
	))
}

func awsBucket(ctx context.Context, cp awsclient.ConfigProvider) (*blob.Bucket, error) {
	return s3blob.NewBucket(ctx, cp, s3BucketName)
}

func awsSQLParams() *rdsmysql.Params {
	return &rdsmysql.Params{
		Endpoint: rdsEndpoint,
		Database: "guestbook",
		User:     "guestbook",
		Password: rdsPassword,
	}
}

func awsMOTDVar(ctx context.Context, client *paramstore.Client) (*runtimevar.Variable, error) {
	return client.NewVariable(ctx, paramstoreVar, runtimevar.StringDecoder, &paramstore.WatchOptions{
		WaitTime: 5 * time.Second,
	})
}
