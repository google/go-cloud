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

package server_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"gocloud.dev/health"
	"gocloud.dev/requestlog"
	"gocloud.dev/server"
)

func ExampleServer_New() {
	// This example is used in https://gocloud.dev/howto/server/

	// Use the constructor function to create the server.
	srv := server.New(http.DefaultServeMux, nil)

	// Register a route.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, World!")
	})

	// Start the server.
	srv.ListenAndServe(":8080")
}

func ExampleServer_RequestLogger() {
	// This example is used in https://gocloud.dev/howto/server/

	// Create a logger, and assign it to the RequestLogger field of a
	// server.Options struct.
	srvOptions := &server.Options{
		RequestLogger: requestlog.NewNCSALogger(os.Stdout, func(error) {}),
	}

	// Pass the options to the Server constructor.
	srv := server.New(http.DefaultServeMux, srvOptions)

	// Register a route.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, World!")
	})

	// Start the server. You will see requests logged to STDOUT.
	if err := srv.ListenAndServe(":8080"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// customHealthCheck is an example health check. It implements the
// health.Checker interface and reports the server is healthy when the healthy
// field is set to true.
type customHealthCheck struct {
	mu      sync.RWMutex
	healthy bool
}

// customHealthCheck implements the health.Checker interface because it has a
// CheckHealth method. Because each application may have a different definition
// of what it means to be "healthy", you will need to define a CheckHealth method
// specific to your application.
func (h *customHealthCheck) CheckHealth() error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if !h.healthy {
		return errors.New("not ready yet!")
	}
	return nil
}
func ExampleServer_HealthChecks() {
	// This example is used in https://gocloud.dev/howto/server/

	// Create a health.Checker from the type we defined for our application.
	// In this example, healthCheck will report the server is unhealthy for 10 seconds
	// after startup, and as healthy henceforth. Check the /healthz/readiness
	// HTTP path to see readiness.
	healthCheck := new(customHealthCheck)
	time.AfterFunc(10*time.Second, func() {
		healthCheck.mu.Lock()
		defer healthCheck.mu.Unlock()
		healthCheck.healthy = true
	})

	// The server.Options struct takes a collection of health checks, because you
	// may need to check several things.
	srvOptions := &server.Options{
		HealthChecks: []health.Checker{healthCheck},
	}

	// Pass the options to the Server constructor.
	srv := server.New(http.DefaultServeMux, srvOptions)

	// Register a route.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, World!")
	})

	// Start the server. You will see requests logged to STDOUT.
	if err := srv.ListenAndServe(":8080"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func ExampleServer_Shutdown() {
	// This example is used in https://gocloud.dev/howto/server/

	// OPTIONAL: Specify a driver in the options for the constructor.
	// NewDefaultDriver will be used by default if it is not explicitly set, and
	// uses http.Server with read, write, and idle timeouts set.
	srvOptions := &server.Options{
		Driver: NewDefaultDriver(),
	}

	// Pass the options to the Server constructor.
	srv := server.New(http.DefaultServeMux, srvOptions)

	// Register routes, call ListenAndServe.

	// Shutdown the server gracefully without interrupting any active connections.
	srv.Shutdown(context.Background())
}
