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
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/oauth"
	"os"

	"github.com/google/wire"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"gocloud.dev/server"
	"gocloud.dev/server/requestlog"
	"golang.org/x/oauth2"
	"google.golang.org/grpc/credentials"
)

// Set is a Wire provider set that provides the diagnostic hooks for
// *server.Server given a GCP token source and a GCP project ID.
var Set = wire.NewSet(
	NewGcpTraceProvider,
	server.Set,
	NewRequestLogger,
	wire.Bind(new(requestlog.Logger), new(*requestlog.StackdriverLogger)),
)

// ProjectID is the Google Cloud Platform project ID.
type ProjectID string

// TokenSource is a source of OAuth2 tokens for use with Google Cloud Platform.
type TokenSource oauth2.TokenSource

// NewGcpTraceProvider returns an OpenTelemetry provider configured for Google Cloud Trace.
//
// The second return value is a Wire cleanup function that shuts down the tracer provider.
func NewGcpTraceProvider(id ProjectID, ts TokenSource, res *resource.Resource, sampler trace.Sampler) (*trace.TracerProvider, error) {
	ctx := context.Background()

	serviceName := "gocloud-server"

	if res == nil {
		var err error
		// Create a resource with GCP detection
		detector := gcp.NewDetector()
		res, err = resource.New(ctx,
			resource.WithDetectors(detector),
			resource.WithTelemetrySDK(),
			resource.WithAttributes(
				semconv.ServiceNameKey.String(serviceName),
				semconv.ServiceVersionKey.String("1.0.0"),
				semconv.CloudAccountIDKey.String(string(id)),
			),
		)

		if err != nil {
			return nil, fmt.Errorf("failed to create resource: %w", err)
		}
	}

	if sampler == nil {
		sampler = trace.AlwaysSample()
	}
	tokenSource := oauth.TokenSource{TokenSource: ts}

	client := otlptracegrpc.NewClient(
		otlptracegrpc.WithEndpoint("cloudtrace.googleapis.com:443"),
		otlptracegrpc.WithTLSCredentials(credentials.NewTLS(&tls.Config{})),
		otlptracegrpc.WithHeaders(map[string]string{"User-Agent": serviceName}),
		otlptracegrpc.WithDialOption(grpc.WithPerRPCCredentials(tokenSource)),
	)
	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Create and register a TracerProvider
	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(res),
		trace.WithSampler(sampler),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp, nil
}

// NewRequestLogger returns a request logger that sends entries to stdout.
func NewRequestLogger() *requestlog.StackdriverLogger {
	// For now, request logs are written to stdout and get picked up by fluentd.
	// This also works when running locally.
	return requestlog.NewStackdriverLogger(os.Stdout, func(e error) { fmt.Println(e) })
}
