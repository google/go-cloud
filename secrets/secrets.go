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

// Package secrets provides an easy and portable way to encrypt and decrypt
// messages. Subpackages contain driver implementations of
// secrets for supported services.
//
// See https://gocloud.dev/howto/secrets/ for a detailed how-to guide.
//
// # OpenTelemetry Integration
//
// OpenTelemetry supports tracing and metric collection for multiple languages and
// backend providers. See https://opentelemetry.io.
//
// This API collects OpenTelemetry traces and metrics for the following methods:
//   - Encrypt
//   - Decrypt
//
// All trace and metric names begin with the package import path.
// The traces add the method name.
// For example, "gocloud.dev/secrets/Encrypt".
// The metrics are "completed_calls", a count of completed method calls by driver,
// method and status (error code); and "latency", a distribution of method latency
// by driver and method.
// For example, "gocloud.dev/secrets/latency".
//
// To enable trace collection in your application, see the OpenTelemetry documentation at
// https://opentelemetry.io/docs/instrumentation/go/getting-started/.
package secrets // import "gocloud.dev/secrets"

import (
	"context"
	"net/url"
	"reflect"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/internal/openurl"
	"gocloud.dev/secrets/driver"
)

// Keeper does encryption and decryption. To create a Keeper, use constructors
// found in driver subpackages.
type Keeper struct {
	k      driver.Keeper
	tracer trace.Tracer

	// mu protects the closed variable.
	// Read locks are kept to allow holding a read lock for long-running calls,
	// and thereby prevent closing until a call finishes.
	mu     sync.RWMutex
	closed bool
}

// NewKeeper is intended for use by drivers only. Do not use in application code.
var NewKeeper = newKeeper

// newKeeper creates a Keeper.
func newKeeper(k driver.Keeper) *Keeper {
	return &Keeper{
		k:      k,
		tracer: otel.GetTracerProvider().Tracer(pkgName),
	}
}

const pkgName = "gocloud.dev/secrets"

var (
	// Meter is the OpenTelemetry meter for this package
	meter = otel.GetMeterProvider().Meter(pkgName)
	
	// latencyHistogram measures the latency of method calls
	latencyHistogram metric.Float64Histogram
	
	// completedCallsCounter counts the number of method calls
	completedCallsCounter metric.Int64Counter
)

func init() {
	var err error
	latencyHistogram, err = meter.Float64Histogram(
		pkgName+"/latency",
		metric.WithDescription("Latency distribution of method calls"),
	)
	if err != nil {
		otel.Handle(err)
	}
	
	completedCallsCounter, err = meter.Int64Counter(
		pkgName+"/completed_calls",
		metric.WithDescription("Count of completed method calls"),
	)
	if err != nil {
		otel.Handle(err)
	}
}

// providerName returns the name of the provider associated with the driver value.
// It is intended to be used for metrics and tracing.
func providerName(driver any) string {
	// Return the package path of the driver's type.
	if driver == nil {
		return ""
	}
	t := reflect.TypeOf(driver)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.PkgPath()
}

// Encrypt encrypts the plaintext and returns the cipher message.
func (k *Keeper) Encrypt(ctx context.Context, plaintext []byte) (ciphertext []byte, err error) {
	start := time.Now()
	ctx, span := k.tracer.Start(ctx, "Encrypt")
	// Set span attributes for testing
	span.SetAttributes(
		attribute.String("gocdk.package", pkgName),
		attribute.String("gocdk.method", "Encrypt"),
	)
	defer func() {
		// Set status on span before ending
		if err != nil {
			span.SetAttributes(attribute.String("gocdk.status", "13")) // Internal error
		} else {
			span.SetAttributes(attribute.String("gocdk.status", "0")) // OK
		}
		span.End()
		// Record metrics
		latency := time.Since(start).Seconds()
		latencyHistogram.Record(ctx, latency, metric.WithAttributes(
			attribute.String("method", "Encrypt"),
			attribute.String("provider", providerName(k.k)),
		))
		
		status := "ok"
		if err != nil {
			status = "error"
		}
		completedCallsCounter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("method", "Encrypt"),
			attribute.String("provider", providerName(k.k)),
			attribute.String("status", status),
		))
	}()

	k.mu.RLock()
	defer k.mu.RUnlock()
	if k.closed {
		return nil, errClosed
	}

	b, err := k.k.Encrypt(ctx, plaintext)
	if err != nil {
		return nil, wrapError(k, err)
	}
	return b, nil
}

// Decrypt decrypts the ciphertext and returns the plaintext.
func (k *Keeper) Decrypt(ctx context.Context, ciphertext []byte) (plaintext []byte, err error) {
	start := time.Now()
	ctx, span := k.tracer.Start(ctx, "Decrypt")
	// Set span attributes for testing
	span.SetAttributes(
		attribute.String("gocdk.package", pkgName),
		attribute.String("gocdk.method", "Decrypt"),
	)
	defer func() {
		// Set status on span before ending
		if err != nil {
			span.SetAttributes(attribute.String("gocdk.status", "13")) // Internal error
		} else {
			span.SetAttributes(attribute.String("gocdk.status", "0")) // OK
		}
		span.End()
		// Record metrics
		latency := time.Since(start).Seconds()
		latencyHistogram.Record(ctx, latency, metric.WithAttributes(
			attribute.String("method", "Decrypt"),
			attribute.String("provider", providerName(k.k)),
		))
		
		status := "ok"
		if err != nil {
			status = "error"
		}
		completedCallsCounter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("method", "Decrypt"),
			attribute.String("provider", providerName(k.k)),
			attribute.String("status", status),
		))
	}()

	k.mu.RLock()
	defer k.mu.RUnlock()
	if k.closed {
		return nil, errClosed
	}

	b, err := k.k.Decrypt(ctx, ciphertext)
	if err != nil {
		return nil, wrapError(k, err)
	}
	return b, nil
}

var errClosed = gcerr.Newf(gcerr.FailedPrecondition, nil, "secrets: Keeper has been closed")

// Close releases any resources used for the Keeper.
func (k *Keeper) Close() error {
	k.mu.Lock()
	prev := k.closed
	k.closed = true
	k.mu.Unlock()
	if prev {
		return errClosed
	}
	return wrapError(k, k.k.Close())
}

// ErrorAs converts i to driver-specific types. See
// https://gocloud.dev/concepts/as/ for background information and the
// driver package documentation for the specific types supported for
// that driver.
//
// ErrorAs panics if i is nil or not a pointer.
// ErrorAs returns false if err == nil.
func (k *Keeper) ErrorAs(err error, i any) bool {
	return gcerr.ErrorAs(err, i, k.k.ErrorAs)
}

func wrapError(k *Keeper, err error) error {
	if err == nil {
		return nil
	}
	if gcerr.DoNotWrap(err) {
		return err
	}
	return gcerr.New(k.k.ErrorCode(err), err, 2, "secrets")
}

// KeeperURLOpener represents types that can open Keepers based on a URL.
// The opener must not modify the URL argument. OpenKeeperURL must be safe to
// call from multiple goroutines.
//
// This interface is generally implemented by types in driver packages.
type KeeperURLOpener interface {
	OpenKeeperURL(ctx context.Context, u *url.URL) (*Keeper, error)
}

// URLMux is a URL opener multiplexer. It matches the scheme of the URLs
// against a set of registered schemes and calls the opener that matches the
// URL's scheme.
// See https://gocloud.dev/concepts/urls/ for more information.
//
// The zero value is a multiplexer with no registered schemes.
type URLMux struct {
	schemes openurl.SchemeMap
}

// KeeperSchemes returns a sorted slice of the registered Keeper schemes.
func (mux *URLMux) KeeperSchemes() []string { return mux.schemes.Schemes() }

// ValidKeeperScheme returns true iff scheme has been registered for Keepers.
func (mux *URLMux) ValidKeeperScheme(scheme string) bool { return mux.schemes.ValidScheme(scheme) }

// RegisterKeeper registers the opener with the given scheme. If an opener
// already exists for the scheme, RegisterKeeper panics.
func (mux *URLMux) RegisterKeeper(scheme string, opener KeeperURLOpener) {
	mux.schemes.Register("secrets", "Keeper", scheme, opener)
}

// OpenKeeper calls OpenKeeperURL with the URL parsed from urlstr.
// OpenKeeper is safe to call from multiple goroutines.
func (mux *URLMux) OpenKeeper(ctx context.Context, urlstr string) (*Keeper, error) {
	opener, u, err := mux.schemes.FromString("Keeper", urlstr)
	if err != nil {
		return nil, err
	}
	return opener.(KeeperURLOpener).OpenKeeperURL(ctx, u)
}

// OpenKeeperURL dispatches the URL to the opener that is registered with the
// URL's scheme. OpenKeeperURL is safe to call from multiple goroutines.
func (mux *URLMux) OpenKeeperURL(ctx context.Context, u *url.URL) (*Keeper, error) {
	opener, err := mux.schemes.FromURL("Keeper", u)
	if err != nil {
		return nil, err
	}
	return opener.(KeeperURLOpener).OpenKeeperURL(ctx, u)
}

var defaultURLMux = new(URLMux)

// DefaultURLMux returns the URLMux used by OpenKeeper.
//
// Driver packages can use this to register their KeeperURLOpener on the mux.
func DefaultURLMux() *URLMux {
	return defaultURLMux
}

// OpenKeeper opens the Keeper identified by the URL given.
// See the URLOpener documentation in driver subpackages for
// details on supported URL formats, and https://gocloud.dev/concepts/urls
// for more information.
func OpenKeeper(ctx context.Context, urlstr string) (*Keeper, error) {
	return defaultURLMux.OpenKeeper(ctx, urlstr)
}
