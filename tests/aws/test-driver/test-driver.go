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
// tests features initialized by the AWS server packages.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"sync"

	"cloud.google.com/go/logging/logadmin"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/xray"
	"github.com/google/go-cloud/tests"
)

var (
	address        string
	awsRegion      string
	gcpProjectID   string
	logadminClient *logadmin.Client
	xrayClient     *xray.XRay
)

func init() {
	flag.StringVar(&address, "address", "http://localhost:8080", "address to hit")
	flag.StringVar(&awsRegion, "aws-region", "us-west-1", "the region used to run the sample app")
	flag.StringVar(&gcpProjectID, "gcp-project", "", "GCP project used to collect request logs")
}

func main() {
	flag.Parse()

	ctx := context.Background()
	var err error
	if _, err = http.Get(address); err != nil {
		log.Fatal(err)
	}

	logadminClient, err = logadmin.NewClient(ctx, gcpProjectID)
	if err != nil {
		log.Fatalf("error creating logadmin client: %v\n", err)
	}

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion),
	})
	if err != nil {
		log.Fatalf("error creating an AWS session: %v\n", err)
	}
	xrayClient = xray.New(sess)

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
