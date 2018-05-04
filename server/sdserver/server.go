// Copyright 2018 Google LLC
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
package sdserver

import (
	"context"
	"fmt"
	"os"

	"cloud.google.com/go/compute/metadata"
	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/goose"
	"github.com/google/go-cloud/health"
	"github.com/google/go-cloud/requestlog"
	"github.com/google/go-cloud/server"
	"go.opencensus.io/exporter/stackdriver"
	"go.opencensus.io/trace"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"
)

// Set is a goose provider set that provides the diagnostic hooks for
// *server.Server given a GCP token source and a GCP project ID.
var Set = goose.NewSet(
	server.Set,
	NewExporter,
	goose.Bind(trace.Exporter(nil), (*stackdriver.Exporter)(nil)),
	NewRequestLogger,
	goose.Bind(requestlog.Logger(nil), (*requestlog.StackdriverLogger)(nil)),
)

// Options holds information to initialize GCP.
type Options struct {
	ProjectID    gcp.ProjectID   // if empty, Init will try to fetch by itself.
	TokenSource  gcp.TokenSource // if nil, ADC is used.
	HealthChecks []health.Checker
}

// Init detects and initializes various clients and configurations for GCP apps.
func Init(o *Options) (*server.Server, gcp.ProjectID, gcp.TokenSource, error) {
	if !metadata.OnGCE() {
		return new(server.Server), "", nil, nil
	}
	ctx := context.Background()
	var projectID gcp.ProjectID
	var ts gcp.TokenSource
	var checks []health.Checker
	if o != nil {
		projectID = gcp.ProjectID(o.ProjectID)
		ts = gcp.TokenSource(o.TokenSource)
		checks = o.HealthChecks
	}
	if projectID == "" || ts == nil {
		creds, err := gcp.DefaultCredentials(ctx)
		if err != nil {
			return nil, "", nil, fmt.Errorf("finding application default credentials: %v", err)
		}
		if projectID == "" {
			projectID, err = gcp.DefaultProjectID(creds)
			if err != nil {
				return nil, "", nil, fmt.Errorf("finding project ID: %v", err)
			}
		}
		if ts == nil {
			ts = gcp.CredentialsTokenSource(creds)
		}
	}
	exporter, err := NewExporter(projectID, ts)
	if err != nil {
		return nil, "", nil, fmt.Errorf("creating trace exporter: %v", err)
	}
	reqlog := NewRequestLogger()
	srv := server.New(&server.Options{
		RequestLogger: reqlog,
		TraceExporter: exporter,
		HealthChecks:  checks,
	})
	return srv, projectID, ts, nil
}

// NewExporter returns a new OpenCensus Stackdriver exporter.
func NewExporter(id gcp.ProjectID, ts gcp.TokenSource) (*stackdriver.Exporter, error) {
	tokOpt := option.WithTokenSource(oauth2.TokenSource(ts))
	return stackdriver.NewExporter(stackdriver.Options{
		ProjectID:               string(id),
		MonitoringClientOptions: []option.ClientOption{tokOpt},
		TraceClientOptions:      []option.ClientOption{tokOpt},
	})
}

// NewRequestLogger returns a request logger that sends entries to stdout.
func NewRequestLogger() *requestlog.StackdriverLogger {
	// For now, request logs are written to stdout and get picked up by fluentd.
	// This also works when running locally.
	return requestlog.NewStackdriverLogger(os.Stdout, func(e error) { fmt.Println(e) })
}
