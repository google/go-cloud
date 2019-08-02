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

package secrets_test

import (
	"context"
	"fmt"
	"log"

	"gocloud.dev/secrets"
	_ "gocloud.dev/secrets/gcpkms"
	"gocloud.dev/secrets/localsecrets"
	"google.golang.org/grpc/status"
)

func Example() {
	ctx := context.Background()

	// Construct a *secrets.Keeper from one of the secrets subpackages.
	// This example uses localsecrets.
	sk, err := localsecrets.NewRandomKey()
	if err != nil {
		log.Fatal(err)
	}
	keeper := localsecrets.NewKeeper(sk)
	defer keeper.Close()

	// Now we can use keeper to Encrypt.
	plaintext := []byte("Go CDK Secrets")
	ciphertext, err := keeper.Encrypt(ctx, plaintext)
	if err != nil {
		log.Fatal(err)
	}

	// And/or Decrypt.
	decrypted, err := keeper.Decrypt(ctx, ciphertext)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(decrypted))

	// Output:
	// Go CDK Secrets
}

func Example_errorAs() {
	// This example is specific to the gcpkms implementation; it
	// demonstrates access to the underlying google.golang.org/grpc/status.Status
	// type.
	// The types exposed for As by gcpkms are documented in
	// https://godoc.org/gocloud.dev/secrets/gcpkms#hdr-As
	ctx := context.Background()

	const url = "gcpkms://projects/proj/locations/global/keyRings/test/ring/wrongkey"
	keeper, err := secrets.OpenKeeper(ctx, url)
	if err != nil {
		log.Fatal(err)
	}
	defer keeper.Close()

	plaintext := []byte("Go CDK secrets")
	_, err = keeper.Encrypt(ctx, plaintext)
	if err != nil {
		var s *status.Status
		if keeper.ErrorAs(err, &s) {
			fmt.Println(s.Code())
		}
	}
}

func ExampleKeeper_Encrypt() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()
	var keeper *secrets.Keeper

	plainText := []byte("Secrets secrets...")
	cipherText, err := keeper.Encrypt(ctx, plainText)
	if err != nil {
		log.Fatal(err)
	}

	// PRAGMA: On gocloud.dev, hide the rest of the function.
	_ = cipherText
}

func ExampleKeeper_Decrypt() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()
	var keeper *secrets.Keeper

	var cipherText []byte // obtained from elsewhere and random-looking
	plainText, err := keeper.Decrypt(ctx, cipherText)
	if err != nil {
		log.Fatal(err)
	}

	// PRAGMA: On gocloud.dev, hide the rest of the function.
	_ = plainText
}
