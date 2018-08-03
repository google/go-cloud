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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const (
	startDelay = 5 * time.Second
	maxBackoff = time.Minute
)

// Logger writes logs (normally error messages) during retries.
type Logger interface {
	Logf(format string, args ...interface{})
}

// Retry keeps calling f with exponentially increased waiting time between tries
// until it no longer returns an error, or the delay passes maxBackoff.
func Retry(logger Logger, f func() error) error {
	var err error
	for d := startDelay; ; {
		time.Sleep(d)
		if err = f(); err == nil {
			return nil
		}
		if d = d * 2; d > maxBackoff {
			break
		}
		logger.Logf("%v, retrying in %v", err, d)
	}
	return fmt.Errorf("maximum retries reached: %v", err)
}

// Get sends a GET request to the address. It returns error for any non 200
// status.
func Get(addr string) func() error {
	return func() error {
		resp, err := http.Get(addr)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("error response got: %s", resp.Status)
		}
		return err
	}
}

// URLSuffix returns a human-readable suffix.
func URLSuffix() (string, error) {
	t := url.PathEscape(time.Now().Format(time.RFC3339))
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return t + "/" + hex.EncodeToString(b), nil
}
