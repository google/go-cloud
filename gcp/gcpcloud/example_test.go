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

package gcpcloud_test

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/google/wire"
	"go.opencensus.io/trace"
	"gocloud.dev/gcp/gcpcloud"
	"gocloud.dev/server"
	"gocloud.dev/server/health"
)

// This is an example of how to bootstrap an HTTP server running on
// Google Cloud Platform (GCP). The code in this function would be
// placed in main().
func Example() {
	// Connect and authenticate to GCP.
	srv, cleanup, err := setup(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	// Set up the HTTP routes.
	http.HandleFunc("/", greet)

	// Run the server. This behaves much like http.ListenAndServe,
	// including that passing a nil handler will use http.DefaultServeMux.
	log.Fatal(srv.ListenAndServe(":8080"))
}

// setup is a Wire injector function that creates an HTTP server
// configured to send diagnostics to Stackdriver. The second return
// value is a clean-up function that can be called to shut down any
// resources created by setup.
//
// The body of this function will be filled in by running Wire. While
// the name of the function does not matter, the signature signals to
// Wire what provider functions to call. See
// https://github.com/google/wire/blob/master/docs/guide.md#injectors
// for more details.
func setup(ctx context.Context) (*server.Server, func(), error) {
	wire.Build(
		// The GCP set includes all the default wiring for GCP, including
		// for *server.Server.
		gcpcloud.GCP,
		// Providing nil instructs the server to use the default sampling policy.
		wire.Value(trace.Sampler(nil)),
		// Health checks can be added to delay your server reporting healthy
		// to the load balancer before critical dependencies are available.
		wire.Value([]health.Checker{}),
	)
	return nil, nil, nil
}

// greet is an ordinary http.HandleFunc.
func greet(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintln(w, "Hello, World!")
}
