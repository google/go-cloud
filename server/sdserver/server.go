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
	"fmt"
	"go.opencensus.io/trace"
	"go.opentelemetry.io/otel/metric"
	"gocloud.dev/server/otel"
	"os"

	gcpmetricsexporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric"
	gcptraceexporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	"github.com/google/wire"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"gocloud.dev/gcp"
	"gocloud.dev/server/requestlog"
)

// Set is a Wire provider set that provides the diagnostic hooks for
// *server.Server given a GCP token source and a GCP project ID.
var Set = wire.NewSet(
	otel.Set,
	NewTraceExporter,
	wire.Bind(new(trace.Exporter), new(*sdktrace.SpanExporter)),
	NewMetricsExporter,
	wire.Bind(new(metric.MeterProvider), new(*sdkmetric.MeterProvider)),
	NewRequestLogger,
	wire.Bind(new(requestlog.Logger), new(*requestlog.StackdriverLogger)),
)

// NewTraceExporter returns a new OpenTelemetry gcp trace exporter.
func NewTraceExporter(projectID gcp.ProjectID) (sdktrace.SpanExporter, error) {
	exporter, err := gcptraceexporter.New(gcptraceexporter.WithProjectID(string(projectID)))
	if err != nil {
		return nil, err
	}

	return exporter, nil
}

// NewMetricsExporter returns a new OpenTelemetry gcp metrics exporter.
func NewMetricsExporter(projectID gcp.ProjectID) (sdkmetric.Exporter, error) {
	exporter, err := gcpmetricsexporter.New(gcpmetricsexporter.WithProjectID(string(projectID)))
	if err != nil {
		return nil, err
	}

	return exporter, nil
}

// NewRequestLogger returns a request logger that sends entries to stdout.
func NewRequestLogger() *requestlog.StackdriverLogger {
	// For now, request logs are written to stdout and get picked up by fluentd.
	// This also works when running locally.
	return requestlog.NewStackdriverLogger(os.Stdout, func(e error) { fmt.Println(e) })
}
