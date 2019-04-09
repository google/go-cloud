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

// The app command is a test app that is initialized with the GCP SDK.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/google/wire"
	"gocloud.dev/health"

	"go.opencensus.io/trace"
)

var appSet = wire.NewSet(
	wire.Value([]health.Checker{connection}),
	trace.AlwaysSample,
	newRouter,
	wire.Bind((*http.Handler)(nil), (*http.ServeMux)(nil)),
)

func newRouter() *http.ServeMux {
	m := http.NewServeMux()
	m.HandleFunc("/", handleMain)
	m.HandleFunc("/connect", handleConnect)
	return m
}

var connection = new(connectionChecker)

func main() {
	var projectID string
	flag.StringVar(&projectID, "project", "", "Project ID to use for the test app")

	flag.Parse()
	srv, cleanup, err := initialize(context.Background())
	if err != nil {
		log.Fatalf("unable to initialize server: %v", err)
	}
	defer cleanup()
	log.Fatal(srv.ListenAndServe(":8080"))
}

func handleMain(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "Hello, World!")
}

func handleConnect(w http.ResponseWriter, r *http.Request) {
	connection.connect()
	fmt.Fprint(w, "Connected")
}

type connectionChecker struct {
	mu        sync.Mutex
	connected bool
}

func (c *connectionChecker) connect() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = true
}

func (c *connectionChecker) CheckHealth() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return errors.New("not connected")
	}
	return nil
}
