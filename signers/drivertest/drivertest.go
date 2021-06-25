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
// limitations under the License.

// Package drivertest provides a conformance test for implementations of
// the signers driver.
package drivertest // import "gocloud.dev/signers/drivertest"

import (
	"context"
	"testing"

	"gocloud.dev/signers"
	"gocloud.dev/signers/driver"
	"gocloud.dev/signers/internal"
)

// Harness describes the functionality test harnesses must provide to run
// conformance tests.
type Harness interface {
	// MakeDriver returns a pair of driver.Signer, each backed by a different
	// key, and an internal.Digester.
	MakeDriver(ctx context.Context) (driver.Signer, driver.Signer, internal.Digester, error)

	// Close is called when the test is complete.
	Close()
}

// HarnessMaker describes functions that construct a harness for running tests.
// It is called exactly once per test.
type HarnessMaker func(ctx context.Context, t *testing.T) (Harness, error)

// RunConformanceTests runs conformance tests for driver implementations of secret management.
func RunConformanceTests(t *testing.T, newHarness HarnessMaker) {
	t.Run("TestSignVerify", func(t *testing.T) {
		testSignVerify(t, newHarness)
	})
	t.Run("TestMultipleSigningsEqual", func(t *testing.T) {
		testMultipleSigningsEqual(t, newHarness)
	})
	t.Run("TestMultipleSigners", func(t *testing.T) {
		testMultipleSigners(t, newHarness)
	})
	t.Run("TestMalformedSignature", func(t *testing.T) {
		testMalformedSignature(t, newHarness)
	})
}

// testSignVerify tests the functionality of signing and verification
func testSignVerify(t *testing.T, newHarness HarnessMaker) {
	ctx := context.Background()
	harness, err := newHarness(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer harness.Close()

	drv, _, digester, err := harness.MakeDriver(ctx)
	if err != nil {
		t.Fatal(err)
	}
	signer := signers.NewSigner(drv)
	defer signer.Close()

	msg := []byte("I'm a secret message!")
	digest := digester.Digest(msg)

	signature, err := signer.Sign(ctx, digest)
	if err != nil {
		t.Fatal(err)
	}
	if signature == nil {
		t.Fatal(err)
	}

	ok, err := signer.Verify(ctx, digest, signature)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("signers do not verify")
	}
}

// testMultipleSigningsEqual tests that signing a digest multiple times with the
// same signer works, and that the signers both verify.
func testMultipleSigningsEqual(t *testing.T, newHarness HarnessMaker) {
	ctx := context.Background()
	harness, err := newHarness(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer harness.Close()

	d, _, digester, err := harness.MakeDriver(ctx)
	if err != nil {
		t.Fatal(err)
	}
	signer := signers.NewSigner(d)
	defer signer.Close()

	msg := []byte("I'm a secret message!")
	digest := digester.Digest(msg)

	sig1, err := signer.Sign(ctx, digest)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := signer.Verify(ctx, digest, sig1)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("verification of sig1 failed")
	}

	sig2, err := signer.Sign(ctx, digest)
	if err != nil {
		t.Fatal(err)
	}
	ok, err = signer.Verify(ctx, digest, sig2)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("verification of sig2 failed")
	}
}

// testMultipleSigners tests that signing the same text with different
// keys works, and that the signature bytes are different.
func testMultipleSigners(t *testing.T, newHarness HarnessMaker) {
	ctx := context.Background()
	harness, err := newHarness(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer harness.Close()

	drv1, drv2, digester, err := harness.MakeDriver(ctx)
	if err != nil {
		t.Fatal(err)
	}
	signer1 := signers.NewSigner(drv1)
	defer signer1.Close()
	signer2 := signers.NewSigner(drv2)
	defer signer2.Close()

	msg := []byte("I'm a secret message!")
	digest := digester.Digest(msg)

	sig, err := signer1.Sign(ctx, digest)
	if err != nil {
		t.Fatal(err)
	}

	ok, err := signer2.Verify(ctx, digest, sig)
	if err == nil && ok {
		t.Error("verification of sig by signer2 should have failed")
	}
}

// testMalformedSignature tests verification returns an error when the
// signature is malformed.
func testMalformedSignature(t *testing.T, newHarness HarnessMaker) {
	ctx := context.Background()
	harness, err := newHarness(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer harness.Close()

	drv, _, digester, err := harness.MakeDriver(ctx)
	if err != nil {
		t.Fatal(err)
	}
	signer := signers.NewSigner(drv)
	defer signer.Close()

	msg := []byte("I'm a secret message!")
	digest := digester.Digest(msg)

	sig, err := signer.Sign(ctx, digest)
	if err != nil {
		t.Fatal(err)
	}
	copySignature := func() []byte {
		return append([]byte{}, sig...)
	}

	l := len(sig)
	for _, tc := range []struct {
		name      string
		malformed []byte
	}{
		{
			name:      "wrong first byte",
			malformed: append([]byte{sig[0] + 1}, sig[1:]...),
		},
		{
			name:      "missing second byte",
			malformed: append(copySignature()[:1], sig[2:]...),
		},
		{
			name:      "wrong last byte",
			malformed: append(copySignature()[:l-2], sig[l-1]-1),
			// },
			// {
			// 	name:      "one more byte",
			// 	malformed: append(sig, 4),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if ok, err := signer.Verify(ctx, digest, tc.malformed); ok && err == nil {
				t.Error("Got ok and no error, want verify error")
			}
		})
	}
}
