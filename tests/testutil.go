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

// Package tests contains some common interfaces and functions shared by the
// underlying platform specific tests.
package tests

import (
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type Test interface {
	Run() error
}

// TestGet sends a GET request to the address with a suffix of random string.
func TestGet(addr string) (string, error) {
	tok := url.PathEscape(time.Now().Format(time.RFC3339))
	resp, err := http.Get(addr + tok)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error response got: %s", resp.Status)
	}
	return tok, err
}
