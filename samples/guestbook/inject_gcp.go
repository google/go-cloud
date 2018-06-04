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

	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/gcsblob"
	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/gcp/gcpcloud"
	"github.com/google/go-cloud/mysql/cloudmysql"
	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/runtimeconfigurator"
	"github.com/google/go-cloud/wire"
)

// Configure these to your GCP project.
const (
	gcsBucketName = "foo-guestbook-bucket"

	cloudSQLRegion   = "us-central1"
	cloudSQLInstance = "guestbook"

	runtimeConfigName = "guestbook"
	runtimeConfigVar  = "motd"
)

func setupGCP(ctx context.Context) (*app, func(), error) {
	panic(wire.Build(
		gcpcloud.GCP,
		appSet,
		gcpBucket,
		gcpMOTDVar,
		gcpSQLParams,
	))
}

func gcpBucket(ctx context.Context, ts gcp.TokenSource) (*blob.Bucket, error) {
	return gcsblob.New(ctx, gcsBucketName, &gcsblob.BucketOptions{
		TokenSource: ts,
	})
}

func gcpSQLParams(id gcp.ProjectID) *cloudmysql.Params {
	return &cloudmysql.Params{
		ProjectID: string(id),
		Region:    cloudSQLRegion,
		Instance:  cloudSQLInstance,
		Database:  "guestbook",
		User:      "guestbook",
	}
}

func gcpMOTDVar(ctx context.Context, client *runtimeconfigurator.Client, project gcp.ProjectID) (*runtimevar.Variable, func(), error) {
	name := runtimeconfigurator.ResourceName{
		ProjectID: string(project),
		Config:    runtimeConfigName,
		Variable:  runtimeConfigVar,
	}
	v, err := client.NewVariable(ctx, name, runtimevar.StringDecoder, &runtimeconfigurator.WatchOptions{
		WaitTime: 5 * time.Second,
	})
	if err != nil {
		return nil, nil, err
	}
	return v, func() { v.Close() }, nil
}
