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

// Package xrayserver provides the diagnostic hooks for a server using
// AWS X-Ray.
package xrayserver // import "gocloud.dev/server/xrayserver"

import (
	"context"
	"fmt"
	"os"

	"github.com/google/wire"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	"gocloud.dev/server"
	"gocloud.dev/server/requestlog"
)

// Set is a Wire provider set that provides the diagnostic hooks for
// *server.Server. This set includes ServiceSet.
var Set = wire.NewSet(
	NewAwsTraceExporter,
	server.Set,
	NewRequestLogger,
	wire.Bind(new(requestlog.Logger), new(*requestlog.NCSALogger)),
)

// NewAwsTraceExporter returns a new OpenTelemetry exporter configured for AWS X-Ray.
//
// The second return value is a Wire cleanup function that calls Shutdown
// on the exporter, ignoring the error.
func NewAwsTraceExporter(option ...otlptracegrpc.Option) (*trace.TracerProvider, error) {
	ctx := context.Background()

	// Create a resource with basic information
	res, err := resource.New(ctx,
		resource.WithOS(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			attribute.String("service.name", "go-cloud-server"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create OTLP exporter configured for AWS X-Ray
	// AWS X-Ray typically uses the OpenTelemetry Collector with the AWS X-Ray exporter
	client := otlptracegrpc.NewClient(
		otlptracegrpc.WithEndpoint("0.0.0.0:4317"), // Default OTLP gRPC endpoint where collector should be running
		otlptracegrpc.WithInsecure(),               // For production, consider using TLS
	)
	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Create a tracer provider with the exporter
	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(res),
		trace.WithSampler(trace.AlwaysSample()),
	)

	// Set the global tracer provider
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp, nil
}

// NewRequestLogger returns a request logger that sends entries to stdout.
func NewRequestLogger() *requestlog.NCSALogger {
	return requestlog.NewNCSALogger(os.Stdout, func(e error) { fmt.Fprintln(os.Stderr, e) })
}
