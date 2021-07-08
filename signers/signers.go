// Copyright 2021 The Go Cloud Development Kit Authors
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
//  limitations under the License.
//

// Package signers provides an easy and portable way to sign and verify
// digests. Subpackages contain driver implementations of signers for
// supported services.
//
// See https://gocloud.dev/howto/signatures/ for a detailed how-to guide.
//
//
// OpenCensus Integration
//
// OpenCensus supports tracing and metric collection for multiple languages and
// backend providers. See https://opencensus.io.
//
// This API collects OpenCensus traces and metrics for the following methods:
//  - Sign
//  - Verify
// All trace and metric names begin with the package import path.
// The traces add the method name.
// For example, "gocloud.dev/signers/Sign".
// The metrics are "completed_calls", a count of completed method calls by driver,
// method and status (error code); and "latency", a distribution of method latency
// by driver and method.
// For example, "gocloud.dev/signers/latency".
//
// To enable trace collection in your application, see "Configure Exporter" at
// https://opencensus.io/quickstart/go/tracing.
// To enable metric collection in your application, see "Exporting stats" at
// https://opencensus.io/quickstart/go/metrics.
package signers // import "gocloud.dev/signers"
import (
	"context"
	"net/url"
	"sync"

	"gocloud.dev/internal/gcerr"
	"gocloud.dev/internal/oc"
	"gocloud.dev/internal/openurl"
	"gocloud.dev/signers/driver"
)

// Signer does digest signing and verifying. To create a Signer, use
// constructors found in driver subpackages.
type Signer struct {
	k      driver.Signer
	tracer *oc.Tracer

	// mu protects the closed variable.
	// Read locks are kept to allow holding a read lock for long-running calls,
	// and thereby prevent closing until a call finishes.
	mu     sync.RWMutex
	closed bool
}

// NewSigner is intended for use by drivers only. Do not use in application code.
var NewSigner = newSigner

// newSigner creates a Signer.
func newSigner(k driver.Signer) *Signer {
	return &Signer{
		k: k,
		tracer: &oc.Tracer{
			Package:        pkgName,
			Provider:       oc.ProviderName(k),
			LatencyMeasure: latencyMeasure,
		},
	}
}

const pkgName = "gocloud.dev/signers"

var (
	latencyMeasure = oc.LatencyMeasure(pkgName)

	// OpenCensusViews are predefined views for OpenCensus metrics.
	// The views include counts and latency distributions for API method calls.
	// See the example at https://godoc.org/go.opencensus.io/stats/view for usage.
	OpenCensusViews = oc.Views(pkgName, latencyMeasure)
)

// Sign creates a signature from a digest using the configured key. The sign
// operation supports both asymmetric and symmetric keys.
func (k *Signer) Sign(ctx context.Context, digest []byte) (signature []byte, err error) {
	ctx = k.tracer.Start(ctx, "Sign")
	defer func() { k.tracer.End(ctx, err) }()

	k.mu.RLock()
	defer k.mu.RUnlock()
	if k.closed {
		return nil, errClosed
	}

	signature, err = k.k.Sign(ctx, digest)
	if err != nil {
		return nil, wrapError(k, err)
	}
	return signature, nil
}

// Verify verifies a signature using the configured key. The verify
// operation supports both symmetric keys and asymmetric keys. In case of
// asymmetric keys public portion of the key is used to verify the
// signature.
func (k *Signer) Verify(ctx context.Context, digest []byte, signature []byte) (ok bool, err error) {
	ctx = k.tracer.Start(ctx, "Verify")
	defer func() { k.tracer.End(ctx, err) }()

	k.mu.RLock()
	defer k.mu.RUnlock()
	if k.closed {
		return false, errClosed
	}

	ok, err = k.k.Verify(ctx, digest, signature)
	if err != nil {
		return false, wrapError(k, err)
	}
	return ok, nil
}

var errClosed = gcerr.Newf(gcerr.FailedPrecondition, nil, "signers: Signer has been closed")

// Close releases any resources used for the Signer.
func (k *Signer) Close() error {
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
func (k *Signer) ErrorAs(err error, i interface{}) bool {
	return gcerr.ErrorAs(err, i, k.k.ErrorAs)
}

func wrapError(k *Signer, err error) error {
	if err == nil {
		return nil
	}
	if gcerr.DoNotWrap(err) {
		return err
	}
	return gcerr.New(k.k.ErrorCode(err), err, 2, "signers")
}

// SignerURLOpener represents types that can open Signers based on a URL.
// The opener must not modify the URL argument. OpenSignerURL must be safe to
// call from multiple goroutines.
//
// This interface is generally implemented by types in driver packages.
type SignerURLOpener interface {
	OpenSignerURL(ctx context.Context, u *url.URL) (*Signer, error)
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

// SignerSchemes returns a sorted slice of the registered Signer schemes.
func (mux *URLMux) SignerSchemes() []string { return mux.schemes.Schemes() }

// ValidSignerScheme returns true iff scheme has been registered for Signers.
func (mux *URLMux) ValidSignerScheme(scheme string) bool { return mux.schemes.ValidScheme(scheme) }

// RegisterSigner registers the opener with the given scheme. If an opener
// already exists for the scheme, RegisterSigner panics.
func (mux *URLMux) RegisterSigner(scheme string, opener SignerURLOpener) {
	mux.schemes.Register("signers", "Signer", scheme, opener)
}

// OpenSigner calls OpenSignerURL with the URL parsed from urlstr.
// OpenSigner is safe to call from multiple goroutines.
func (mux *URLMux) OpenSigner(ctx context.Context, urlstr string) (*Signer, error) {
	opener, u, err := mux.schemes.FromString("Signer", urlstr)
	if err != nil {
		return nil, err
	}
	return opener.(SignerURLOpener).OpenSignerURL(ctx, u)
}

// OpenSignerURL dispatches the URL to the opener that is registered with the
// URL's scheme. OpenSignerURL is safe to call from multiple goroutines.
func (mux *URLMux) OpenSignerURL(ctx context.Context, u *url.URL) (*Signer, error) {
	opener, err := mux.schemes.FromURL("Signer", u)
	if err != nil {
		return nil, err
	}
	return opener.(SignerURLOpener).OpenSignerURL(ctx, u)
}

var defaultURLMux = new(URLMux)

// DefaultURLMux returns the URLMux used by OpenSigner.
//
// Driver packages can use this to register their SignerURLOpener on the mux.
func DefaultURLMux() *URLMux {
	return defaultURLMux
}

// OpenSigner opens the Signer identified by the URL given.
// See the URLOpener documentation in driver subpackages for
// details on supported URL formats, and https://gocloud.dev/concepts/urls
// for more information.
func OpenSigner(ctx context.Context, urlstr string) (*Signer, error) {
	return defaultURLMux.OpenSigner(ctx, urlstr)
}
