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

package oteltest

import (
	"fmt"
	"go.opentelemetry.io/otel/codes"
	"sort"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"gocloud.dev/gcerrors"
)

var (
	methodKey   = attribute.Key("gocdk_method")
	providerKey = attribute.Key("gocdk_provider")
	statusKey   = attribute.Key("gocdk_status")
)

// Call represents a method call/span with its result code.
type Call struct {
	Method string
	Code   gcerrors.ErrorCode
	Attrs  []attribute.KeyValue
}

func formatSpanData(s sdktrace.ReadOnlySpan) string {
	if s == nil {
		return "missing"
	}
	// OTel uses codes.Code for status.
	return fmt.Sprintf("<Name: %q, Code: %s>", s.Name(), s.Status().Code.String())
}

func formatCall(c *Call) string {
	if c == nil {
		return "nothing"
	}
	// gcerrors.ErrorCode is an int, just print it.
	return fmt.Sprintf("<Name: %q, Code: %d>", c.Method, c.Code)
}

// Diff compares the list of spans and metric data obtained from OpenTelemetry
// instrumentation (using a test exporter like `sdktrace/tracetest.NewExporter`
// and `sdkmetric/metrictest.NewExporter`) with an expected list of calls.
// The span/metric name and status code/status attribute are compared.
// Order matters for traces (though not for metrics).
//
// gotSpans should be the result from a test trace exporter (e.g., exporter.GetSpans()).
// gotMetrics should be the result from a test metric exporter (e.g., exporter.GetMetrics()).
// namePrefix is the prefix prepended to method names in spans/metrics mostly its the package name.
// provider is the name of the provider used (e.g., "aws").
// want is the list of expected calls.
func Diff(gotSpans []sdktrace.ReadOnlySpan, gotMetrics []metricdata.ScopeMetrics, namePrefix, provider string, want []Call) string {
	ds := diffSpans(gotSpans, namePrefix, want)
	dc := DiffMetrics(gotMetrics, namePrefix, provider, want)
	if len(ds) > 0 {
		ds = "trace: " + ds + "\n"
	}
	if len(dc) > 0 {
		dc = "metrics: " + dc
	}
	return ds + dc
}

func mapStatusCode(code gcerrors.ErrorCode) codes.Code {
	// For gcerrors used by gocloud, OK -> Ok, everything else -> Error is common.
	if code == gcerrors.OK {
		return codes.Ok
	}
	return codes.Error
}

func diffSpans(got []sdktrace.ReadOnlySpan, prefix string, want []Call) string {
	var diffs []string
	add := func(i int, g sdktrace.ReadOnlySpan, w *Call) {
		diffs = append(diffs, fmt.Sprintf("#%d: got %s, want %s", i, formatSpanData(g), formatCall(w)))
	}

	for i := 0; i < len(got) || i < len(want); i++ {
		var gotSpan sdktrace.ReadOnlySpan
		if i < len(got) {
			gotSpan = got[i]
		}

		switch {
		case i >= len(got):
			add(i, nil, &want[i])
		case i >= len(want):
			add(i, gotSpan, nil)
		default:
			expectedName := prefix + "." + want[i].Method
			expectedCode := mapStatusCode(want[i].Code) // Map wanted gcerrors code to OTel code.

			if gotSpan == nil || gotSpan.Name() != expectedName || gotSpan.Status().Code != expectedCode {
				w := want[i]
				w.Method = prefix + "." + w.Method
				add(i, gotSpan, &w)
			}
		}
	}
	return strings.Join(diffs, "\n")
}

func DiffMetrics(got []metricdata.ScopeMetrics, prefix, provider string, wantCalls []Call) string {
	// OTel metric data is structured. We need to iterate through it to find the
	// relevant metric data points and their attributes.
	var diffs []string
	gotTags := map[string]bool{} // map of canonicalized data point attributes

	// Helper to convert attribute.Set to a canonical string key
	attrSetToCanonicalString := func(set attribute.Set) string {
		// Get key-value pairs, sort them, and format into a stable string.
		attrs := make([]attribute.KeyValue, 0, set.Len())
		iter := set.Iter()
		for iter.Next() {
			attrs = append(attrs, iter.Attribute())
		}
		sort.Slice(attrs, func(i, j int) bool {
			return string(attrs[i].Key) < string(attrs[j].Key)
		})
		parts := make([]string, len(attrs))
		for i, attr := range attrs {
			// Format value based on type - attribute.Value doesn't have a simple String()
			// that's guaranteed to be consistent for comparison. Using fmt.Sprint is safer.
			parts[i] = fmt.Sprintf("%s:%s", attr.Key, fmt.Sprint(attr.Value.AsInterface()))
		}
		return strings.Join(parts, ",")
	}

	// Helper function to collect relevant attributes for tag comparison.
	processAtrributes := func(attrSets ...attribute.Set) {

		var requiredAttributes []attribute.KeyValue
		for _, attrSet := range attrSets {
			for _, a := range attrSet.ToSlice() {
				if a.Key == providerKey {
					requiredAttributes = append(requiredAttributes, a)
				}

				if a.Key == methodKey {
					requiredAttributes = append(requiredAttributes, a)
				}

				if a.Key == statusKey {
					requiredAttributes = append(requiredAttributes, a)
				}
			}
		}

		if len(requiredAttributes) > 0 {
			gotTags[attrSetToCanonicalString(attribute.NewSet(requiredAttributes...))] = true
		}

	}

	// Iterate through all collected metrics to find relevant data points.
	for _, sm := range got {

		for _, m := range sm.Metrics {

			// Using a switch will allow us accommodate other types of metrics.
			switch v := m.Data.(type) {
			case metricdata.Sum[int64]:
				// Handle int64 Sum metrics.
				for _, dp := range v.DataPoints {
					processAtrributes(sm.Scope.Attributes, dp.Attributes)
				}
			case metricdata.Sum[float64]:
				// gocloud usually records counts. Check for Sum metrics.
				for _, dp := range v.DataPoints {
					processAtrributes(sm.Scope.Attributes, dp.Attributes)
				}
			default:
				// Handle any other types of metrics.
				processAtrributes(sm.Scope.Attributes)
			}
		}
	}

	// Check that each wanted call has a corresponding metric data point with the correct attributes.
	for _, wc := range wantCalls {
		// Construct the expected set of attributes for the wanted call.
		expectedAttributes := []attribute.KeyValue{providerKey.String(provider)}

		if wc.Method != "" {
			expectedAttributes = append(expectedAttributes,
				methodKey.String(prefix+"."+wc.Method),
				statusKey.String(fmt.Sprint(wc.Code)))
		}

		// Canonicalize the expected attributes to check against the collected ones.
		expectedKey := attrSetToCanonicalString(attribute.NewSet(expectedAttributes...))

		if !gotTags[expectedKey] {
			diffs = append(diffs, fmt.Sprintf("missing metric data point with attributes %q", expectedKey))
		}
	}
	return strings.Join(diffs, "\n")
}
