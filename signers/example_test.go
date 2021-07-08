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
	"crypto"
	"crypto/sha512"
	"fmt"
	"log"

	"google.golang.org/grpc/status"

	"gocloud.dev/signers"
	_ "gocloud.dev/signers/gcpsig"
	"gocloud.dev/signers/localsigners"
)

func Example() {
	ctx := context.Background()

	// Construct a *signers.Signer from one of the signers subpackages.
	// This example uses localsigners.
	sk, err := localsigners.NewRandomPepper()
	if err != nil {
		log.Fatal(err)
	}
	signer, err := localsigners.NewSigner(crypto.MD5, sk)
	if err != nil {
		log.Fatal(err)
	}
	defer signer.Close()

	// Now we can use signer to Sign.
	plaintext := []byte("Go CDK signers")
	signature, err := signer.Sign(ctx, plaintext)
	if err != nil {
		log.Fatal(err)
	}

	// And/or Verify.
	ok, err := signer.Verify(ctx, plaintext, signature)
	if err != nil {
		log.Fatal(err)
	}
	if !ok {
		log.Fatal("Signature verification failed")
	}
}

func Example_errorAs() {
	// This example is specific to the gcpsig implementation; it
	// demonstrates access to the underlying google.golang.org/grpc/status.Status
	// type.
	// The types exposed for As by gcpsig are documented in
	// https://godoc.org/gocloud.dev/signatures/gcpsig#hdr-As
	ctx := context.Background()

	const url = "gcpsig://projects/proj/locations/global/keyRings/test/ring/wrongkey"
	signer, err := signers.OpenSigner(ctx, url)
	if err != nil {
		log.Fatal(err)
	}
	defer signer.Close()

	plaintext := []byte("Go CDK signers")
	_, err = signer.Sign(ctx, plaintext)
	if err != nil {
		var s *status.Status
		if signer.ErrorAs(err, &s) {
			fmt.Println(s.Code())
		}
	}
}

func ExampleSigner_Sign() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()
	var signer *signers.Signer

	plainText := []byte("How now brow cow")
	digest := sha512.Sum512(plainText)
	cipherText, err := signer.Sign(ctx, digest[:])
	if err != nil {
		log.Fatal(err)
	}

	// PRAGMA: On gocloud.dev, hide the rest of the function.
	_ = cipherText
}

func ExampleSigner_Verify() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()
	var signer *signers.Signer

	plainText := []byte("How now brow cow")
	digest := sha512.Sum512(plainText)
	var signature []byte // obtained from elsewhere and random-looking
	ok, err := signer.Verify(ctx, digest[:], signature)
	if err != nil {
		log.Fatal(err)
	}
	if !ok {
		log.Fatal("Signature verification failed")
	}

	// PRAGMA: On gocloud.dev, hide the rest of the function.
}
