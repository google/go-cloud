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

	"gocloud.dev/secrets/localsecrets"
)

func Example() {
	ctx := context.Background()

	// Construct a *secrets.Keeper from one of the secrets subpackages.
	// This example uses localsecrets.
	keeper := localsecrets.NewKeeper(localsecrets.ByteKey("secret key"))

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
