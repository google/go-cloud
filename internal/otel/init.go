// Copyright 2025 The Go Cloud Development Kit Authors
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

package otel

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

// ConfigureTraceProvider sets up the global trace provider with the given exporter.
// It returns a function to shut down the exporter.
func ConfigureTraceProvider(ctx context.Context, exporter sdktrace.SpanExporter, serviceName string) (func(context.Context) error, error) {
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	// Set the global trace provider
	otel.SetTracerProvider(tp)

	// Set the global propagator to tracecontext (the default is no-op)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp.Shutdown, nil
}

// TracerForPackage returns a tracer for the given package using the global provider.
func TracerForPackage(pkg string) trace.Tracer {
	return otel.Tracer(pkg)
}

// ConfigureMeterProvider sets up the given meter provider with the given exporter.
// It returns a function to collect and export metrics on demand, and a shutdown function.
func ConfigureMeterProvider(ctx context.Context, exporter sdkmetric.Exporter, serviceName string) (func(context.Context) error, func(context.Context) error, error) {
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return nil, nil, err
	}

	// Create a periodic reader with the exporter
	reader := sdkmetric.NewPeriodicReader(
		exporter,
		sdkmetric.WithInterval(60*time.Second),
	)

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(res),
	)

	// Set the global meter provider
	otel.SetMeterProvider(mp)

	// Function to force collection and export of metrics
	forceCollect := func(ctx context.Context) error {
		// Periodic readers have ForceFlush method we can use
		return reader.ForceFlush(ctx)
	}

	return forceCollect, mp.Shutdown, nil
}

// MeterForPackage returns a meter for the given package using the global provider.
func MeterForPackage(pkg string) metric.Meter {
	return otel.Meter(pkg)
}
