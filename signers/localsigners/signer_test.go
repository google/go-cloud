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

package localsigners

import (
	"context"
	"crypto"
	"log"
	"testing"

	"gocloud.dev/signers"
	"gocloud.dev/signers/driver"
	"gocloud.dev/signers/drivertest"
	"gocloud.dev/signers/internal"
)

type harness struct{}

func (h *harness) MakeDriver(_ context.Context) (driver.Signer, driver.Signer, internal.Digester, error) {
	pepper1, err := NewRandomPepper()
	if err != nil {
		log.Fatal(err)
	}
	pepper2, err := NewRandomPepper()
	if err != nil {
		log.Fatal(err)
	}
	return &signer{hash: crypto.MD5, pepper: pepper1},
		&signer{hash: crypto.MD5, pepper: pepper2},
		&internal.SHA256{},
		nil
}

func (h *harness) Close() {}

func newHarness(_ context.Context, _ *testing.T) (drivertest.Harness, error) {
	return &harness{}, nil
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness)
}

func TestOpenSigner(t *testing.T) {
	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"base64signer://", false},
		// OK.
		{"base64signer://md5/smGbjm71Nxd1Ig5FS0wj9SlbzAIrnolCz9bQQ6uAhl4=", false},
		// Valid base64, but < 32 bytes.
		{"base64signer://md5/c2VjcmV0", true},
		// Valid base64, but > 32 bytes.
		{"base64signer://md5/c2VjcmV0c2VjcmV0c2VjcmV0c2VjcmV0c2VjcmV0c3NlY3JldHNlY3JldHNlY3JldHNlY3JldHNlY3JldHM=", false},
		// Invalid base64 key.
		{"base64signer://md5/not-valid-base64", true},
		// Valid base64 key (but invalid if using Std encoding instead of URL encoding).
		{"base64signer://md5/UKcmEoZW7nKl0uPHr8yV__KJm0ANhiFz8PzDN-gYWq8=", true},
		// Invalid parameter.
		{"base64signer:///?param=value", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		signer, err := signers.OpenSigner(ctx, test.URL)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
		if err == nil {
			if err = signer.Close(); err != nil {
				t.Errorf("%s: got error during close: %v", test.URL, err)
			}
		}
	}
}
