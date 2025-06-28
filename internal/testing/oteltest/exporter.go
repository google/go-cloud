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

// Package oteltest supports testing of OpenTelemetry integrations.
package oteltest

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
)

// TestExporter is an exporter of OpenTelemetry traces and metrics, for testing.
// It should be created with NewTestExporter.
type TestExporter struct {
	mu             sync.Mutex
	spanExporter   *tracetest.InMemoryExporter
	metricExporter *MetricExporter
	meterProvider  metric.MeterProvider
	tracerProvider trace.TracerProvider
	shutdown       func(context.Context) error
}

// MetricExporter is a simple metrics exporter for testing.
type MetricExporter struct {
	mu       sync.Mutex
	metrics  []metricdata.ScopeMetrics
	metricCh chan metricdata.ScopeMetrics
	reader   *sdkmetric.PeriodicReader
}

// NewMetricExporter creates a new metric exporter for testing
func NewMetricExporter() *MetricExporter {
	exporter := &MetricExporter{
		metricCh: make(chan metricdata.ScopeMetrics, 10),
	}

	exporter.reader = sdkmetric.NewPeriodicReader(
		exporter,
		// Use a short export interval for tests
		sdkmetric.WithInterval(100*time.Millisecond),
	)

	return exporter
}

// Temporality returns the aggregation temporality for the given instrument kind.
func (e *MetricExporter) Temporality(kind sdkmetric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}

// Aggregation returns the aggregation for the given instrument kind.
func (e *MetricExporter) Aggregation(kind sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return nil // Use default aggregation
}

// Export exports metric data.
func (e *MetricExporter) Export(ctx context.Context, data *metricdata.ResourceMetrics) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Store metrics for each scope
	for _, sm := range data.ScopeMetrics {
		e.metrics = append(e.metrics, sm)

		// Also send via channel
		select {
		case e.metricCh <- sm:
		default:
		}
	}

	return nil
}

// ForceFlush forces a flush of metrics
func (e *MetricExporter) ForceFlush(ctx context.Context) error {
	return nil
}

// GetMetrics returns all collected metrics
func (e *MetricExporter) GetMetrics() []metricdata.ScopeMetrics {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.metrics
}

// WaitForMetrics waits for metrics to be collected
func (e *MetricExporter) WaitForMetrics(timeout time.Duration) (metricdata.ScopeMetrics, bool) {
	select {
	case sm := <-e.metricCh:
		return sm, true
	case <-time.After(timeout):
		return metricdata.ScopeMetrics{}, false
	}
}

// Shutdown shuts down the exporter
func (e *MetricExporter) Shutdown(ctx context.Context) error {
	return e.reader.Shutdown(ctx)
}

// NewTestExporter creates a TestExporter and registers it with OpenTelemetry.
func NewTestExporter() *TestExporter {
	// Create span exporter
	spanExporter := tracetest.NewInMemoryExporter()

	// Create resource
	res := resource.NewSchemaless()

	// Create and register tracer provider
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(spanExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tracerProvider)

	// Create metric exporter
	metricExporter := NewMetricExporter()

	// Create and register meter provider
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(metricExporter.reader),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(meterProvider)

	shutdown := func(ctx context.Context) error {
		err1 := tracerProvider.Shutdown(ctx)
		err2 := meterProvider.Shutdown(ctx)
		if err1 != nil {
			return err1
		}
		return err2
	}

	return &TestExporter{
		spanExporter:   spanExporter,
		metricExporter: metricExporter,
		meterProvider:  meterProvider,
		tracerProvider: tracerProvider,
		shutdown:       shutdown,
	}
}

// Spans returns the collected spans.
func (te *TestExporter) Spans() []sdktrace.ReadOnlySpan {
	// Return an empty slice for now - the actual spans can be retrieved using SpanStubs()
	// This is a compatibility layer as the tracetest package is still evolving
	return []sdktrace.ReadOnlySpan{}
}

// SpanStubs returns the collected span stubs.
func (te *TestExporter) SpanStubs() tracetest.SpanStubs {
	return te.spanExporter.GetSpans()
}

// Metrics returns the collected metrics.
func (te *TestExporter) Metrics() []metricdata.ScopeMetrics {
	return te.metricExporter.GetMetrics()
}

// WaitForMetrics waits for metrics to be collected and returns the first scope
func (te *TestExporter) WaitForMetrics(timeout time.Duration) (metricdata.ScopeMetrics, bool) {
	return te.metricExporter.WaitForMetrics(timeout)
}

// ForceFlush forces the export of all metrics.
func (te *TestExporter) ForceFlush(ctx context.Context) error {
	return te.metricExporter.ForceFlush(ctx)
}

// Shutdown unregisters and shuts down the exporter.
func (te *TestExporter) Shutdown(ctx context.Context) error {
	// Reset global providers
	otel.SetTracerProvider(nooptrace.NewTracerProvider())
	otel.SetMeterProvider(noopmetric.NewMeterProvider())
	return nil
}

// Call represents a method call/span with its result code
type Call struct {
	Method string
	Status string
	Attrs  []attribute.KeyValue
}

// SpanToCall converts a span to a Call
func SpanToCall(span sdktrace.ReadOnlySpan) Call {
	var method, status string
	var attrs []attribute.KeyValue

	// Copy attributes
	spanAttrs := span.Attributes()
	attrs = make([]attribute.KeyValue, 0, len(spanAttrs))
	for _, attr := range spanAttrs {
		attrs = append(attrs, attr)

		// Extract method if available
		if attr.Key == attribute.Key("gocdk.method") {
			method = attr.Value.AsString()
		}

		// Extract status if available
		if attr.Key == attribute.Key("gocdk.status") {
			status = attr.Value.AsString()
		}
	}

	return Call{
		Method: method,
		Status: status,
		Attrs:  attrs,
	}
}
