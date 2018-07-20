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

// Package internal contains functions and types shared by tests on different
// platforms under tests.
package internal

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	startDelay = 5 * time.Second
	maxBackoff = time.Minute
)

type Test interface {
	Run() error
}

// RunTests runs all tests concurrently, prints the results and returns whether
// there is any failure.
func RunTests(tests []Test) (ok bool) {
	var wg sync.WaitGroup
	wg.Add(len(tests))
	ok = true

	for _, tc := range tests {
		go func(t Test) {
			defer wg.Done()
			log.Printf("Test %s running...\n", t)
			if err := t.Run(); err != nil {
				log.Printf("Test %s failed: %v\n", t, err)
				ok = false
			}
			log.Printf("Test %s passed\n", t)
		}(tc)
	}

	wg.Wait()
	return ok
}

// Retry keeps retrying hitting url using f with exponentially increased delay
// until the delay passes maxBackoff.
func Retry(url string, f func(string) error) error {
	var err error
	for d := startDelay; d <= maxBackoff; d *= 2 {
		time.Sleep(d)
		err = f(url)
		if err == nil {
			return nil
		}
	}
	return err
}

// TestGet sends a GET request to the address with a suffix of the current time.
func TestGet(addr string) (string, error) {
	tok := url.PathEscape(time.Now().Format(time.RFC3339))
	resp, err := http.Get(addr + tok)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error response got: %s", resp.Status)
	}
	return tok, err
}
