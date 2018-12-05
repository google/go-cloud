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

//
package drivertest

import (
	"context"
	"github.com/google/go-cloud/internal/secrets/driver"
	"github.com/google/go-cmp/cmp"
	"testing"
)

// Harness descibes the functionality test harnesses must provide to run
// conformance tests.
type Harness interface {
	// MakeDriver
	MakeDriver(ctx context.Context) (driver.Encrypter, driver.Decrypter, error)
}

// HarnessMaker describes functions that construct a harness for running tests.
// It is called exactly once per test; Harness.Close() will be called when the test is complete.
type HarnessMaker func(ctx context.Context, t *testing.T) (Harness, error)

// RunConformanceTests runs conformance tests for provider implementations of secret management.
func RunConformanceTests(t *testing.T, newHarness HarnessMaker) {
	t.Run("TestEncryptDecrypt", func(t *testing.T) {
		testEncryptDecrypt(t, newHarness)
	})
}

// testWrite tests the functionality of NewWriter and Writer.
func testEncryptDecrypt(t *testing.T, newHarness HarnessMaker) {
	ctx := context.Background()
	harness, err := newHarness(ctx, t)
	if err != nil {
		t.Fatal(err)
	}

	encrypter, decrypter, err := harness.MakeDriver(ctx)
	if err != nil {
		t.Fatal(err)
	}

	msg := []byte("I'm a secret message!")
	encryptedMsg, err := encrypter.Encrypt(ctx, msg)
	if err != nil {
		t.Fatal(err)
	}
	if cmp.Equal(msg, encryptedMsg) {
		t.Error("Encrypted message should not match plain text.")
	}
	decryptedMsg, err := decrypter.Decrypt(ctx, encryptedMsg)
	if err != nil {
		t.Fatal(err)
	}
	if !cmp.Equal(msg, decryptedMsg) {
		t.Error("Decrypted message should match original message.")
	}

}
