// Copyright 2018 The Go Cloud Authors
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

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/google/go-cloud/health"
	"github.com/google/go-cloud/wire"

	"go.opencensus.io/trace"
)

type appConfig struct {
	region string
}

var appSet = wire.NewSet(
	wire.Value([]health.Checker{connection}),
	trace.AlwaysSample,
)

var connection = &connectionChecker{connected: true}

func main() {
	cfg := new(appConfig)
	flag.StringVar(&cfg.region, "region", "us-west-1", "AWS region for xray")
	srv, cleanup, err := initialize(context.Background(), cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	m := http.NewServeMux()
	m.HandleFunc("/", handleMain)
	m.HandleFunc("/disconnect", handleDisconnect)
	log.Fatal(srv.ListenAndServe(":8080", m))
}

func handleMain(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "Hello, World!")
}

func handleDisconnect(w http.ResponseWriter, r *http.Request) {
	connection.disconnect()
	fmt.Fprintf(w, "Disconnected")
}

type connectionChecker struct {
	mu        sync.Mutex
	connected bool
}

func (c *connectionChecker) disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = false
}

func (c *connectionChecker) CheckHealth() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return errors.New("not connected")
	}
	return nil
}
