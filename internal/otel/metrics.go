// Copyright 2019-2025 The Go Cloud Development Kit Authors
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

// Package otel supports OpenTelemetry tracing and metrics for the Go Cloud Development Kit.
package otel

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

// MetricSet contains the standard metrics used by Go CDK APIs.
type MetricSet struct {
	Latency        metric.Float64Histogram
	CompletedCalls metric.Int64Counter
	BytesRead      metric.Int64Counter
	BytesWritten   metric.Int64Counter
}

// NewMetricSet creates a standard set of metrics for a Go CDK package.
func NewMetricSet(ctx context.Context, pkg string) (*MetricSet, error) {
	meter := otel.GetMeterProvider().Meter(pkg)

	latency, err := meter.Float64Histogram(
		pkg+".latency",
		metric.WithDescription("Latency of method call in milliseconds"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create latency metric: %w", err)
	}

	completedCalls, err := meter.Int64Counter(
		pkg+".completed_calls",
		metric.WithDescription("Count of method calls"),
		metric.WithUnit("{call}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create completed_calls metric: %w", err)
	}

	bytesRead, err := meter.Int64Counter(
		pkg+".bytes_read",
		metric.WithDescription("Number of bytes read"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create bytes_read metric: %w", err)
	}

	bytesWritten, err := meter.Int64Counter(
		pkg+".bytes_written",
		metric.WithDescription("Number of bytes written"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create bytes_written metric: %w", err)
	}

	return &MetricSet{
		Latency:        latency,
		CompletedCalls: completedCalls,
		BytesRead:      bytesRead,
		BytesWritten:   bytesWritten,
	}, nil
}

// InitMetrics initializes metrics with an OTLP exporter.
func InitMetrics(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	// Create a resource with service information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create the OTLP exporter
	exporter, err := otlpmetricgrpc.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	// Create reader for the exporter
	reader := sdkmetric.NewPeriodicReader(exporter)

	// Create meter provider
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(res),
	)

	// Set the global meter provider
	otel.SetMeterProvider(meterProvider)

	// Return shutdown function
	return func(ctx context.Context) error {
		return meterProvider.Shutdown(ctx)
	}, nil
}

// RecordLatency records a latency measurement with standard attributes.
func RecordLatency(ctx context.Context, latencyMeter metric.Float64Histogram, method, provider, status string, duration time.Duration) {
	if latencyMeter == nil {
		return
	}

	attrs := []attribute.KeyValue{
		MethodKey.String(method),
	}

	if provider != "" {
		attrs = append(attrs, ProviderKey.String(provider))
	}

	if status != "" {
		attrs = append(attrs, StatusKey.String(status))
	}

	latencyMeter.Record(ctx, float64(duration.Nanoseconds())/1e6, metric.WithAttributes(attrs...))
}

// RecordCount records a count with standard attributes.
func RecordCount(ctx context.Context, counter metric.Int64Counter, method, provider, status string, count int64) {
	if counter == nil {
		return
	}

	attrs := []attribute.KeyValue{
		MethodKey.String(method),
	}

	if provider != "" {
		attrs = append(attrs, ProviderKey.String(provider))
	}

	if status != "" {
		attrs = append(attrs, StatusKey.String(status))
	}

	counter.Add(ctx, count, metric.WithAttributes(attrs...))
}

// RecordBytesRead records bytes read with provider attribute.
func RecordBytesRead(ctx context.Context, counter metric.Int64Counter, provider string, n int64) {
	if counter == nil || n <= 0 {
		return
	}

	var attrs []attribute.KeyValue
	if provider != "" {
		attrs = append(attrs, ProviderKey.String(provider))
	}

	counter.Add(ctx, n, metric.WithAttributes(attrs...))
}

// RecordBytesWritten records bytes written with provider attribute.
func RecordBytesWritten(ctx context.Context, counter metric.Int64Counter, provider string, n int64) {
	if counter == nil || n <= 0 {
		return
	}

	var attrs []attribute.KeyValue
	if provider != "" {
		attrs = append(attrs, ProviderKey.String(provider))
	}

	counter.Add(ctx, n, metric.WithAttributes(attrs...))
}
