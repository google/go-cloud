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

package main_test

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"cloud.google.com/go/logging/logadmin"
	tracepb "cloud.google.com/go/trace/apiv1"
	"google.golang.org/api/iterator"
	cloudtracepb "google.golang.org/genproto/googleapis/devtools/cloudtrace/v1"
)

var (
	address   string
	projectID string
)

func init() {
	flag.StringVar(&address, "address", "http://localhost:8080", "address to hit")
	flag.StringVar(&projectID, "project", "", "GCP project used to deploy and run tests")
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

func TestRequestlog(t *testing.T) {
	t.Parallel()
	const url = "/requestlog/"
	tok, err := get(address + url)
	if err != nil {
		t.Fatal("error sending request:", err)
	}

	time.Sleep(time.Minute)
	readLogEntries(context.Background(), t, url, tok)
}

func readLogEntries(ctx context.Context, t *testing.T, url, tok string) {
	logadminClient, err := logadmin.NewClient(ctx, projectID)
	if err != nil {
		t.Fatal("error creating logadmin client:", err)
	}

	iter := logadminClient.Entries(context.Background(),
		logadmin.Filter(fmt.Sprintf(`timestamp >= %q`, tok)),
		logadmin.Filter(fmt.Sprintf(`httpRequest.requestUrl = "%s%s"`, url, tok)),
	)
	_, err = iter.Next()
	if err == iterator.Done {
		t.Errorf("no entry found for request log that matches %s", tok)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestTrace(t *testing.T) {
	t.Parallel()
	const url = "/trace/"
	tok, err := get(address + url)
	if err != nil {
		t.Fatal("error sending request:", err)
	}

	time.Sleep(time.Minute)
	readTrace(context.Background(), t, url, tok)
}

func readTrace(ctx context.Context, t *testing.T, url, tok string) {
	traceClient, err := tracepb.NewClient(ctx)
	if err != nil {
		t.Fatalf("error creating trace client: %v\n", err)
	}

	req := &cloudtracepb.ListTracesRequest{
		ProjectId: projectID,
		Filter:    fmt.Sprintf("+root:%s%s", url, tok),
	}
	it := traceClient.ListTraces(context.Background(), req)
	_, err = it.Next()
	if err == iterator.Done {
		t.Errorf("no trace found for %s", tok)
	}
	if err != nil {
		t.Fatal(err)
	}
}
