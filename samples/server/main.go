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
	"go.opentelemetry.io/otel"
	"log"
	"net/http"
	"sync"
	"time"

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
		return errors.New("not ready yet")
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

	projectID, err := gcp.DefaultProjectID(credentials)
	if err != nil {
		log.Fatal(err)
	}

	if *doTrace {
		fmt.Println("Exporting traces to Stackdriver")

		traceSampler := sdserver.NewTraceSampler(ctx)

		spanExporter, err0 := sdserver.NewTraceExporter(projectID)
		if err0 != nil {
			log.Fatal(err0)
		}
		tp, cleanup, err0 := sdserver.NewTraceProvider(ctx, spanExporter, traceSampler)
		if err0 != nil {
			log.Fatal(err0)
		}
		defer cleanup()
		otel.SetTracerProvider(tp)

		metricsReader, err0 := sdserver.NewMetricsReader(projectID)
		if err0 != nil {
			log.Fatal(err0)
		}
		mp, cleanup2, err0 := sdserver.NewMeterProvider(ctx, metricsReader)
		if err0 != nil {
			log.Fatal(err0)
		}
		defer cleanup2()
		otel.SetMeterProvider(mp)
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
		RequestLogger: sdserver.NewRequestLogger(),
		HealthChecks:  []health.Checker{healthCheck},

		Driver: &server.DefaultDriver{},
	}

	s := server.New(mux, options)
	fmt.Printf("Listening on %s\n", *addr)

	err = s.ListenAndServe(*addr)
	if err != nil {
		log.Fatal(err)
	}
}
