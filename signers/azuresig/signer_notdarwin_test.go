// +build !darwin

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

package azuresig_test

import (
	"context"
	"testing"

	"gocloud.dev/signers"
)

// This test no longer works on MacOS, as OpenKeeper always fails with "MSI not available".

func TestOpenKeeper(t *testing.T) {
	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"azuresig://mykeyvault.vault.azure.net/keys/mykey/myversion", false},
		// No version -> OK.
		{"azuresig://mykeyvault.vault.azure.net/keys/mykey", false},
		// Setting algorithm query param -> OK.
		{"azuresig://mykeyvault.vault.azure.net/keys/mykey/myversion?algorithm=RSA-OAEP", false},
		// Invalid query parameter.
		{"azuresig://mykeyvault.vault.azure.net/keys/mykey/myversion?param=value", true},
		// Missing key vault name.
		{"azuresig:///vault.azure.net/keys/mykey/myversion", true},
		// Missing "keys".
		{"azuresig://mykeyvault.vault.azure.net/mykey/myversion", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		keeper, err := signers.OpenSigner(ctx, test.URL)
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
