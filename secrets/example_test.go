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

	"gocloud.dev/secrets/gcpkms"
	"google.golang.org/grpc/status"
)

func ExampleKeeper_ErrorAs() {

	// Get a client to use with the KMS API.
	ctx := context.Background()
	client, done, err := gcpkms.Dial(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
	// Close the connection when done.
	defer done()

	// Construct a *secrets.Keeper.
	keeper := gcpkms.NewKeeper(
		client,
		// Get the key resource ID.
		// See https://cloud.google.com/kms/docs/object-hierarchy#key for more
		// information.
		&gcpkms.KeyID{
			ProjectID: "wrong-project-id",
			Location:  "global",
			KeyRing:   "test",
			Key:       "key-name",
		},
		nil,
	)

	// Now we can use keeper to encrypt or decrypt.
	plaintext := []byte("Hello, Secrets!")
	if _, err := keeper.Encrypt(ctx, plaintext); err != nil {
		var s *status.Status
		if keeper.ErrorAs(err, &s) {
			fmt.Println("got error as google.golang.org/grpc/status.Status")
		}
	}

	// Output:
	// got error as google.golang.org/grpc/status.Status
}
