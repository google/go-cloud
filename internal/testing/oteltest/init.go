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

package oteltest

import (
	"context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// configureTraceProvider sets up the global trace provider with the given exporter.
// It returns a function to shut down the exporter.
func configureTraceProvider(serviceName string, exporter sdktrace.SpanExporter, sampler sdktrace.Sampler, res *resource.Resource, asyncExport bool) (func(context.Context) error, error) {
	var err error
	if res == nil {
		res = resource.Default()
	}

	res, err = resource.Merge(
		res,
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return nil, err
	}

	if sampler == nil {
		sampler = sdktrace.AlwaysSample()
	}

	var exporterOpt sdktrace.TracerProviderOption
	if asyncExport {
		exporterOpt = sdktrace.WithSyncer(exporter)
	} else {
		exporterOpt = sdktrace.WithBatcher(exporter)

	}

	tp := sdktrace.NewTracerProvider(
		exporterOpt,
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Set the global trace provider.
	otel.SetTracerProvider(tp)

	// Set the global propagator to tracecontext (the default is no-op).
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp.Shutdown, nil
}

// configureMeterProvider sets up the given meter provider with the given exporter.
// It returns a function to collect and export metrics on demand, and a shutdown function.
func configureMeterProvider(serviceName string, reader sdkmetric.Reader, res *resource.Resource, views []sdkmetric.View) (func(context.Context) error, error) {
	var err error
	if res == nil {
		res = resource.Default()
	}

	res, err = resource.Merge(
		res,
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return nil, err
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(res),
		sdkmetric.WithView(views...),
	)

	// Set the global meter provider.
	otel.SetMeterProvider(mp)

	return func(ctx context.Context) error {
		return mp.Shutdown(ctx)
	}, nil
}
