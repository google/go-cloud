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
// ADOT collector.
package xrayserver // import "gocloud.dev/server/xrayserver"

import (
	"context"
	"fmt"
	ec2res "go.opentelemetry.io/contrib/detectors/aws/ec2"
	"go.opentelemetry.io/contrib/propagators/aws/xray"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"gocloud.dev/server"
	"gocloud.dev/server/requestlog"
	"google.golang.org/grpc"
	"os"

	"github.com/google/wire"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Set is a Wire provider set that provides the diagnostic hooks for
// *server.Server using aws specific formats outlined here
// https://aws-otel.github.io/docs/getting-started/go-sdk/manual-instr.
var Set = wire.NewSet(
	server.Set,
	TracesSet,
	MetricsSet,

	NewRequestLogger,
	wire.Bind(new(requestlog.Logger), new(*requestlog.NCSALogger)),
)

// TracesSet is a Wire provider set that provides the open telemetry trace provider given the exporter.
var TracesSet = wire.NewSet(
	NewTextMapPropagator,
	wire.Bind(new(propagation.TextMapPropagator), new(*xray.Propagator)),
	NewTraceSampler,
	NewTraceExporter,
	NewTraceProvider,
	wire.Bind(new(trace.TracerProvider), new(*sdktrace.TracerProvider)),
)

// MetricsSet is a Wire provider set that provides the open telemetry metrics provider given the exporter.
var MetricsSet = wire.NewSet(
	NewMetricsReader,
	NewMeterProvider,
	wire.Bind(new(metric.MeterProvider), new(*sdkmetric.MeterProvider)),
)

func NewResource(ctx context.Context) (*resource.Resource, error) {

	res, err := resource.New(ctx,
		resource.WithDetectors(ec2res.NewResourceDetector()),
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

func NewTextMapPropagator() *xray.Propagator {
	return &xray.Propagator{}
}

// NewTraceSampler returns a new OpenTelemetry trace sampler.
func NewTraceSampler() sdktrace.Sampler {
	return sdktrace.AlwaysSample()
}

func NewTraceExporter(ctx context.Context) (sdktrace.SpanExporter, error) {
	traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure(), otlptracegrpc.WithEndpoint("0.0.0.0:4317"), otlptracegrpc.WithDialOption(grpc.WithBlock()))
	if err != nil {
		return nil, err
	}
	return traceExporter, nil
}

// NewTraceProvider returns a new trace provider for our service to utilise.
//
// The second return value is a Wire cleanup function that calls Close on the provider.
func NewTraceProvider(ctx context.Context, exp sdktrace.SpanExporter, sampler sdktrace.Sampler) (*sdktrace.TracerProvider, func(), error) {

	res, err := NewResource(ctx)
	if err != nil {
		return nil, nil, err
	}

	idg := xray.NewIDGenerator()

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
		sdktrace.WithIDGenerator(idg),
	)

	return tp, func() { _ = tp.Shutdown(ctx) }, nil
}

func NewMetricsReader(ctx context.Context) (sdkmetric.Reader, error) {

	// Create and start new OTLP metric exporter
	metricExporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithInsecure(), otlpmetricgrpc.WithEndpoint("0.0.0.0:4317"), otlpmetricgrpc.WithDialOption(grpc.WithBlock()))
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
func NewRequestLogger() *requestlog.NCSALogger {
	return requestlog.NewNCSALogger(os.Stdout, func(e error) { _, _ = fmt.Fprintln(os.Stderr, e) })
}
