// Copyright 2019 The Go Cloud Development Kit Authors
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

// Package probe provides a function for waiting for a server endpoint to
// become available.
package probe

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/xerrors"
)

// WaitForHealthy polls a URL repeatedly until the server responds with a
// non-server-error status, the context is canceled, or the context's deadline
// is met. WaitForHealthy returns an error in the latter two cases.
func WaitForHealthy(ctx context.Context, u *url.URL) error {
	// Create health request.
	// (Avoiding http.NewRequest to not reparse the URL.)
	req := &http.Request{
		Method: http.MethodGet,
		URL:    u,
	}
	req = req.WithContext(ctx)

	// Poll for healthy.
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		// Check response. Allow 200-level (success) or 400-level (client error)
		// status codes. The latter is permitted for the case where the application
		// doesn't serve explicit health checks.
		resp, err := http.DefaultClient.Do(req)
		if err == nil && (statusCodeInRange(resp.StatusCode, 200) || statusCodeInRange(resp.StatusCode, 400)) {
			return nil
		}
		// Wait for the next tick.
		select {
		case <-tick.C:
		case <-ctx.Done():
			return xerrors.Errorf("wait for healthy: %w", ctx.Err())
		}
	}
}

// statusCodeInRange reports whether the given HTTP status code is in the range
// [start, start+100).
func statusCodeInRange(statusCode, start int) bool {
	if start < 100 || start%100 != 0 {
		panic("statusCodeInRange start must be a multiple of 100")
	}
	return start <= statusCode && statusCode < start+100
}
