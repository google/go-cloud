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

package main

import (
	"context"
	"github.com/google/wire"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/propagators/autoprop"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// otelTracesProviderSet is a Wire provider set that provides the open telemetry trace provider
var otelTracesProviderSet = wire.NewSet(
	NewTraceProvider,
	wire.Bind(new(trace.TracerProvider), new(*sdktrace.TracerProvider)),
)

// otelMetricsProviderSet is a Wire provider set that provides the open telemetry metrics provider
var otelMetricsProviderSet = wire.NewSet(
	NewMeterProvider,
	wire.Bind(new(metric.MeterProvider), new(*sdkmetric.MeterProvider)),
)

// OtelLogsSet is a Wire provider set that provides the open telemetry logs provider given the exporter
var OtelLogsSet = wire.NewSet(
	NewLogsExporter,
	NewLoggerProvider,
	wire.Bind(new(log.LoggerProvider), new(*sdklog.LoggerProvider)),
)

func newResource() *resource.Resource {
	return resource.Default()
}

func newPropagationTextMap() propagation.TextMapPropagator {
	return autoprop.NewTextMapPropagator()
}

// newTraceExporter returns a new OpenTelemetry gcp trace exporter.
func newTraceExporter(ctx context.Context) (sdktrace.SpanExporter, error) {

	traceExporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return nil, err
	}

	return traceExporter, nil
}

// newTraceSampler returns a new OpenTelemetry trace sampler.
func newTraceSampler(ctx context.Context) sdktrace.Sampler {
	return sdktrace.AlwaysSample()
}

// NewTraceProvider returns a new trace provider for our service to utilise.
//
// The second return value is a Wire cleanup function that calls Close on the provider,
func NewTraceProvider(ctx context.Context, exporter sdktrace.SpanExporter, sampler sdktrace.Sampler) (*sdktrace.TracerProvider, func()) {

	res := newResource()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sampler),
		sdktrace.WithResource(res),
	)

	return tp, func() { _ = tp.Shutdown(ctx) }
}

// newMetricsReader returns a new OpenTelemetry gcp metrics exporter.
func newMetricsReader(ctx context.Context) (sdkmetric.Reader, error) {
	// Create and start new OTLP metric exporter
	metricReader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return nil, err
	}

	return metricReader, nil
}

// NewMeterProvider returns a new metric provider for our service to utilise.
//
// The second return value is a Wire cleanup function that calls Close on the provider,
func NewMeterProvider(ctx context.Context, reader sdkmetric.Reader) (*sdkmetric.MeterProvider, func()) {

	res := newResource()

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(reader),
	)
	return meterProvider, func() { _ = meterProvider.Shutdown(ctx) }
}

// NewLogsExporter returns a new OpenTelemetry gcp metrics exporter.
func NewLogsExporter(ctx context.Context) (sdklog.Exporter, error) {
	// Create and start new OTLP metric exporter
	logsExporter, err := autoexport.NewLogExporter(ctx)
	if err != nil {
		return nil, err
	}

	return logsExporter, nil
}

// NewLoggerProvider returns a new logger provider for our service to utilise.
//
// The second return value is a Wire cleanup function that calls Close on the provider,
func NewLoggerProvider(ctx context.Context, res *resource.Resource, exporter sdklog.Exporter) (*sdklog.LoggerProvider, func(), error) {

	var err error
	if exporter == nil {
		exporter, err = autoexport.NewLogExporter(ctx)
		if err != nil {
			return nil, nil, err
		}
	}

	processor := sdklog.NewBatchProcessor(exporter)
	logProvider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(processor),
	)
	return logProvider, func() { _ = logProvider.Shutdown(context.TODO()) }, nil
}
