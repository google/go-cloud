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
	_ "gocloud.dev/secrets/localsecrets"
)

func Example_openFromURL() {
	ctx := context.Background()

	// Create a Keeper using a URL.
	// This example uses "localsecrets", the in-memory implementation.
	// We need to add a blank import line to register the localsecrets driver's
	// URLOpener, which implements secrets.KeeperURLOpener:
	// import _ "gocloud.dev/secrets/localsecrets"
	// localsecrets registers for the "base64key" scheme.
	// All secrets.OpenKeeper URLs also work with "secrets+" or "secrets+keeper+" prefixes,
	// e.g., "secrets+base64key://..." or "secrets+variable+base64key://...".
	// All secrets URLs also work with the "secrets+" prefix, e.g., "secrets+base64key://".
	k, err := secrets.OpenKeeper(ctx, "base64key://smGbjm71Nxd1Ig5FS0wj9SlbzAIrnolCz9bQQ6uAhl4=")
	if err != nil {
		log.Fatal(err)
	}
	defer k.Close()

	// Now we can use k to encrypt/decrypt.
	plaintext := []byte("Go CDK Secrets")
	ciphertext, err := k.Encrypt(ctx, plaintext)
	if err != nil {
		log.Fatal(err)
	}
	decrypted, err := k.Decrypt(ctx, ciphertext)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(decrypted))

	// Output:
	// Go CDK Secrets
}
