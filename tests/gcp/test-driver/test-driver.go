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
	"os"
	"time"

	"github.com/google/go-cloud/tests/internal"
	"google.golang.org/api/option"
	"google.golang.org/grpc"

	"cloud.google.com/go/logging/logadmin"
	tracepb "cloud.google.com/go/trace/apiv1"
)

const dialTimeout = 5 * time.Second

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
	err := internal.Retry(address, func(url string) error {
		client := http.Client{
			Timeout: dialTimeout,
		}
		_, err := client.Get(url)
		return err
	})
	if err != nil {
		log.Fatal(err)
	}

	opt := option.WithGRPCDialOption(grpc.WithTimeout(dialTimeout))
	logadminClient, err = logadmin.NewClient(ctx, projectID, opt)
	if err != nil {
		log.Fatalf("error creating logadmin client: %v\n", err)
	}
	traceClient, err = tracepb.NewClient(ctx, opt)
	if err != nil {
		log.Fatalf("error creating trace client: %v\n", err)
	}

	tests := []internal.Test{
		testRequestlog{
			url: "/requestlog/",
		},
		testTrace{
			url: "/trace/",
		},
	}
	if !internal.RunTests(tests) {
		os.Exit(1)
	}
}
