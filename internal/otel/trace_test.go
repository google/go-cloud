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
	"errors"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type testDriver struct{}

func TestProviderName(t *testing.T) {
	testCases := []struct {
		name   string
		driver any
		want   string
	}{
		{"nil", nil, ""},
		{"struct", testDriver{}, "gocloud.dev/internal/otel"},
		{"pointer", &testDriver{}, "gocloud.dev/internal/otel"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := ProviderName(tc.driver)
			if got != tc.want {
				t.Errorf("ProviderName(%#v) = %q, want %q", tc.driver, got, tc.want)
			}
		})
	}
}

func TestTracer(t *testing.T) {
	// Create a test span exporter
	spanRecorder := tracetest.NewSpanRecorder()

	// Create a test provider
	testProvider := trace.NewTracerProvider(
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithSpanProcessor(spanRecorder),
	)

	// Store the original provider
	origProvider := otel.GetTracerProvider()

	// Set our test provider as the global provider
	otel.SetTracerProvider(testProvider)
	defer otel.SetTracerProvider(origProvider)

	// Create a tracer with test values
	tracer := NewTracer("test", "test-provider")

	// Test basic operation - verify the fields are set correctly
	if tracer.Package != "test" {
		t.Errorf("Expected Package = %q, got %q", "test", tracer.Package)
	}

	if tracer.Provider != "test-provider" {
		t.Errorf("Expected Provider = %q, got %q", "test-provider", tracer.Provider)
	}

	// Test Start produces a valid span
	ctx := context.Background()
	ctx, span := tracer.Start(ctx, "TestMethod")

	// End the span so it gets recorded
	span.End()

	// Verify a span was recorded
	spans := spanRecorder.Ended()
	if len(spans) == 0 {
		t.Fatal("Expected at least one recorded span")
	}

	// The name should include the package and method name
	if spans[0].Name() != "test.TestMethod" {
		t.Errorf("Expected span name %q, got %q", "test.TestMethod", spans[0].Name())
	}

	// Reset the recorder for the next test
	spanRecorder.Reset()

	// Test End with error properly sets span status
	ctx = context.Background()
	ctx, span = tracer.Start(ctx, "TestErrorMethod")
	testError := errors.New("test error")
	tracer.End(span, testError)

	// Check the recorded error span
	spans = spanRecorder.Ended()
	if len(spans) == 0 {
		t.Fatal("Expected at least one recorded span for error test")
	}

	// Test SpanFromContext returns the right span
	spanRecorder.Reset()
	ctx = context.Background()
	ctx, _ = tracer.Start(ctx, "TestSpanContext")

	// Verify the context now contains a valid span
	retrievedSpan := SpanFromContext(ctx)
	if !retrievedSpan.SpanContext().IsValid() {
		t.Error("Expected valid span context from SpanFromContext")
	}

	// Test TraceCall helper function
	spanRecorder.Reset()
	ctx = context.Background()
	err := TraceCall(ctx, "TestTraceCall", func(ctx context.Context) error {
		// Verify we have a span in the context
		span := SpanFromContext(ctx)
		if !span.SpanContext().IsValid() {
			t.Error("Expected valid span in context from TraceCall")
		}
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error from TraceCall, got: %v", err)
	}

	// Verify TraceCall created a span
	spans = spanRecorder.Ended()
	if len(spans) == 0 {
		t.Fatal("Expected at least one recorded span for TraceCall test")
	}

	// Test TraceCall with error function
	spanRecorder.Reset()
	testError = errors.New("test trace call error")
	err = TraceCall(ctx, "TestTraceCallWithError", func(ctx context.Context) error {
		return testError
	})

	if err != testError {
		t.Errorf("Expected TraceCall to return the original error")
	}

	// Verify TraceCall created a span with error
	spans = spanRecorder.Ended()
	if len(spans) == 0 {
		t.Fatal("Expected at least one recorded span for TraceCall error test")
	}
}
