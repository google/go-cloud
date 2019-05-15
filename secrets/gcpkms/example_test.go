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

package gcpkms_test

import (
	"context"
	"log"

	"gocloud.dev/secrets"
	"gocloud.dev/secrets/gcpkms"
)

func ExampleOpenKeeper() {
	// Get a client to use with the KMS API.
	ctx := context.Background()
	client, done, err := gcpkms.Dial(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
	// Close the connection when done.
	defer done()

	// Construct a *secrets.Keeper.
	keeper := gcpkms.OpenKeeper(
		client,
		// Get the key resource ID.
		// See https://cloud.google.com/kms/docs/object-hierarchy#key for more
		// information.
		// You can also use gcpkms.KeyResourceID to construct the string.
		"projects/MYPROJECT/locations/MYLOCATION/keyRings/MYKEYRING/cryptoKeys/MYKEY",
		nil,
	)
	defer keeper.Close()

	// Now we can use keeper to encrypt or decrypt.
	plaintext := []byte("Hello, Secrets!")
	ciphertext, err := keeper.Encrypt(ctx, plaintext)
	if err != nil {
		log.Fatal(err)
	}
	decrypted, err := keeper.Decrypt(ctx, ciphertext)
	if err != nil {
		log.Fatal(err)
	}
	_ = decrypted
}

func Example_openFromURL() {
	ctx := context.Background()

	// secrets.OpenKeeper creates a *secrets.Keeper from a URL.
	// The host + path are the key resourceID; see
	// https://cloud.google.com/kms/docs/object-hierarchy#key
	// for more information.
	keeper, err := secrets.OpenKeeper(ctx, "gcpkms://projects/MYPROJECT/locations/MYLOCATION/keyRings/MYKEYRING/cryptoKeys/MYKEY")
	if err != nil {
		log.Fatal(err)
	}
	defer keeper.Close()
}
