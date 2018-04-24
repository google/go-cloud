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

// The test-driver command makes various requests to the deployed test app and
// tests features initialized with the server by the SDK.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"cloud.google.com/go/logging/logadmin"
	tracepb "cloud.google.com/go/trace/apiv1"
)

var (
	address        string
	projectID      string
	logadminClient *logadmin.Client
	traceClient    *tracepb.Client
)

func init() {
	flag.StringVar(&address, "address", "http://localhost:8080", "address to hit")
	flag.StringVar(&projectID, "project", "", "GCP project used to deploy and run tests")
}

func main() {
	flag.Parse()

	ctx := context.Background()
	var err error
	if _, err = http.Get(address); err != nil {
		log.Fatal(err)
	}

	logadminClient, err = logadmin.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("error creating logadmin client: %v\n", err)
	}
	traceClient, err = tracepb.NewClient(ctx)
	if err != nil {
		log.Fatalf("error creating trace client: %v\n", err)
	}

	tests := []test{
		testRequestlog{
			url: "/requestlog/",
		},
		testTrace{
			url: "/trace/",
		},
	}
	var wg sync.WaitGroup
	wg.Add(len(tests))

	for _, t := range tests {
		go func(t test) {
			defer wg.Done()
			log.Printf("Test %s running\n", t)
			if err := t.Run(); err != nil {
				log.Printf("Test %s failed: %v\n", t, err)
			} else {
				log.Printf("Test %s passed\n", t)
			}
		}(t)
	}

	wg.Wait()
}

type test interface {
	Run() error
}

// get sends a GET request to the address with a suffix of random string.
func get(addr string) (string, error) {
	tok := url.PathEscape(time.Now().Format(time.RFC3339))
	resp, err := http.Get(addr + tok)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error response got: %s", resp.Status)
	}
	return tok, err
}
