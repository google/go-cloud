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

// Package sdserver provides the diagnostic hooks for a server using
// Stackdriver.
package sdserver // import "gocloud.dev/server/sdserver"

import (
	"context"
	"fmt"
	"os"

	"github.com/google/wire"
	gcpdetector "go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"gocloud.dev/gcp"
	"gocloud.dev/server"
	"gocloud.dev/server/requestlog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/oauth"
)

// Set is a Wire provider set that provides the diagnostic hooks for
// *server.Server given a GCP token source and a GCP project ID.
var Set = wire.NewSet(
	server.Set,
	NewTextMapPropagator,
	NewTraceSampler,
	NewTraceExporter,
	NewTraceProvider,
	wire.Bind(new(trace.TracerProvider), new(*sdktrace.TracerProvider)),
	NewMetricsReader,
	NewMeterProvider,
	wire.Bind(new(metric.MeterProvider), new(*sdkmetric.MeterProvider)),

	NewRequestLogger,
	wire.Bind(new(requestlog.Logger), new(*requestlog.StackdriverLogger)),
)

func NewResource(ctx context.Context) (*sdkresource.Resource, error) {
	res, err := sdkresource.New(ctx,
		sdkresource.WithDetectors(gcpdetector.NewDetector()),
		sdkresource.WithTelemetrySDK(),
		sdkresource.WithProcess(),
		sdkresource.WithOS(),
		sdkresource.WithContainer(),
		sdkresource.WithHost(),
	)
	if err != nil {
		return nil, err
	}

	return sdkresource.Merge(sdkresource.Default(), res)
}

func NewTextMapPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

// NewTraceSampler returns a new OpenTelemetry trace sampler.
func NewTraceSampler(ctx context.Context) sdktrace.Sampler {
	return sdktrace.AlwaysSample()
}

// NewTraceExporter returns a new OpenTelemetry gcp trace exporter.
func NewTraceExporter(projectID gcp.ProjectID) (sdktrace.SpanExporter, error) {
	ctx := context.Background()
	creds, err := oauth.NewApplicationDefault(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load application default credentials: %w", err)
	}
	exporter, err := otlptracegrpc.New(
		ctx,
		otlptracegrpc.WithEndpoint("telemetry.googleapis.com:443"),
		otlptracegrpc.WithDialOption(grpc.WithPerRPCCredentials(creds)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}
	return exporter, nil
}

// NewTraceProvider returns a new trace provider for our service to utilise.
//
// The second return value is a Wire cleanup function that calls Close on the provider,
func NewTraceProvider(ctx context.Context, exporter sdktrace.SpanExporter, sampler sdktrace.Sampler) (*sdktrace.TracerProvider, func(), error) {
	res, err := NewResource(ctx)
	if err != nil {
		return nil, nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sampler),
		sdktrace.WithResource(res),
	)
	return tp, func() { _ = tp.Shutdown(ctx) }, nil
}

// NewMetricsReader returns a new OpenTelemetry gcp metrics reader and exporter.
func NewMetricsReader(projectID gcp.ProjectID) (sdkmetric.Reader, error) {
	ctx := context.Background()
	exporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint("telemetry.googleapis.com:443"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}
	return sdkmetric.NewPeriodicReader(exporter), nil
}

// NewMeterProvider returns a new metric provider for our service to utilise.
//
// The second return value is a Wire cleanup function that calls Close on the provider.
func NewMeterProvider(ctx context.Context, reader sdkmetric.Reader) (*sdkmetric.MeterProvider, func(), error) {
	res, err := NewResource(ctx)
	if err != nil {
		return nil, nil, err
	}
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(reader),
	)
	return meterProvider, func() { _ = meterProvider.Shutdown(ctx) }, nil
}

// NewRequestLogger returns a request logger that sends entries to stdout.
func NewRequestLogger() *requestlog.StackdriverLogger {
	// For now, request logs are written to stdout and get picked up by fluentd.
	// This also works when running locally.
	return requestlog.NewStackdriverLogger(os.Stdout, func(e error) { fmt.Println(e) })
}
