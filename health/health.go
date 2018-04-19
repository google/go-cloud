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

// Package health provides health check handlers.
package health

import (
	"io"
	"net/http"
)

// Handler is an HTTP handler that reports on the success of an
// aggregate of Checkers.  The zero value is always healthy.
type Handler struct {
	checkers []Checker
}

// Add adds a new check to the handler.
func (h *Handler) Add(c Checker) {
	h.checkers = append(h.checkers, c)
}

// ServeHTTP returns 200 if it is healthy, 500 otherwise.
func (h *Handler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	for _, c := range h.checkers {
		if err := c.CheckHealth(); err != nil {
			writeUnhealthy(w)
			return
		}
	}
	writeHealthy(w)
}

func writeUnhealthy(w http.ResponseWriter) {
	w.Header().Set("Content-Length", "9")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusInternalServerError)
	io.WriteString(w, "unhealthy")
}

// HandleLive is an http.HandleFunc that handles liveness checks by
// immediately responding with an HTTP 200 status.
func HandleLive(w http.ResponseWriter, _ *http.Request) {
	writeHealthy(w)
}

func writeHealthy(w http.ResponseWriter) {
	w.Header().Set("Content-Length", "2")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, "ok")
}

// Checker wraps the CheckHealth method.
//
// CheckHealth returns nil if the resource is healthy, or a non-nil
// error if the resource is not healthy.  CheckHealth must be safe to
// call from multiple goroutines.
type Checker interface {
	CheckHealth() error
}
