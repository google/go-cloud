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

//go:build wireinject
// +build wireinject

package main

import (
	"context"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	azcontainer "github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/google/wire"
	"gocloud.dev/blob"
	"gocloud.dev/blob/azureblob"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/blobvar"
	"gocloud.dev/server"
	"gocloud.dev/server/requestlog"
)

// This file wires the generic interfaces up to Microsoft Azure. It
// won't be directly included in the final binary, since it includes a Wire
// injector template function (setupAzure), but the declarations will be copied
// into wire_gen.go when Wire is run.

// setupAzure is a Wire injector function that sets up the application using
// Azure.
func setupAzure(ctx context.Context, flags *cliFlags) (*server.Server, func(), error) {
	// This will be filled in by Wire with providers from the provider sets in
	// wire.Build.
	wire.Build(
		wire.InterfaceValue(new(requestlog.Logger), requestlog.Logger(nil)),
		azureblob.NewDefaultServiceURLOptions,
		azureblob.NewDefaultClient,
		azureblob.NewServiceURL,
		applicationSet,
		bucketName,
		azureBucket,
		azureMOTDVar,
		server.Set,
		wire.InterfaceValue(new(propagation.TextMapPropagator), propagation.TextMapPropagator(nil)),
		wire.InterfaceValue(new(trace.TracerProvider), trace.TracerProvider(nil)),
		wire.InterfaceValue(new(metric.MeterProvider), metric.MeterProvider(nil)),
		dialLocalSQL,
	)
	return nil, nil, nil
}

func bucketName(flags *cliFlags) azureblob.ContainerName {
	return azureblob.ContainerName(flags.bucket)
}

// azureBucket is a Wire provider function that returns the Azure bucket based
// on the command-line flags.
func azureBucket(ctx context.Context, client *azcontainer.Client, flags *cliFlags) (*blob.Bucket, func(), error) {
	b, err := azureblob.OpenBucket(ctx, client, nil)
	if err != nil {
		return nil, nil, err
	}
	return b, func() { b.Close() }, nil
}

// azureMOTDVar is a Wire provider function that returns the Message of the Day
// variable read from a blob stored in Azure.
func azureMOTDVar(ctx context.Context, b *blob.Bucket, flags *cliFlags) (*runtimevar.Variable, error) {
	return blobvar.OpenVariable(b, flags.motdVar, runtimevar.StringDecoder, &blobvar.Options{
		WaitDuration: flags.motdVarWaitTime,
	})
}
