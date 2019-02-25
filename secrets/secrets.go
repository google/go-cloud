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
// messages.
//
// Subpackages contain distinct implementations of secrets for various
// providers, including Cloud and on-prem solutions. For example, "localsecrets"
// supports encryption/decryption using a locally provided key. Your application
// should import one of these provider-specific subpackages and use its exported
// function(s) to create a *Keeper; do not use the NewKeeper function in this
// package. For example:
//
//  keeper := localsecrets.NewKeeper(myKey)
//  encrypted, err := keeper.Encrypt(ctx.Background(), []byte("text"))
//  ...
//
// Then, write your application code using the *Keeper type. You can easily
// reconfigure your initialization code to choose a different provider.
// You can develop your application locally using localsecrets, or deploy it to
// multiple Cloud providers. You may find http://github.com/google/wire useful
// for managing your initialization code.
//
//
// OpenCensus Integration
//
// OpenCensus supports tracing and metric collection for multiple languages and
// backend providers. See https://opencensus.io.
//
// This API collects OpenCensus traces and metrics for the following methods:
//  - Encrypt
//  - Decrypt
// All trace and metric names begin with the package import path.
// The traces add the method name.
// For example, "gocloud.dev/secrets/Encrypt".
// The metrics are "completed_calls", a count of completed method calls by provider,
// method and status (error code); and "latency", a distribution of method latency
// by provider and method.
// For example, "gocloud.dev/secrets/latency".
//
// To enable trace collection in your application, see "Configure Exporter" at
// https://opencensus.io/quickstart/go/tracing.
// To enable metric collection in your application, see "Exporting stats" at
// https://opencensus.io/quickstart/go/metrics.
package secrets // import "gocloud.dev/secrets"

import (
	"context"

	"gocloud.dev/internal/gcerr"
	"gocloud.dev/internal/oc"
	"gocloud.dev/secrets/driver"
)

// Keeper does encryption and decryption. To create a Keeper, use constructors
// found in provider-specific subpackages.
type Keeper struct {
	k      driver.Keeper
	tracer *oc.Tracer
}

// NewKeeper is intended for use by provider implementations.
var NewKeeper = newKeeper

// newKeeper creates a Keeper.
func newKeeper(k driver.Keeper) *Keeper {
	return &Keeper{
		k: k,
		tracer: &oc.Tracer{
			Package:        pkgName,
			Provider:       oc.ProviderName(k),
			LatencyMeasure: latencyMeasure,
		},
	}
}

const pkgName = "gocloud.dev/secrets"

var (
	latencyMeasure = oc.LatencyMeasure(pkgName)

	// OpenCensusViews are predefined views for OpenCensus metrics.
	// The views include counts and latency distributions for API method calls.
	// See the example at https://godoc.org/go.opencensus.io/stats/view for usage.
	OpenCensusViews = oc.Views(pkgName, latencyMeasure)
)

// Encrypt encrypts the plaintext and returns the cipher message.
func (k *Keeper) Encrypt(ctx context.Context, plaintext []byte) (ciphertext []byte, err error) {
	ctx = k.tracer.Start(ctx, "Encrypt")
	defer func() { k.tracer.End(ctx, err) }()

	b, err := k.k.Encrypt(ctx, plaintext)
	if err != nil {
		return nil, wrapError(k, err)
	}
	return b, nil
}

// Decrypt decrypts the ciphertext and returns the plaintext.
func (k *Keeper) Decrypt(ctx context.Context, ciphertext []byte) (plaintext []byte, err error) {
	ctx = k.tracer.Start(ctx, "Decrypt")
	defer func() { k.tracer.End(ctx, err) }()

	b, err := k.k.Decrypt(ctx, ciphertext)
	if err != nil {
		return nil, wrapError(k, err)
	}
	return b, nil
}

// ErrorAs converts i to provider-specific error types when you want to directly
// handle the raw error types returned by the provider. This means that you
// will write some provider-specific code to handle the error, so use with care.
//
// See the documentation for the subpackage used to instantiate Keeper to see
// which error type(s) are supported.
//
// ErrorAs panics if i is nil or not a pointer.
// ErrorAs returns false if err == nil.
func (k *Keeper) ErrorAs(err error, i interface{}) bool {
	return gcerr.ErrorAs(err, i, k.k.ErrorAs)
}

func wrapError(k *Keeper, err error) error {
	if gcerr.DoNotWrap(err) {
		return err
	}
	return gcerr.New(k.k.ErrorCode(err), err, 2, "secrets")
}
