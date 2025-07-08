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
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/trace"
	"gocloud.dev/server"
	"os"

	gcpmex "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric"
	gcptex "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	gcppropagator "github.com/GoogleCloudPlatform/opentelemetry-operations-go/propagator"
	"github.com/google/wire"
	gcpres "go.opentelemetry.io/contrib/detectors/gcp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"gocloud.dev/gcp"
	"gocloud.dev/server/requestlog"
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

func NewResource(ctx context.Context) (*resource.Resource, error) {

	res, err := resource.New(ctx,
		resource.WithDetectors(gcpres.NewDetector()),
		resource.WithTelemetrySDK(),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithContainer(),
		resource.WithHost(),
	)
	if err != nil {
		return nil, err
	}

	return resource.Merge(resource.Default(), res)
}

func NewTextMapPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		gcppropagator.CloudTraceOneWayPropagator{},
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
	exporter, err := gcptex.New(gcptex.WithProjectID(string(projectID)))
	if err != nil {
		return nil, err
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
	metricExporter, err := gcpmex.New(gcpmex.WithProjectID(string(projectID)))
	if err != nil {
		return nil, err
	}

	return sdkmetric.NewPeriodicReader(metricExporter), nil
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
