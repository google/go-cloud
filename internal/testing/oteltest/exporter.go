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
	"testing"

	"go.opentelemetry.io/otel"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
)

// TestExporter is an exporter of OpenTelemetry traces and metrics, for testing.
// It should be created with NewTestExporter.
type TestExporter struct {
	mu             sync.Mutex
	spanExporter   *tracetest.InMemoryExporter
	metricExporter *metricExporter
	shutdown       func(context.Context) error
}

// metricExporter is a simple metrics exporter for testing.
// It implements the sdkmetric.Exporter interface.
type metricExporter struct {
	mu                  sync.Mutex
	reader              sdkmetric.Reader
	temporalitySelector sdkmetric.TemporalitySelector
	aggregationSelector sdkmetric.AggregationSelector
	rm                  *metricdata.ResourceMetrics
}

var _ sdkmetric.Exporter = (*metricExporter)(nil)

// newMetricExporter creates a new metric exporter for testing.
func newMetricExporter() *metricExporter {

	reader := sdkmetric.NewManualReader()

	return &metricExporter{
		reader:              reader,
		temporalitySelector: sdkmetric.DefaultTemporalitySelector,
		aggregationSelector: sdkmetric.DefaultAggregationSelector,
		rm:                  &metricdata.ResourceMetrics{},
	}
}

// Temporality returns the aggregation temporality for the given instrument kind.
func (e *metricExporter) Temporality(kind sdkmetric.InstrumentKind) metricdata.Temporality {
	return e.temporalitySelector(kind)
}

// Aggregation returns the aggregation for the given instrument kind.
func (e *metricExporter) Aggregation(kind sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return e.aggregationSelector(kind)
}

// Export exports metric data.
func (e *metricExporter) Export(ctx context.Context, data *metricdata.ResourceMetrics) error {

	return nil
}

// GetMetrics returns all collected metrics.
func (e *metricExporter) GetMetrics() []metricdata.ScopeMetrics {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.rm.ScopeMetrics
}

// ForceFlush forces a flush of metrics.
func (e *metricExporter) ForceFlush(ctx context.Context) error {

	err := e.reader.Collect(ctx, e.rm)
	if err == nil {
		return err
	}
	return e.Export(ctx, e.rm)
}

// Reset the current in-memory storage.
func (e *metricExporter) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rm = &metricdata.ResourceMetrics{}
}

// Shutdown shuts down the exporter.
func (e *metricExporter) Shutdown(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Reset()

	return nil
}

// NewTestExporter creates a TestExporter and registers it with OpenTelemetry.
func NewTestExporter(t *testing.T, views []sdkmetric.View) *TestExporter {
	// Create span exporter
	se := tracetest.NewInMemoryExporter()

	res := resource.NewSchemaless()

	traceShutdown, err := configureTraceProvider("test", se, sdktrace.AlwaysSample(), res, true)
	if err != nil {
		t.Fatalf("Failed to configure trace provider: %v", err)
	}
	// Create metric exporter
	me := newMetricExporter()

	// Create and register meter provider.
	metricsShutdown, err := configureMeterProvider("test", me.reader, res, views)
	if err != nil {
		t.Fatalf("Failed to configure meter provider: %v", err)
	}

	shutdown := func(ctx context.Context) error {
		err1 := traceShutdown(ctx)
		err2 := metricsShutdown(ctx)
		if err1 != nil {
			return err1
		}
		return err2
	}

	return &TestExporter{
		spanExporter:   se,
		metricExporter: me,
		shutdown:       shutdown,
	}
}

// GetSpans returns the collected span stubs.
func (te *TestExporter) GetSpans() tracetest.SpanStubs {
	return te.spanExporter.GetSpans()
}

// GetMetrics returns the collected metrics.
func (te *TestExporter) GetMetrics(ctx context.Context) []metricdata.ScopeMetrics {

	_ = te.metricExporter.ForceFlush(ctx)

	return te.metricExporter.GetMetrics()
}

// ForceFlush forces the export of all metrics.
func (te *TestExporter) ForceFlush(ctx context.Context) error {
	return te.metricExporter.ForceFlush(ctx)
}

// Shutdown unregisters and shuts down the exporter.
func (te *TestExporter) Shutdown(ctx context.Context) error {

	if te.shutdown != nil {
		err := te.shutdown(ctx)
		if err != nil {
			return err
		}
	}
	// Reset global providers
	otel.SetTracerProvider(nooptrace.NewTracerProvider())
	otel.SetMeterProvider(noopmetric.NewMeterProvider())
	return nil
}
