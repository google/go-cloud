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

package awskms_test

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go/aws/session"
	"gocloud.dev/secrets"
	"gocloud.dev/secrets/awskms"
)

func ExampleOpenKeeper() {
	// Establish an AWS session.
	// See https://docs.aws.amazon.com/sdk-for-go/api/aws/session/ for more info.
	sess, err := session.NewSession(nil)
	if err != nil {
		log.Fatal(err)
	}

	// Get a client to use with the KMS API.
	client, err := awskms.Dial(sess)
	if err != nil {
		log.Fatal(err)
	}

	// Construct a *secrets.Keeper.
	keeper := awskms.OpenKeeper(
		client,
		// Get the key ID. Here is an example of using an alias. See
		// https://docs.aws.amazon.com/kms/latest/developerguide/viewing-keys.html#find-cmk-id-arn
		// for more details.
		"alias/test-secrets",
		nil,
	)
	defer keeper.Close()

	// Now we can use keeper to encrypt or decrypt.
	ctx := context.Background()
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
	// The host + path are the key ID; this example uses an alias. See
	// https://docs.aws.amazon.com/kms/latest/developerguide/viewing-keys.html#find-cmk-id-arn
	// for more details.
	keeper, err := secrets.OpenKeeper(ctx, "awskms://alias/my-key")
	if err != nil {
		log.Fatal(err)
	}
	defer keeper.Close()
}
