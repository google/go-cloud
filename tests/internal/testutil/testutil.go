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

// Package testutil contains utility functions used by server tests against
// different platforms.
package testutil

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"
)

const (
	startDelay = 5 * time.Second
	maxBackoff = time.Minute
)

// Retry keeps retrying hitting url using f with exponentially increased delay
// until the delay passes maxBackoff.
func Retry(t *testing.T, s string, f func(string) error) {
	var err error
	for d := startDelay; d <= maxBackoff; d *= 2 {
		time.Sleep(d)
		if err = f(s); err == nil {
			return
		}
	}
	t.Fatalf("maximum retries reached: %v", err)
}

// Get sends a GET request to the address. It returns error for any non 200
// status.
func Get(addr string) error {
	resp, err := http.Get(addr)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error response got: %s", resp.Status)
	}
	return err
}

// URLSuffix append a human-readible suffix to the test URL.
func URLSuffix(addr string) string {
	return url.PathEscape(time.Now().Format(time.RFC3339))
}
