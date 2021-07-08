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

package localsigners_test

import (
	"context"
	"crypto"
	"log"

	"gocloud.dev/signers"
	"gocloud.dev/signers/localsigners"
)

func ExampleNewSigner() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.

	pepper, err := localsigners.NewRandomPepper()
	if err != nil {
		log.Fatal(err)
	}
	signer, err := localsigners.NewSigner(crypto.MD4, pepper)
	if err != nil {
		log.Fatal(err)
	}
	defer signer.Close()
}

func Example_openFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.

	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/signers/localsigners"

	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// Using "base64signer://", a new random key will be generated.
	randomKeySigner, err := signers.OpenSigner(ctx, "base64signer://")
	if err != nil {
		log.Fatal(err)
	}
	defer randomKeySigner.Close()

	// Otherwise, the URL hostname must be a base64-encoded pepper.
	savedKeySigner, err := signers.OpenSigner(ctx, "base64signer://MD5/smGbjm71Nxd1Ig5FS0wj9SlbzAIrnolCz9bQQ6uAhl4=")
	if err != nil {
		log.Fatal(err)
	}
	defer savedKeySigner.Close()
}
