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
	"log"
	"net/http"
	"sync"

	"cloud.google.com/go/logging/logadmin"
	tracepb "cloud.google.com/go/trace/apiv1"
	"github.com/google/go-cloud/tests"
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

	testCases := []tests.Test{
		testRequestlog{
			url: "/requestlog/",
		},
		testTrace{
			url: "/trace/",
		},
	}
	var wg sync.WaitGroup
	wg.Add(len(testCases))

	for _, tc := range testCases {
		go func(t tests.Test) {
			defer wg.Done()
			log.Printf("Test %s running\n", t)
			if err := t.Run(); err != nil {
				log.Printf("Test %s failed: %v\n", t, err)
			} else {
				log.Printf("Test %s passed\n", t)
			}
		}(tc)
	}

	wg.Wait()
}
