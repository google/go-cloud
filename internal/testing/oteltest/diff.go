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
	"strings"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Diff compares OpenTelemetry trace spans and metrics to expected values.
// It returns a non-empty string if there are any discrepancies.
func Diff(spans []sdktrace.ReadOnlySpan, pkg, provider string, want []Call) string {
	if len(spans) != len(want) {
		return fmt.Sprintf("got %d spans, want %d", len(spans), len(want))
	}

	// Convert spans to calls for easier comparison
	var got []Call
	for _, span := range spans {
		call := SpanToCall(span)
		// Check if the span belongs to the expected package
		if !hasAttributeWithValue(call.Attrs, attribute.Key("gocdk.package"), pkg) {
			continue
		}
		// Check if the span belongs to the expected provider
		if provider != "" && !hasAttributeWithValue(call.Attrs, attribute.Key("gocdk.provider"), provider) {
			continue
		}
		got = append(got, call)
	}

	if len(got) != len(want) {
		return fmt.Sprintf("got %d matching spans, want %d", len(got), len(want))
	}

	// Verify each span matches the expected call
	for i, g := range got {
		w := want[i]
		if g.Method != w.Method {
			return fmt.Sprintf("#%d: got method %q, want %q", i, g.Method, w.Method)
		}
		if g.Status != w.Status {
			return fmt.Sprintf("#%d: got status %q, want %q", i, g.Status, w.Status)
		}
	}

	return ""
}

// DiffSpanAttr verifies that a span has an attribute with the expected value.
// It's useful for more detailed assertions on span attributes.
func DiffSpanAttr(span sdktrace.ReadOnlySpan, key attribute.Key, wantValue string) string {
	for _, attr := range span.Attributes() {
		if attr.Key == key {
			if attr.Value.AsString() == wantValue {
				return ""
			}
			return fmt.Sprintf("for key %s: got %q, want %q", key, attr.Value.AsString(), wantValue)
		}
	}
	return fmt.Sprintf("key %s not found", key)
}

// hasAttributeWithValue checks if the attribute set contains a key with the expected value.
func hasAttributeWithValue(attrs []attribute.KeyValue, key attribute.Key, value string) bool {
	for _, attr := range attrs {
		if attr.Key == key && attr.Value.AsString() == value {
			return true
		}
	}
	return false
}

// FormatSpans returns a formatted string of span data for debugging.
func FormatSpans(spans []sdktrace.ReadOnlySpan) string {
	var b strings.Builder
	for i, span := range spans {
		fmt.Fprintf(&b, "#%d: %s\n", i, span.Name())
		fmt.Fprintf(&b, "  TraceID: %s\n", span.SpanContext().TraceID())
		fmt.Fprintf(&b, "  SpanID: %s\n", span.SpanContext().SpanID())
		fmt.Fprintf(&b, "  Status: %s\n", span.Status().Code)

		if len(span.Attributes()) > 0 {
			fmt.Fprintf(&b, "  Attributes:\n")
			for _, attr := range span.Attributes() {
				fmt.Fprintf(&b, "    %s: %s\n", attr.Key, attr.Value.AsString())
			}
		}

		if len(span.Events()) > 0 {
			fmt.Fprintf(&b, "  Events:\n")
			for _, event := range span.Events() {
				fmt.Fprintf(&b, "    %s\n", event.Name)
				for _, attr := range event.Attributes {
					fmt.Fprintf(&b, "      %s: %s\n", attr.Key, attr.Value.AsString())
				}
			}
		}
	}
	return b.String()
}
