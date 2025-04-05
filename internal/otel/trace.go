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

package otel

import (
	"context"
	"fmt"
	"reflect"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"gocloud.dev/gcerrors"
)

// Common attribute keys used across the Go CDK
var (
	MethodKey   = attribute.Key("gocdk.method")
	PackageKey  = attribute.Key("gocdk.package")
	ProviderKey = attribute.Key("gocdk.provider")
	StatusKey   = attribute.Key("gocdk.status")
	ErrorKey    = attribute.Key("gocdk.error")
)

// Tracer provides OpenTelemetry tracing for Go CDK packages.
type Tracer struct {
	Package  string
	Provider string
}

// ProviderName returns the name of the provider associated with the driver value.
// It is intended to be used to set Tracer.Provider.
// It actually returns the package path of the driver's type.
func ProviderName(driver any) string {
	// Return the last component of the package path.
	if driver == nil {
		return ""
	}
	t := reflect.TypeOf(driver)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.PkgPath()
}

// NewTracer creates a new Tracer for a package and optional provider.
func NewTracer(pkg string, provider ...string) *Tracer {
	providerName := ""
	if len(provider) > 0 && provider[0] != "" {
		providerName = provider[0]
	}

	return &Tracer{
		Package:  pkg,
		Provider: providerName,
	}
}

// Start creates and starts a new span and returns the updated context and span.
func (t *Tracer) Start(ctx context.Context, methodName string) (context.Context, trace.Span) {
	fullName := t.Package + "." + methodName

	// Build attributes list
	attrs := []attribute.KeyValue{
		PackageKey.String(t.Package),
		MethodKey.String(methodName),
	}

	if t.Provider != "" {
		attrs = append(attrs, ProviderKey.String(t.Provider))
	}

	// Use the global tracer provider
	return otel.Tracer(t.Package).Start(ctx, fullName, trace.WithAttributes(attrs...))
}

// End completes a span with error information if applicable.
func (t *Tracer) End(span trace.Span, err error) {
	if err != nil {
		code := gcerrors.Code(err)
		span.SetAttributes(
			ErrorKey.String(err.Error()),
			StatusKey.String(fmt.Sprint(code)),
		)
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	} else {
		span.SetStatus(codes.Ok, "")
	}

	span.End()
}

// StartSpan is a convenience function that creates a span using the global tracer.
func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return otel.Tracer("").Start(ctx, name, trace.WithAttributes(attrs...))
}

// TraceCall is a helper that traces the execution of a function.
func TraceCall(ctx context.Context, name string, fn func(context.Context) error) error {
	ctx, span := StartSpan(ctx, name)
	defer span.End()

	err := fn(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return err
}

// SpanFromContext retrieves the current span from the context.
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// TracingEnabled returns whether tracing is currently enabled.
func TracingEnabled() bool {
	return otel.GetTracerProvider() != trace.NewNoopTracerProvider()
}
