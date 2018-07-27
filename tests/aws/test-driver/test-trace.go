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
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/xray"
	"github.com/google/go-cloud/tests"
)

type testTrace struct {
	url string
}

func (t testTrace) Run() error {
	tok, err := tests.TestGet(serverAddr + t.url)
	if err != nil {
		return fmt.Errorf("error sending request: %v", err)
	}

	time.Sleep(time.Minute)
	return t.readTrace(tok)
}

func (t testTrace) readTrace(tok string) error {
	now := time.Now()
	// This filter ideally should have a filter on the tok but the trace summary
	// sent to X-Ray doesn't match the client library on this part ("name" vs
	// "http.url").
	out, err := xrayClient.GetTraceSummaries(&xray.GetTraceSummariesInput{
		StartTime: aws.Time(now.Add(-time.Minute)),
		EndTime:   aws.Time(now),
	})
	if err != nil {
		return err
	}
	if len(out.TraceSummaries) == 0 {
		return fmt.Errorf("no trace found for %s", tok)
	}
	return nil
}

func (t testTrace) String() string {
	return fmt.Sprintf("%T:%s", t, t.url)
}
