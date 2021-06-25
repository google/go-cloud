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

package awssig_test

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go/aws/session"

	"gocloud.dev/signers"
	"gocloud.dev/signers/awssig"
)

func ExampleOpenSigner() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.

	// Establish an AWS session.
	// See https://docs.aws.amazon.com/sdk-for-go/api/aws/session/ for more info.
	sess, err := session.NewSession(nil)
	if err != nil {
		log.Fatal(err)
	}

	// Get a client to use with the KMS API.
	client, err := awssig.Dial(sess)
	if err != nil {
		log.Fatal(err)
	}

	// Construct a *signers.Signer.
	signer := awssig.OpenSigner(client, "RSASSA_PKCS1_V1_5_SHA_512", "alias/test-secrets", nil)
	defer signer.Close()
}

func Example_openFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/signers/awssig"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// Use one of the following:

	// 1. By ID.
	signerByID, err := signers.OpenSigner(ctx,
		"awssig://RSASSA_PKCS1_V1_5_SHA_512/1234abcd-12ab-34cd-56ef-1234567890ab?region=us-east-1")
	if err != nil {
		log.Fatal(err)
	}
	defer signerByID.Close()

	// 2. By alias.
	signerByAlias, err := signers.OpenSigner(ctx,
		"awssig://RSASSA_PSS_SHA_256/alias/ExampleAlias?region=us-east-1")
	if err != nil {
		log.Fatal(err)
	}
	defer signerByAlias.Close()

	// 3. By ARN.
	const arn = "arn:aws:kms:us-east-1:111122223333:key/" +
		"1234abcd-12ab-34bc-56ef-1234567890ab"
	signerByARN, err := signers.OpenSigner(ctx,
		"awssig://RSASSA_PSS_SHA_384/"+arn+"?region=us-east-1")
	if err != nil {
		log.Fatal(err)
	}
	defer signerByARN.Close()
}
