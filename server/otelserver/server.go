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

// Package otelserver provides the diagnostic hooks for a server using
// AWS X-Ray.
package otelserver // import "gocloud.dev/server/otelserver"

import (
	"context"
	"fmt"
	"github.com/google/wire"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
	"go.opentelemetry.io/otel/trace"
	"gocloud.dev/server"
	"gocloud.dev/server/requestlog"
	"os"
)

// Set is a Wire provider set that provides the diagnostic hooks for
// *server.Server. This set includes ServiceSet.
var Set = wire.NewSet(
	server.Set,
	OtelResourceSet,
	OtelTraceSet,
	OtelMetricsSet,
	OtelLogsSet,

	NewRequestLogger,
	wire.Bind(new(requestlog.Logger), new(*requestlog.NCSALogger)),
)

// OtelResourceSet is a Wire provider set that provides the open telemetry resource given the service name
var OtelResourceSet = wire.NewSet(
	NewResource,
	wire.Bind(new(resource.Resource), new(*resource.Resource)),
)

// OtelTraceSet is a Wire provider set that provides the open telemetry trace provider given the exporter
var OtelTraceSet = wire.NewSet(
	NewTraceProvider,
	wire.Bind(new(trace.TracerProvider), new(*sdktrace.TracerProvider)),

)

// OtelMetricsSet is a Wire provider set that provides the open telemetry metrics provider given the exporter
var OtelMetricsSet = wire.NewSet(
	NewMeterProvider,
	wire.Bind(new(metric.MeterProvider), new(*sdkmetric.MeterProvider)),
)

// OtelLogsSet is a Wire provider set that provides the open telemetry logs provider given the exporter
var OtelLogsSet = wire.NewSet(
	NewLoggerProvider,
	wire.Bind(new(otellog.LoggerProvider), new(*sdklog.LoggerProvider)),
)

func NewResource(serviceName, serviceVersion string) (*resource.Resource, error) {
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

// NewLoggerProvider returns a new logger provider for our service to utilise.
//
// The second return value is a Wire cleanup function that calls Close on the provider,
func NewLoggerProvider(ctx context.Context, res *resource.Resource, exporter sdklog.Exporter) (*sdklog.LoggerProvider, func()) {

	processor := sdklog.NewBatchProcessor(exporter)
	logProvider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(processor),
	)
	return logProvider, func() { _ = logProvider.Shutdown(context.TODO()) }
}

// NewRequestLogger returns a request logger that sends entries to stdout.
func NewRequestLogger() *requestlog.NCSALogger {
	return requestlog.NewNCSALogger(os.Stdout, func(e error) { fmt.Fprintln(os.Stderr, e) })
}
