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
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"gocloud.dev/server/health"
	"golang.org/x/xerrors"
)

func TestWaitForHealthy(t *testing.T) {
	t.Run("ImmediateHealthy", func(t *testing.T) {
		srv := httptest.NewServer(new(health.Handler))
		defer srv.Close()
		u, err := url.Parse(srv.URL)
		if err != nil {
			t.Fatal(err)
		}

		if err := WaitForHealthy(context.Background(), u); err != nil {
			t.Error("waitForHealthy returned error:", err)
		}
	})
	t.Run("404", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		defer srv.Close()
		u, err := url.Parse(srv.URL)
		if err != nil {
			t.Fatal(err)
		}

		if err := WaitForHealthy(context.Background(), u); err != nil {
			t.Error("waitForHealthy returned error:", err)
		}
	})
	t.Run("SecondTimeHealthy", func(t *testing.T) {
		handler := new(health.Handler)
		handler.Add(new(secondTimeHealthy))
		srv := httptest.NewServer(handler)
		defer srv.Close()
		u, err := url.Parse(srv.URL)
		if err != nil {
			t.Fatal(err)
		}

		if err := WaitForHealthy(context.Background(), u); err != nil {
			t.Error("waitForHealthy returned error:", err)
		}
	})
	t.Run("ContextCancelled", func(t *testing.T) {
		handler := new(health.Handler)
		handler.Add(constHealthChecker{xerrors.New("bad")})
		srv := httptest.NewServer(handler)
		defer srv.Close()
		u, err := url.Parse(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)

		err = WaitForHealthy(ctx, u)
		cancel()
		if err == nil {
			t.Error("waitForHealthy did not return error")
		}
	})
}

type constHealthChecker struct {
	err error
}

func (c constHealthChecker) CheckHealth() error {
	return c.err
}

type secondTimeHealthy struct {
	mu            sync.Mutex
	firstReported bool
}

func (c *secondTimeHealthy) CheckHealth() error {
	defer c.mu.Unlock()
	c.mu.Lock()
	if !c.firstReported {
		c.firstReported = true
		return xerrors.New("check again later")
	}
	return nil
}
