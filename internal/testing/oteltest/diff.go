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
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Diff compares OpenTelemetry trace spans and metrics to expected values.
// It returns a non-empty string if there are any discrepancies.
func Diff(spans []sdktrace.ReadOnlySpan, want []Call) string {
	if len(spans) != len(want) {
		return fmt.Sprintf("got %d spans, want %d", len(spans), len(want))
	}

	// Convert spans to calls for easier comparison.
	var got []Call
	for _, span := range spans {
		call := SpanToCall(span)

		got = append(got, call)
	}

	if len(got) != len(want) {
		return fmt.Sprintf("got %d matching spans, want %d", len(got), len(want))
	}

	// Verify each span matches the expected call.
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
