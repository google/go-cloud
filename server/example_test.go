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
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"gocloud.dev/server"

	"gocloud.dev/server/health"
	"gocloud.dev/server/requestlog"
)

func ExampleServer_New() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.

	// Use the constructor function to create the server.
	srv := server.New(http.DefaultServeMux, nil)

	// Register a route.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, World!")
	})

	// Start the server. If ListenAndServe returns an error, print it and exit.
	if err := srv.ListenAndServe(":8080"); err != nil {
		log.Fatalf("%v", err)
	}
}

func ExampleServer_RequestLogger() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.

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
		log.Fatalf("%v", err)
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
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.

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

	// The server.Options struct takes a slice of health checks, because you
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
		log.Fatalf("%v", err)
	}
}

func ExampleServer_Shutdown() {
	// OPTIONAL: Specify a driver in the options for the constructor.
	// NewDefaultDriver will be used by default if it is not explicitly set, and
	// uses http.Server with read, write, and idle timeouts set. When Shutdown
	// is called on the server, it is called on the driver.
	srvOptions := &server.Options{
		Driver: server.NewDefaultDriver(),
	}

	// Pass the options to the Server constructor.
	srv := server.New(http.DefaultServeMux, srvOptions)

	// If your application will be behind a load balancer that handles graceful
	// shutdown of requests, you may not need to call Shutdown on the server
	// directly. If you need to ensure graceful shutdown directly, it is important
	// to have a separate goroutine, because ListenAndServe blocks indefinitely.
	go func() {
		interrupt := make(chan os.Signal, 1)
		signal.Notify(interrupt, os.Interrupt)
		// Receive off the chanel in a loop, because the interrupt could be sent
		// before ListenAndServe starts.
		for {
			<-interrupt
			srv.Shutdown(context.Background())
		}
	}()

	// Register a route.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, World!")
	})

	// Start the server. You will see requests logged to STDOUT.
	// In the absence of an error, ListenAndServe blocks forever.
	if err := srv.ListenAndServe(":8080"); err != nil {
		log.Fatalf("%v", err)
	}
}
