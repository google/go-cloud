// Copyright 2018 The Go Cloud Development Kit Authors
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
package server // import "gocloud.dev/server"

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/wire"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
	
	"gocloud.dev/server/driver"
	"gocloud.dev/server/health"
	"gocloud.dev/server/requestlog"
)

// Set is a Wire provider set that produces a *Server given the fields of
// Options.
var Set = wire.NewSet(
	New,
	wire.Struct(new(Options), "RequestLogger", "HealthChecks", "TraceExporter", "DefaultSamplingPolicy", "TracerProvider", "Driver"),
	wire.Value(&DefaultDriver{}),
	wire.Bind(new(driver.Server), new(*DefaultDriver)),
)

// Server is a preconfigured HTTP server with diagnostic hooks.
// The zero value is a server with the default options.
type Server struct {
	reqlog         requestlog.Logger
	handler        http.Handler
	wrappedHandler http.Handler
	healthHandler  health.Handler
	te             sdktrace.SpanExporter
	sampler        sdktrace.Sampler
	tracerProvider trace.TracerProvider
	once           sync.Once
	driver         driver.Server
}

// Options is the set of optional parameters.
type Options struct {
	// RequestLogger specifies the logger that will be used to log requests.
	RequestLogger requestlog.Logger

	// HealthChecks specifies the health checks to be run when the
	// /healthz/readiness endpoint is requested.
	HealthChecks []health.Checker

	// TraceExporter exports sampled trace spans.
	// Deprecated: Use TracerProvider instead. If both are provided, TracerProvider takes precedence.
	TraceExporter sdktrace.SpanExporter

	// DefaultSamplingPolicy is a function that takes a
	// trace.SpanStartOptions struct and returns a decision about
	// whether it should be sampled and exported.
	// Deprecated: Use TracerProvider instead. If TracerProvider is provided, this is ignored.
	DefaultSamplingPolicy sdktrace.Sampler

	// TracerProvider is a configured OpenTelemetry TracerProvider.
	// If set, this is used directly instead of creating one from TraceExporter and DefaultSamplingPolicy.
	TracerProvider trace.TracerProvider

	// Driver serves HTTP requests.
	Driver driver.Server
}

// New creates a new server. New(nil, nil) is the same as new(Server).
func New(h http.Handler, opts *Options) *Server {
	srv := &Server{handler: h}
	if opts != nil {
		srv.reqlog = opts.RequestLogger
		srv.te = opts.TraceExporter
		for _, c := range opts.HealthChecks {
			srv.healthHandler.Add(c)
		}
		srv.sampler = opts.DefaultSamplingPolicy
		srv.tracerProvider = opts.TracerProvider
		srv.driver = opts.Driver
	}
	return srv
}

func (srv *Server) init() {
	srv.once.Do(func() {
		// If a TracerProvider was provided, use it directly
		if srv.tracerProvider != nil {
			// Set the global tracer provider if a custom one was provided
			otel.SetTracerProvider(srv.tracerProvider)
			otel.SetTextMapPropagator(propagation.TraceContext{})
		} else if srv.te != nil {
			// Legacy approach: create a provider from an exporter
			// Create resource
			res, err := resource.New(context.Background(),
				resource.WithTelemetrySDK(),
				resource.WithAttributes(
					semconv.ServiceNameKey.String("gocloud-server"),
				),
			)
			if err == nil {
				// Set up OpenTelemetry tracer provider with the exporter
				tp := sdktrace.NewTracerProvider(
					sdktrace.WithBatcher(srv.te),
					sdktrace.WithResource(res),
				)
				
				if srv.sampler != nil {
					// Create a new provider with the sampler
					tp = sdktrace.NewTracerProvider(
						sdktrace.WithBatcher(srv.te),
						sdktrace.WithSampler(srv.sampler),
						sdktrace.WithResource(res),
					)
				}
				
				// Set the global tracer provider
				otel.SetTracerProvider(tp)
				otel.SetTextMapPropagator(propagation.TraceContext{})
			}
		}
		if srv.driver == nil {
			srv.driver = NewDefaultDriver()
		}
		if srv.handler == nil {
			srv.handler = http.DefaultServeMux
		}
		// Setup health checks, /healthz route is taken by health checks by default.
		// Note: App Engine Flex uses /_ah/health by default, which can be changed
		// in app.yaml. We may want to do an auto-detection for flex in future.
		const healthPrefix = "/healthz/"

		mux := http.NewServeMux()
		mux.HandleFunc(healthPrefix+"liveness", health.HandleLive)
		mux.Handle(healthPrefix+"readiness", &srv.healthHandler)
		h := srv.handler
		if srv.reqlog != nil {
			h = requestlog.NewHandler(srv.reqlog, h)
		}
		// Wrap with OpenTelemetry HTTP handler
		h = otelhttp.NewHandler(h, "go-cloud-server")
		mux.Handle("/", h)
		srv.wrappedHandler = mux
	})
}

// ListenAndServe is a wrapper to use wherever http.ListenAndServe is used.
// It wraps the http.Handler provided to New with a handler that handles tracing and
// request logging. If the handler is nil, then http.DefaultServeMux will be used.
// A configured Requestlogger will log all requests except HealthChecks.
func (srv *Server) ListenAndServe(addr string) error {
	srv.init()
	return srv.driver.ListenAndServe(addr, srv.wrappedHandler)
}

// ListenAndServeTLS is a wrapper to use wherever http.ListenAndServeTLS is used.
// It wraps the http.Handler provided to New with a handler that handles tracing and
// request logging. If the handler is nil, then http.DefaultServeMux will be used.
// A configured Requestlogger will log all requests except HealthChecks.
func (srv *Server) ListenAndServeTLS(addr, certFile, keyFile string) error {
	// Check if the driver implements the optional interface.
	tlsDriver, ok := srv.driver.(driver.TLSServer)
	if !ok {
		return fmt.Errorf("driver %T does not support ListenAndServeTLS", srv.driver)
	}
	srv.init()
	return tlsDriver.ListenAndServeTLS(addr, certFile, keyFile, srv.wrappedHandler)
}

// Shutdown gracefully shuts down the server without interrupting any active connections.
func (srv *Server) Shutdown(ctx context.Context) error {
	if srv.driver == nil {
		return nil
	}
	return srv.driver.Shutdown(ctx)
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

// ListenAndServeTLS sets the address and handler on DefaultDriver's http.Server,
// then calls ListenAndServeTLS on it.
//
// DefaultDriver.Server.TLSConfig may be set to configure additional TLS settings.
func (dd *DefaultDriver) ListenAndServeTLS(addr, certFile, keyFile string, h http.Handler) error {
	dd.Server.Addr = addr
	dd.Server.Handler = h
	return dd.Server.ListenAndServeTLS(certFile, keyFile)
}

// Shutdown gracefully shuts down the server without interrupting any active connections,
// by calling Shutdown on DefaultDriver's http.Server
func (dd *DefaultDriver) Shutdown(ctx context.Context) error {
	return dd.Server.Shutdown(ctx)
}
