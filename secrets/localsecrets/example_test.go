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

package localsecrets_test

import (
	"context"
	"fmt"
	"log"

	"gocloud.dev/secrets"
	"gocloud.dev/secrets/localsecrets"
)

func ExampleNewKeeper() {
	// localsecrets.Keeper utilizes the golang.org/x/crypto/nacl/secretbox package
	// for the crypto implementation, and secretbox requires a secret key
	// that is a [32]byte. localsecrets
	secretKey, err := localsecrets.NewRandomKey()
	if err != nil {
		log.Fatal(err)
	}
	keeper := localsecrets.NewKeeper(secretKey)
	defer keeper.Close()

	// Now we can use keeper to encrypt or decrypt.
	plaintext := []byte("Hello, Secrets!")
	ctx := context.Background()
	ciphertext, err := keeper.Encrypt(ctx, plaintext)
	if err != nil {
		log.Fatal(err)
	}
	decrypted, err := keeper.Decrypt(ctx, ciphertext)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(decrypted))

	// Output:
	// Hello, Secrets!
}

func Example_openFromURL() {
	ctx := context.Background()

	// secrets.OpenKeeper creates a *secrets.Keeper from a URL.
	// Using "base64key://", a new random key will be generated.
	keeper, err := secrets.OpenKeeper(ctx, "base64key://")
	if err != nil {
		log.Fatal(err)
	}
	defer keeper.Close()

	// The URL hostname must be a base64-encoded key, of length 32 bytes when decoded.
	keeper, err = secrets.OpenKeeper(ctx, "base64key://smGbjm71Nxd1Ig5FS0wj9SlbzAIrnolCz9bQQ6uAhl4=")
	if err != nil {
		log.Fatal(err)
	}
	defer keeper.Close()
}
