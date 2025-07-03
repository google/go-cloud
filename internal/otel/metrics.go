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
	"fmt"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"strings"
)

// Units are encoded according to the case-sensitive abbreviations from the
// Unified Code for Units of Measure: http://unitsofmeasure.org/ucum.html.
const (
	unitMilliseconds = "ms"
)

var (
	DefaultMillisecondsBoundaries = []float64{
		0.0, 0.1, 0.2, 0.4, 0.6, 0.8, 1.0,
		2.0, 3.0, 4.0, 5.0, 6.0, 8.0, 10.0,
		13.0, 16.0, 20.0, 25.0, 30.0, 40.0,
		50.0, 65.0, 80.0, 100.0, 130.0, 160.0,
		200.0, 250.0, 300.0, 400.0, 500.0,
		650.0, 800.0, 1000.0, 2000.0, 5000.0, 10000.0,
	}
)

func Views(pkg string) []sdkmetric.View {

	return []sdkmetric.View{

		// View for latency histogram.
		func(inst sdkmetric.Instrument) (sdkmetric.Stream, bool) {
			if inst.Kind == sdkmetric.InstrumentKindHistogram {
				if inst.Name == pkg+"/latency" {
					return sdkmetric.Stream{
						Name:        inst.Name,
						Description: "Distribution of method latency, by provider and method.",
						Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
							Boundaries: DefaultMillisecondsBoundaries,
						},
						AttributeFilter: func(kv attribute.KeyValue) bool {
							return kv.Key == PackageKey || kv.Key == MethodKey
						},
					}, true
				}
			}

			return sdkmetric.Stream{}, false
		},

		// View for completed_calls count.
		func(inst sdkmetric.Instrument) (sdkmetric.Stream, bool) {
			if inst.Kind == sdkmetric.InstrumentKindHistogram {
				if inst.Name == pkg+"/latency" {
					return sdkmetric.Stream{
						Name:        strings.Replace(inst.Name, "/latency", "/completed_calls", 1),
						Description: "Count of method calls by provider, method and status.",
						Aggregation: sdkmetric.DefaultAggregationSelector(sdkmetric.InstrumentKindCounter),
						AttributeFilter: func(kv attribute.KeyValue) bool {
							return kv.Key == MethodKey || kv.Key == StatusKey
						},
					}, true
				}
			}
			return sdkmetric.Stream{}, false
		},
	}
}

// LatencyMeasure returns the measure for method call latency used by Go CDK APIs.
func LatencyMeasure(pkg string, provider string) metric.Float64Histogram {

	attrs := []attribute.KeyValue{
		PackageKey.String(pkg),
		ProviderKey.String(provider),
	}

	pkgMeter := otel.Meter(pkg, metric.WithInstrumentationAttributes(attrs...))

	m, err := pkgMeter.Float64Histogram(
		pkg+"/latency",
		metric.WithDescription("Latency distribution of method calls"),
		metric.WithUnit(unitMilliseconds),
	)

	if err != nil {
		// The only possible errors are from invalid key or value names, and those are programming
		// errors that will be found during testing.
		panic(fmt.Sprintf("fullName=%q, provider=%q: %v", pkg, pkgMeter, err))
	}

	return m
}
