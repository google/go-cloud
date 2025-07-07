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
	"go.opentelemetry.io/contrib/detectors/aws/ec2"
	"go.opentelemetry.io/otel/sdk/resource"

	"go.opencensus.io/trace"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
	"gocloud.dev/server/otel"

	"github.com/google/wire"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"gocloud.dev/server/requestlog"
)

// Set is a Wire provider set that provides the diagnostic hooks for
// *server.Server given a GCP token source and a GCP project ID.
var Set = wire.NewSet(
	otel.Set,
	NewTraceExporter,
	wire.Bind(new(trace.Exporter), new(*sdktrace.SpanExporter)),
	NewMetricsExporter,
	wire.Bind(new(metric.MeterProvider), new(*sdkmetric.MeterProvider)),
	NewRequestLogger,
	wire.Bind(new(requestlog.Logger), new(*requestlog.StackdriverLogger)),
)

// ResourceSet is a Wire provider set that provides the open telemetry resource given the service name
var ResourceSet = wire.NewSet(
	NewResource,
	wire.Bind(new(resource.Resource), new(*resource.Resource)),
)

// TracesSet is a Wire provider set that provides the open telemetry trace provider given the exporter
var TracesSet = wire.NewSet(
	NewTraceExporter,
	wire.Bind(new(sdktrace.SpanExporter), new(*sdktrace.SpanExporter)),
	NewTraceProvider,
	wire.Bind(new(trace.TracerProvider), new(*sdktrace.TracerProvider)),

)

// MetricsSet is a Wire provider set that provides the open telemetry metrics provider given the exporter
var MetricsSet = wire.NewSet(
	NewMetricsExporter,
	wire.Bind(new(sdkmetric.Exporter), new(*sdkmetric.Exporter)),
	NewMeterProvider,
	wire.Bind(new(metric.MeterProvider), new(*sdkmetric.MeterProvider)),
)

func NewResource() (*resource.Resource, error) {

	// Instantiate a new EC2 Resource detector
	ec2ResourceDetector := ec2.NewResourceDetector()
	resource, err := ec2ResourceDetector.Detect(context.Background())

	return resource.Merge(resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		))
}

// NewTraceProvider returns a new trace provider for our service to utilise.
//
// The second return value is a Wire cleanup function that calls Close on the provider,
func NewTraceProvider(res *resource.Resource, exp sdktrace.SpanExporter) (*sdktrace.TracerProvider, func()) {

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)

	return tp, func() { _ = tp.Shutdown(context.TODO()) }
}

// NewMeterProvider returns a new metric provider for our service to utilise.
//
// The second return value is a Wire cleanup function that calls Close on the provider,
func NewMeterProvider(res *resource.Resource, exporter sdkmetric.Exporter, readerOpts ...sdkmetric.PeriodicReaderOption) (*sdkmetric.MeterProvider, func()) {

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter, readerOpts...)),
	)
	return meterProvider, func() { _ = meterProvider.Shutdown(context.TODO()) }
}
