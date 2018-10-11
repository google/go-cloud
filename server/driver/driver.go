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

// Package driver defines an interface for custom HTTP listeners.
package driver

import (
	"context"
	"net/http"
)

// A type that implements Server dispatches requests to an http.Handler.
type Server interface {
	// ListenAndServe listens on the TCP network address addr and then
	// calls Serve with handler to handle requests on incoming connections.
	ListenAndServe(addr string, h http.Handler) error
	// Shutdown gracefully shuts down the server without interrupting
	// any active connections.
	Shutdown(ctx context.Context) error
}
