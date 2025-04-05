// Copyright 2019 The Go Cloud Development Kit Authors
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

// Command server runs a simple HTTP server with integrated Cloud Trace (OpenTelemetry)
// and health checks.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	"gocloud.dev/gcp"
	"gocloud.dev/server"
	"gocloud.dev/server/health"
	"gocloud.dev/server/sdserver"
)

// GlobalMonitoredResource implements monitoredresource.Interface to provide a
// basic global resource based on the project ID. If you're running this sample
// on GCE or EC2, you may prefer to use monitoredresource.Autodetect() instead.
type GlobalMonitoredResource struct {
	projectID string
}

// MonitoredResource returned the monitored resource.
func (g GlobalMonitoredResource) MonitoredResource() (string, map[string]string) {
	return "global", map[string]string{"project_id": g.projectID}
}

func helloHandler(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, "Hello\n")
}

func mainHandler(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, "Welcome to the home page!\n")
}

// customHealthCheck is an example health check. It implements the
// health.Checker interface and reports the server is healthy when the healthy
// field is set to true.
type customHealthCheck struct {
	mu      sync.RWMutex
	healthy bool
}

func (h *customHealthCheck) CheckHealth() error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if !h.healthy {
		return errors.New("not ready yet!")
	}
	return nil
}

func main() {
	addr := flag.String("listen", ":8080", "HTTP port to listen on")
	doTrace := flag.Bool("trace", true, "Export traces to Stackdriver")
	flag.Parse()

	ctx := context.Background()
	credentials, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		log.Fatal(err)
	}
	tokenSource := gcp.CredentialsTokenSource(credentials)
	projectID, err := gcp.DefaultProjectID(credentials)
	if err != nil {
		log.Fatal(err)
	}

	traceProvider := trace.NewTracerProvider(trace.WithSampler(trace.NeverSample()))
	if *doTrace {
		fmt.Println("Exporting traces to Google Cloud Trace")

		// Create resource with project information
		res := resource.NewWithAttributes(
			"testing",
			// Add relevant resource attributes here
		)

		traceProvider, err = sdserver.NewGcpTraceProvider(sdserver.ProjectID(projectID), sdserver.TokenSource(tokenSource), res, trace.AlwaysSample())
		if err != nil {
			log.Fatal(err)
		}

		otel.SetTracerProvider(traceProvider)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/hello", helloHandler)
	mux.HandleFunc("/", mainHandler)

	// healthCheck will report the server is unhealthy for 10 seconds after
	// startup, and as healthy henceforth. Check the /healthz/readiness
	// HTTP path to see readiness.
	healthCheck := new(customHealthCheck)
	time.AfterFunc(10*time.Second, func() {
		healthCheck.mu.Lock()
		defer healthCheck.mu.Unlock()
		healthCheck.healthy = true
	})

	options := &server.Options{
		RequestLogger:  sdserver.NewRequestLogger(),
		HealthChecks:   []health.Checker{healthCheck},
		TracerProvider: traceProvider,

		// Note: Sampling is now configured at the TracerProvider level above
		// In production you will likely want to use a probabilistic sampler
		// instead of AlwaysSample to avoid generating too much data
		Driver: &server.DefaultDriver{},
	}

	s := server.New(mux, options)
	fmt.Printf("Listening on %s\n", *addr)

	err = s.ListenAndServe(*addr)
	if err != nil {
		log.Fatal(err)
	}
}
