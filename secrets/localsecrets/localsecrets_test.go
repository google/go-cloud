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

package localsecrets

import (
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	"gocloud.dev/secrets"
	"gocloud.dev/secrets/driver"
	"gocloud.dev/secrets/drivertest"
)

type harness struct{}

func (h *harness) MakeDriver(ctx context.Context) (driver.Keeper, driver.Keeper, error) {
	secret1, err := NewRandomKey()
	if err != nil {
		log.Fatal(err)
	}
	secret2, err := NewRandomKey()
	if err != nil {
		log.Fatal(err)
	}
	return &keeper{secretKey: secret1}, &keeper{secretKey: secret2}, nil
}

func (h *harness) Close() {}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	t.Helper()

	return &harness{}, nil
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyAs{}})
}

type verifyAs struct{}

func (v verifyAs) Name() string {
	return "verify As function"
}

func (v verifyAs) ErrorCheck(k *secrets.Keeper, err error) error {
	var s string
	if k.ErrorAs(err, &s) {
		return errors.New("Keeper.ErrorAs expected to fail")
	}
	return nil
}

func TestSmallData(t *testing.T) {
	key, err := NewRandomKey()
	if err != nil {
		t.Fatal(err)
	}
	keeper := NewKeeper(key)
	defer keeper.Close()

	ctx := context.Background()
	const plaintext = "hello world"
	ciphertext, err := keeper.Encrypt(ctx, []byte(plaintext))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		Ciphertext           []byte
		Want                 string
		WantErr              bool
		WantInvalidLengthErr bool // true if we want the error to be that the ciphertext length is invalid
	}{
		{nil, "", true, true},
		{[]byte{}, "", true, true},
		{[]byte{0}, "", true, true},
		{ciphertext[:1], "", true, true},
		{ciphertext[:nonceSize-1], "", true, true},
		{ciphertext[:nonceSize], "", true, false}, // not invalid, but Decrypt will fail
		{ciphertext, plaintext, false, false},     // works
	}

	for _, test := range tests {
		got, err := keeper.Decrypt(ctx, test.Ciphertext)
		if (err != nil) != test.WantErr {
			t.Errorf("got err %v from Decrypt, want error? %v", err, test.WantErr)
		}
		if err == nil {
			if gotStr := string(got); gotStr != test.Want {
				t.Errorf("got %s want %s", gotStr, test.Want)
			}
		} else {
			if gotInvalid := strings.Contains(err.Error(), "invalid message length"); gotInvalid != test.WantInvalidLengthErr {
				t.Errorf("got invalid message length error? %v want %v", gotInvalid, test.WantInvalidLengthErr)
			}
		}
		// Encrypt should always work.
		if _, err := keeper.Encrypt(ctx, test.Ciphertext); err != nil {
			t.Errorf("got error %v from Encrypt, want nil", err)
		}
	}
}

func TestOpenKeeper(t *testing.T) {
	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"base64key://", false},
		// OK.
		{"base64key://smGbjm71Nxd1Ig5FS0wj9SlbzAIrnolCz9bQQ6uAhl4=", false},
		// Valid base64, but < 32 bytes.
		{"base64key://c2VjcmV0", true},
		// Valid base64, but > 32 bytes.
		{"base64key://c2VjcmV0c2VjcmV0c2VjcmV0c2VjcmV0c2VjcmV0c3NlY3JldHNlY3JldHNlY3JldHNlY3JldHNlY3JldHM=", true},
		// Invalid base64 key.
		{"base64key://not-valid-base64", true},
		// Valid base64 key (but invalid if using Std encoding instead of URL encoding).
		{"base64Key://UKcmEoZW7nKl0uPHr8yV__KJm0ANhiFz8PzDN-gYWq8=", false},
		// Invalid parameter.
		{"base64key://?param=value", true},
		// OK with encryption context.
		{"base64key://smGbjm71Nxd1Ig5FS0wj9SlbzAIrnolCz9bQQ6uAhl4=?context_abc=foo&context_def=bar", false},
		// OK with encryption context and empty host (random key).
		{"base64key://?context_tenant=123", false},
		// Encryption context with multiple values for same key is an error.
		{"base64key://smGbjm71Nxd1Ig5FS0wj9SlbzAIrnolCz9bQQ6uAhl4=?context_abc=foo&context_abc=bar", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		keeper, err := secrets.OpenKeeper(ctx, test.URL)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
		if err == nil {
			if err = keeper.Close(); err != nil {
				t.Errorf("%s: got error during close: %v", test.URL, err)
			}
		}
	}
}

func TestEncryptionContext(t *testing.T) {
	key, err := NewRandomKey()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const plaintext = "hello world"

	// Encrypt with encryption context.
	keeperWithCtx := OpenKeeper(key, &KeeperOptions{
		EncryptionContext: map[string]string{"tenant": "abc", "env": "prod"},
	})
	defer keeperWithCtx.Close()

	ciphertext, err := keeperWithCtx.Encrypt(ctx, []byte(plaintext))
	if err != nil {
		t.Fatal(err)
	}

	// Decrypt with the same encryption context should succeed.
	got, err := keeperWithCtx.Decrypt(ctx, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != plaintext {
		t.Errorf("got %q, want %q", string(got), plaintext)
	}

	// Decrypt without encryption context should fail.
	keeperNoCtx := NewKeeper(key)
	defer keeperNoCtx.Close()

	_, err = keeperNoCtx.Decrypt(ctx, ciphertext)
	if err == nil {
		t.Error("expected Decrypt to fail without encryption context, but it succeeded")
	}

	// Decrypt with a different encryption context should fail.
	keeperDiffCtx := OpenKeeper(key, &KeeperOptions{
		EncryptionContext: map[string]string{"tenant": "xyz"},
	})
	defer keeperDiffCtx.Close()

	_, err = keeperDiffCtx.Decrypt(ctx, ciphertext)
	if err == nil {
		t.Error("expected Decrypt to fail with different encryption context, but it succeeded")
	}

	// Encrypt without context, decrypt with context should fail.
	ciphertext2, err := keeperNoCtx.Encrypt(ctx, []byte(plaintext))
	if err != nil {
		t.Fatal(err)
	}
	_, err = keeperWithCtx.Decrypt(ctx, ciphertext2)
	if err == nil {
		t.Error("expected Decrypt to fail when ciphertext was encrypted without context, but it succeeded")
	}
}

func TestEncryptionContextSalt(t *testing.T) {
	key, err := NewRandomKey()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const plaintext = "hello world"
	ec := map[string]string{"tenant": "abc"}

	// Encrypt with custom salt.
	keeperSalt1 := OpenKeeper(key, &KeeperOptions{
		EncryptionContext: ec,
		Salt:              []byte("salt1"),
	})
	defer keeperSalt1.Close()

	ciphertext, err := keeperSalt1.Encrypt(ctx, []byte(plaintext))
	if err != nil {
		t.Fatal(err)
	}

	// Decrypt with the same salt should succeed.
	got, err := keeperSalt1.Decrypt(ctx, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != plaintext {
		t.Errorf("got %q, want %q", string(got), plaintext)
	}

	// Decrypt with a different salt should fail.
	keeperSalt2 := OpenKeeper(key, &KeeperOptions{
		EncryptionContext: ec,
		Salt:              []byte("salt2"),
	})
	defer keeperSalt2.Close()

	_, err = keeperSalt2.Decrypt(ctx, ciphertext)
	if err == nil {
		t.Error("expected Decrypt to fail with different salt, but it succeeded")
	}

	// Decrypt with default salt (nil) should also fail.
	keeperDefaultSalt := OpenKeeper(key, &KeeperOptions{
		EncryptionContext: ec,
	})
	defer keeperDefaultSalt.Close()

	_, err = keeperDefaultSalt.Decrypt(ctx, ciphertext)
	if err == nil {
		t.Error("expected Decrypt to fail with default salt vs custom salt, but it succeeded")
	}
}

func TestOpenKeeperURLWithSalt(t *testing.T) {
	ctx := context.Background()
	const plaintext = "hello world"

	// Open two keepers with same key + context but different salts via URL.
	k1, err := secrets.OpenKeeper(ctx, "base64key://smGbjm71Nxd1Ig5FS0wj9SlbzAIrnolCz9bQQ6uAhl4=?context_a=1&salt=salt1")
	if err != nil {
		t.Fatal(err)
	}
	defer k1.Close()

	ciphertext, err := k1.Encrypt(ctx, []byte(plaintext))
	if err != nil {
		t.Fatal(err)
	}

	// Same key + context + salt via URL should decrypt.
	k1b, err := secrets.OpenKeeper(ctx, "base64key://smGbjm71Nxd1Ig5FS0wj9SlbzAIrnolCz9bQQ6uAhl4=?context_a=1&salt=salt1")
	if err != nil {
		t.Fatal(err)
	}
	defer k1b.Close()

	got, err := k1b.Decrypt(ctx, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != plaintext {
		t.Errorf("got %q, want %q", string(got), plaintext)
	}

	// Different salt via URL should fail to decrypt.
	k2, err := secrets.OpenKeeper(ctx, "base64key://smGbjm71Nxd1Ig5FS0wj9SlbzAIrnolCz9bQQ6uAhl4=?context_a=1&salt=salt2")
	if err != nil {
		t.Fatal(err)
	}
	defer k2.Close()

	_, err = k2.Decrypt(ctx, ciphertext)
	if err == nil {
		t.Error("expected Decrypt to fail with different salt, but it succeeded")
	}
}
