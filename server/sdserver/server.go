// Copyright 2018 The Go Cloud Development Kit Authors
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

// Package sdserver provides the diagnostic hooks for a server using
// Stackdriver.
package sdserver // import "gocloud.dev/server/sdserver"

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"

	"github.com/google/wire"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv/v1.17.0"
	"gocloud.dev/server"
	"gocloud.dev/server/requestlog"
	"golang.org/x/oauth2"
	"google.golang.org/grpc/credentials"
)

// Set is a Wire provider set that provides the diagnostic hooks for
// *server.Server given a GCP token source and a GCP project ID.
var Set = wire.NewSet(
	server.Set,
	NewTraceExporter,
	wire.Bind(new(trace.SpanExporter), new(*otlptrace.Exporter)),
	NewRequestLogger,
	wire.Bind(new(requestlog.Logger), new(*requestlog.StackdriverLogger)),
)

// ProjectID is the Google Cloud Platform project ID.
type ProjectID string

// TokenSource is a source of OAuth2 tokens for use with Google Cloud Platform.
type TokenSource oauth2.TokenSource

// NewTraceExporter returns a new OpenTelemetry exporter configured for Google Cloud Stackdriver.
//
// The second return value is a Wire cleanup function that shuts down the tracer provider.
func NewTraceExporter(id ProjectID, ts TokenSource) (*otlptrace.Exporter, func(), error) {
	ctx := context.Background()
	
	// Create a resource with GCP detection
	detector := gcp.NewDetector()
	res, err := resource.New(ctx,
		resource.WithDetectors(detector),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String("gocloud-server"),
			semconv.ServiceVersionKey.String("1.0.0"),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create resource: %w", err)
	}
	
	// Get user agent string for headers
	uaStr := "gocloud-server"
	
	client := otlptracegrpc.NewClient(
		otlptracegrpc.WithEndpoint("cloudtrace.googleapis.com:443"),
		otlptracegrpc.WithTLSCredentials(credentials.NewTLS(&tls.Config{})),
		otlptracegrpc.WithHeaders(map[string]string{"User-Agent": uaStr}),
	)
	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}
	
	// Create and register a TracerProvider
	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(res),
		trace.WithSampler(trace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	
	return exporter, func() { tp.Shutdown(context.Background()) }, nil
}

// NewRequestLogger returns a request logger that sends entries to stdout.
func NewRequestLogger() *requestlog.StackdriverLogger {
	// For now, request logs are written to stdout and get picked up by fluentd.
	// This also works when running locally.
	return requestlog.NewStackdriverLogger(os.Stdout, func(e error) { fmt.Println(e) })
}

// MonitoredResourceInterface represents a type that can provide monitored resource information.
// This interface is provided for backwards compatibility with OpenCensus.
// Deprecated: Use OpenTelemetry resource detection mechanisms directly instead.
type MonitoredResourceInterface interface {
	// MonitoredResource returns the monitored resource type and a map of labels.
	MonitoredResource() (string, map[string]string)
}

// NewOTelExporter creates a new OpenTelemetry exporter for use with Google Cloud Trace.
// It accepts a monitored resource for backwards compatibility with OpenCensus configurations.
//
// Deprecated: This function is provided for backward compatibility with code using the
// previous OpenCensus-based API. New code should use NewTraceExporter directly.
//
// The second return value is a cleanup function that flushes any pending traces and shuts down the exporter.
// The third return value is an empty interface for backward compatibility; it is always nil.
func NewOTelExporter(projectID ProjectID, ts TokenSource, mr MonitoredResourceInterface) (*otlptrace.Exporter, func(), error) {
	ctx := context.Background()
	
	// Get resource information from the monitored resource
	resType, labels := mr.MonitoredResource()
	
	// Create attributes from the resource labels
	attrs := []attribute.KeyValue{
		attribute.String("gcp.resource_type", resType),
	}
	for k, v := range labels {
		attrs = append(attrs, attribute.String(k, v))
	}
	
	// Create a resource with the attributes
	res, err := resource.New(ctx,
		resource.WithTelemetrySDK(),
		resource.WithAttributes(attrs...),
		resource.WithAttributes(
			semconv.ServiceNameKey.String("gocloud-server"),
			semconv.ServiceVersionKey.String("1.0.0"),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create resource: %w", err)
	}
	
	// Setup secure connection options
	secureOption := otlptracegrpc.WithTLSCredentials(credentials.NewTLS(&tls.Config{}))
	
	// Setup headers for user agent
	headerOption := otlptracegrpc.WithHeaders(map[string]string{"User-Agent": "gocloud-server"})
	
	client := otlptracegrpc.NewClient(
		secureOption,
		otlptracegrpc.WithEndpoint("cloudtrace.googleapis.com:443"),
		headerOption,
	)
	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}
	
	// Create and register a TracerProvider
	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(res),
		trace.WithSampler(trace.AlwaysSample()),
	)
	
	return exporter, func() { tp.Shutdown(context.Background()) }, nil
}
