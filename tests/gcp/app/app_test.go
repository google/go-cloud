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
	"testing"

	"cloud.google.com/go/logging/logadmin"
	tracepb "cloud.google.com/go/trace/apiv1"
	"github.com/google/go-cloud/tests/internal/testutil"
	"google.golang.org/api/iterator"
	cloudtracepb "google.golang.org/genproto/googleapis/devtools/cloudtrace/v1"
)

const (
	requestlogURL = "/requestlog/"
	traceURL      = "/trace/"
)

var (
	address   string
	projectID string
)

func init() {
	flag.StringVar(&address, "address", "http://localhost:8080", "address to hit")
	flag.StringVar(&projectID, "project", "", "GCP project used to deploy and run tests")
}

func TestRequestLog(t *testing.T) {
	t.Parallel()
	tok := testutil.URLSuffix(address + requestlogURL)
	testutil.Retry(t, address+requestlogURL+tok, testutil.Get)
	testutil.Retry(t, tok, readLogEntries)
}

func readLogEntries(tok string) error {
	ctx := context.Background()
	logadminClient, err := logadmin.NewClient(ctx, projectID)
	if err != nil {
		return fmt.Errorf("error creating logadmin client: %v", err)
	}

	iter := logadminClient.Entries(context.Background(),
		logadmin.ProjectIDs([]string{projectID}),
		logadmin.Filter(fmt.Sprintf(`httpRequest.requestUrl = "%s%s"`, requestlogURL, tok)),
	)
	_, err = iter.Next()
	if err == iterator.Done {
		return fmt.Errorf("no entry found for request log that matches %s", tok)
	}
	return err
}

func TestTrace(t *testing.T) {
	t.Parallel()
	tok := testutil.URLSuffix(address + traceURL)
	testutil.Retry(t, address+traceURL+tok, testutil.Get)
	testutil.Retry(t, tok, readTrace)
}

func readTrace(tok string) error {
	ctx := context.Background()
	traceClient, err := tracepb.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("error creating trace client: %v\n", err)
	}

	req := &cloudtracepb.ListTracesRequest{
		ProjectId: projectID,
		Filter:    fmt.Sprintf("+root:%s%s", traceURL, tok),
	}
	it := traceClient.ListTraces(context.Background(), req)
	_, err = it.Next()
	if err == iterator.Done {
		return fmt.Errorf("no trace found for %s", tok)
	}
	return err
}
