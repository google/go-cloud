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

package signers_test

import (
	"context"
	"log"

	"gocloud.dev/signers"
	_ "gocloud.dev/signers/localsigners"
)

func Example_openFromURL() {
	ctx := context.Background()

	// Create a Signer using a URL.
	// This example uses "localsigners", the in-memory implementation.
	// We need to add a blank import line to register the localsigners driver's
	// URLOpener, which implements secrets.SignerURLOpener:
	// import _ "gocloud.dev/signers/localsigners"
	// localsigners registers for the "base64signer" scheme.
	// All signers.OpenSigner URLs also work with "signers+" or "signers+signer+" prefixes,
	// e.g., "signers+base64signer://..." or "signers+variable+base64signer:///...".
	// All signers URLs also work with the "signers+" prefix, e.g., "signers+base64signer:///".
	k, err := signers.OpenSigner(ctx, "base64signer:///smGbjm71Nxd1Ig5FS0wj9SlbzAIrnolCz9bQQ6uAhl4=")
	if err != nil {
		log.Fatal(err)
	}
	defer k.Close()

	// Now we can use k to sign/verify.
	plaintext := []byte("Go CDK signers")
	signature, err := k.Sign(ctx, plaintext)
	if err != nil {
		log.Fatal(err)
	}
	ok, err := k.Verify(ctx, signature, signature)
	if err != nil {
		log.Fatal(err)
	}
	if !ok {
		log.Fatal(err)
	}
}
