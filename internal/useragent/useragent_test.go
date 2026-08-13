// Copyright 2026 The Go Cloud Development Kit Authors
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

package useragent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// gotUserAgentHeader carries the User-Agent the test server saw back to the
// test in the response, rather than in a variable shared with the handler
// goroutine.
const gotUserAgentHeader = "X-Got-User-Agent"

func TestHTTPClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(gotUserAgentHeader, r.UserAgent())
	}))
	defer srv.Close()

	tests := []struct {
		name   string
		client *http.Client
	}{
		// A client with no Transport uses http.DefaultTransport implicitly.
		// Wrapping it must not panic.
		{"nil Transport", &http.Client{}},
		{"http.DefaultClient", http.DefaultClient},
		{"explicit Transport", srv.Client()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := HTTPClient(test.client, "blob")
			resp, err := client.Get(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			gotUserAgent := resp.Header.Get(gotUserAgentHeader)
			if err := resp.Body.Close(); err != nil {
				t.Errorf("failed to close the response body: %v", err)
			}
			if want := userAgentString("blob"); !strings.Contains(gotUserAgent, want) {
				t.Errorf("got User-Agent %q, want it to contain %q", gotUserAgent, want)
			}
		})
	}
}

// TestHTTPClientDoesNotMutate verifies that wrapping leaves the caller's client
// alone; it is shared and may be used for other purposes.
func TestHTTPClientDoesNotMutate(t *testing.T) {
	original := &http.Client{}
	if got := HTTPClient(original, "blob"); got == original {
		t.Error("got the same *http.Client back, want a copy")
	}
	if original.Transport != nil {
		t.Errorf("got Transport %v on the original client, want it left nil", original.Transport)
	}
}
