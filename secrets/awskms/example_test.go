// Copyright 2019 The Go Cloud Authors
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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"gocloud.dev/secrets/awskms"
)

func Example_encrypt() {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-west-1"),
	})
	if err != nil {
		log.Fatal(err)
	}

	// Get a client to use with the KMS API.
	client, err := awskms.Dial(sess)
	if err != nil {
		log.Fatal(err)
	}

	plaintext := []byte("Hello, Secrets!")

	keeper := awskms.NewKeeper(
		client,
		// Get the key resource ID. Here is an example of using an alias. See
		// https://docs.aws.amazon.com/kms/latest/developerguide/viewing-keys.html#find-cmk-id-arn
		// for more details.
		"alias/test-secrets",
		nil,
	)

	// Makes the request to the KMS API to encrypt the plain text into a binary.
	encrypted, err := keeper.Encrypt(context.Background(), plaintext)
	if err != nil {
		log.Fatal(err)
	}
	// Store the encrypted secret.
	_ = encrypted
}

func Example_decrypt() {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-west-1"),
	})
	if err != nil {
		panic(err)
	}

	// Get a client to use with the KMS API.
	client, err := awskms.Dial(sess)
	if err != nil {
		panic(err)
	}

	// Get the secret to be decrypted from some kind of storage.
	var ciphertext []byte

	// keyID is not needed when doing decryption.
	keeper := awskms.NewKeeper(client, "", nil)

	// Makes the request to the KMS API to decrypt the binary into plain text.
	decrypted, err := keeper.Decrypt(context.Background(), ciphertext)
	if err != nil {
		panic(err)
	}
	// Use the decrypted secret.
	_ = decrypted
}
