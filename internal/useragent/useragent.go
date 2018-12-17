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

// Package useragent includes constants and utilitiesfor setting the User-Agent
// for Go Cloud connections to GCP.
package useragent // import "gocloud.dev/internal/useragent"

import (
	"net/http"
)

// GoCloudUserAgent is the User-Agent to set for Go Cloud requests to GCP.
const GoCloudUserAgent = "go-cloud/0.1"

// userAgentTransport wraps an http.RoundTripper, adding a User-Agent header
// to each request.
type userAgentTransport struct {
	base http.RoundTripper
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request to avoid mutating it.
	newReq := *req
	newReq.Header = make(http.Header)
	for k, vv := range req.Header {
		newReq.Header[k] = vv
	}
	// Append to the User-Agent string to preserve other information.
	newReq.Header.Set("User-Agent", req.UserAgent()+" "+GoCloudUserAgent)
	return t.base.RoundTrip(&newReq)
}

// HTTPClient wraps client and appends GoCloudUserAgent to the User-Agent header
// for all requests.
func HTTPClient(client *http.Client) *http.Client {
	c := *client
	c.Transport = &userAgentTransport{base: c.Transport}
	return &c
}
