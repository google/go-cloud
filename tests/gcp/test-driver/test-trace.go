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

package main

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/api/iterator"
	cloudtracepb "google.golang.org/genproto/googleapis/devtools/cloudtrace/v1"
)

type testTrace struct {
	url string
}

func (t testTrace) Run() error {
	tok, err := get(address + t.url)
	if err != nil {
		return fmt.Errorf("error sending request: %v", err)
	}

	time.Sleep(time.Minute)
	return t.readTrace(tok)
}

func (t testTrace) readTrace(tok string) error {
	req := &cloudtracepb.ListTracesRequest{
		ProjectId: projectID,
		Filter:    fmt.Sprintf("+root:%s%s", t.url, tok),
	}
	it := traceClient.ListTraces(context.Background(), req)
	_, err := it.Next()
	if err == iterator.Done {
		return fmt.Errorf("no trace found for %s", tok)
	}
	if err != nil {
		return err
	}
	return nil
}

func (t testTrace) String() string {
	return fmt.Sprintf("%T:%s", t, t.url)
}
