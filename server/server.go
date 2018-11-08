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

// Package server provides a preconfigured HTTP server with diagnostic hooks.
package server

import (
	"context"
	"net/http"
	"path"
	"sync"
	"time"

	"github.com/google/go-cloud/health"
	"github.com/google/go-cloud/requestlog"
	"github.com/google/go-cloud/server/driver"
	"github.com/google/go-cloud/wire"

	"go.opencensus.io/trace"
)

// Set is a Wire provider set that produces a *Server given the fields of
// Options. This set might add new inputs over time, but they can always be the
// zero value.
var Set = wire.NewSet(
	New,
	Options{},
	wire.Value(&DefaultDriver{}),
	wire.Bind(new(driver.Server), new(DefaultDriver)),
)

// Server is a preconfigured HTTP server with diagnostic hooks.
// The zero value is a server with the default options.
type Server struct {
	reqlog        requestlog.Logger
	healthHandler health.Handler
	te            trace.Exporter
	sampler       trace.Sampler
	once          sync.Once
	driver        driver.Server
}

// Options is the set of optional parameters.
type Options struct {
	// RequestLogger specifies the logger that will be used to log requests.
	RequestLogger requestlog.Logger

	// HealthChecks specifies the health checks to be run when the
	// /healthz/readiness endpoint is requested.
	HealthChecks []health.Checker

	// TraceExporter exports sampled trace spans.
	TraceExporter trace.Exporter

	// DefaultSamplingPolicy is a function that takes a
	// trace.SamplingParameters struct and returns a true or false decision about
	// whether it should be sampled and exported.
	DefaultSamplingPolicy trace.Sampler

	// Driver serves HTTP requests.
	Driver driver.Server
}

// New creates a new server. New(nil) is the same as new(Server).
func New(opts *Options) *Server {
	srv := new(Server)
	if opts != nil {
		srv.reqlog = opts.RequestLogger
		srv.te = opts.TraceExporter
		for _, c := range opts.HealthChecks {
			srv.healthHandler.Add(c)
		}
		srv.sampler = opts.DefaultSamplingPolicy
		srv.driver = opts.Driver
	}
	return srv
}

func (srv *Server) init() {
	srv.once.Do(func() {
		if srv.te != nil {
			trace.RegisterExporter(srv.te)
		}
		if srv.sampler != nil {
			trace.ApplyConfig(trace.Config{DefaultSampler: srv.sampler})
		}
		if srv.driver == nil {
			srv.driver = NewDefaultDriver()
		}
	})
}

// ListenAndServe is a wrapper to use wherever http.ListenAndServe is used.
// It wraps the passed-in http.Handler with a handler that handles tracing and
// request logging. If the handler is nil, then http.DefaultServeMux will be used.
// A configured Requestlogger will log all requests except HealthChecks.
func (srv *Server) ListenAndServe(addr string, h http.Handler) error {
	srv.init()

	// Setup health checks, /healthz route is taken by health checks by default.
	// Note: App Engine Flex uses /_ah/health by default, which can be changed
	// in app.yaml. We may want to do an auto-detection for flex in future.
	hr := "/healthz/"
	hcMux := http.NewServeMux()
	hcMux.HandleFunc(path.Join(hr, "liveness"), health.HandleLive)
	hcMux.Handle(path.Join(hr, "readiness"), &srv.healthHandler)

	mux := http.NewServeMux()
	mux.Handle(hr, hcMux)
	h = http.Handler(handler{h})
	if srv.reqlog != nil {
		h = requestlog.NewHandler(srv.reqlog, h)
	}
	mux.Handle("/", h)

	return srv.driver.ListenAndServe(addr, mux)
}

// Shutdown gracefully shuts down the server without interrupting any active connections.
func (srv *Server) Shutdown(ctx context.Context) error {
	if srv.driver == nil {
		return nil
	}
	return srv.driver.Shutdown(ctx)
}

// handler is a handler wrapper that handles tracing through OpenCensus for users.
// TODO(shantuo): unify handler types from trace, requestlog, health checks, etc together.
type handler struct {
	handler http.Handler
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), r.URL.Host+r.URL.Path)
	defer span.End()

	r = r.WithContext(ctx)

	if h.handler == nil {
		h.handler = http.DefaultServeMux
	}
	h.handler.ServeHTTP(w, r)
}

// DefaultDriver implements the driver.Server interface. The zero value is a valid http.Server.
type DefaultDriver struct {
	Server http.Server
}

// NewDefaultDriver creates a driver with an http.Server with default timeouts.
func NewDefaultDriver() *DefaultDriver {
	return &DefaultDriver{
		Server: http.Server{
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
	}
}

// ListenAndServe sets the address and handler on DefaultDriver's http.Server,
// then calls ListenAndServe on it.
func (dd *DefaultDriver) ListenAndServe(addr string, h http.Handler) error {
	dd.Server.Addr = addr
	dd.Server.Handler = h
	return dd.Server.ListenAndServe()
}

// Shutdown gracefully shuts down the server without interrupting any active connections,
// by calling Shutdown on DefaultDriver's http.Server
func (dd *DefaultDriver) Shutdown(ctx context.Context) error {
	return dd.Server.Shutdown(ctx)
}
