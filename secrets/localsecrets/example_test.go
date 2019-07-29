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
	"log"

	"gocloud.dev/secrets"
	"gocloud.dev/secrets/localsecrets"
)

func ExampleNewKeeper() {
	// PRAGMA(gocloud.dev): Package this example for gocloud.dev.

	secretKey, err := localsecrets.NewRandomKey()
	if err != nil {
		log.Fatal(err)
	}
	keeper := localsecrets.NewKeeper(secretKey)
	defer keeper.Close()
}

func Example_openFromURL() {
	// PRAGMA(gocloud.dev): Package this example for gocloud.dev.

	// PRAGMA(gocloud.dev): Add a blank import: _ "gocloud.dev/secrets/localsecrets"

	// PRAGMA(gocloud.dev): Skip until next blank line.
	ctx := context.Background()

	// Using "base64key://", a new random key will be generated.
	randomKeyKeeper, err := secrets.OpenKeeper(ctx, "base64key://")
	if err != nil {
		log.Fatal(err)
	}
	defer randomKeyKeeper.Close()

	// Otherwise, the URL hostname must be a base64-encoded key, of length 32 bytes when decoded.
	savedKeyKeeper, err := secrets.OpenKeeper(ctx, "base64key://smGbjm71Nxd1Ig5FS0wj9SlbzAIrnolCz9bQQ6uAhl4=")
	if err != nil {
		log.Fatal(err)
	}
	defer savedKeyKeeper.Close()
}
