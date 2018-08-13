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

package main_test

import (
	"context"
	"flag"
	"fmt"
	"path"
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
	flag.StringVar(&address, "address", "", "address to hit")
	flag.StringVar(&projectID, "project", "", "GCP project used to deploy and run tests")
}

func TestRequestLog(t *testing.T) {
	t.Parallel()
	if address == "" || projectID == "" {
		t.Skip("Both address and project need to be setup to run server tests.")
	}
	suf, err := testutil.URLSuffix()
	if err != nil {
		t.Fatal("cannot generate URL:", err)
	}
	p := path.Clean(fmt.Sprintf("/%s/%s", requestlogURL, suf))
	if err := testutil.Retry(t, testutil.Get(address+p)); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	c, err := logadmin.NewClient(ctx, projectID)
	if err != nil {
		t.Fatalf("error creating logadmin client: %v", err)
	}
	defer c.Close()

	if err := testutil.Retry(t, readLogEntries(ctx, c, p)); err != nil {
		t.Error(err)
	}
}

func readLogEntries(ctx context.Context, c *logadmin.Client, u string) func() error {
	return func() error {
		iter := c.Entries(context.Background(),
			logadmin.ProjectIDs([]string{projectID}),
			logadmin.Filter(fmt.Sprintf(`httpRequest.requestUrl = %q`, u)),
		)
		_, err := iter.Next()
		if err == iterator.Done {
			return fmt.Errorf("no entry found for request log that matches %q", u)
		}
		return err
	}
}

func TestTrace(t *testing.T) {
	t.Parallel()
	if address == "" || projectID == "" {
		t.Skip("Both address and project need to be setup to run server tests.")
	}
	suf, err := testutil.URLSuffix()
	if err != nil {
		t.Fatal("cannot generate URL:", err)
	}
	p := path.Clean(fmt.Sprintf("/%s/%s", traceURL, suf))
	if err := testutil.Retry(t, testutil.Get(address+p)); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	c, err := tracepb.NewClient(ctx)
	if err != nil {
		t.Fatalf("error creating trace client: %v\n", err)
	}
	defer c.Close()
	if err := testutil.Retry(t, readTrace(ctx, c, p)); err != nil {
		t.Error(err)
	}
}

func readTrace(ctx context.Context, c *tracepb.Client, u string) func() error {
	return func() error {
		req := &cloudtracepb.ListTracesRequest{
			ProjectId: projectID,
			Filter:    fmt.Sprintf("+root:%s", u),
		}
		it := c.ListTraces(context.Background(), req)
		_, err := it.Next()
		if err == iterator.Done {
			return fmt.Errorf("no trace found for %q", u)
		}
		return err
	}
}
