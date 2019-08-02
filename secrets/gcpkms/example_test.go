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

package gcpkms_test

import (
	"context"
	"log"

	"gocloud.dev/secrets"
	"gocloud.dev/secrets/gcpkms"
)

func ExampleOpenKeeper() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// Get a client to use with the KMS API.
	client, done, err := gcpkms.Dial(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
	// Close the connection when done.
	defer done()

	// You can also use gcpkms.KeyResourceID to construct this string.
	const keyID = "projects/MYPROJECT/" +
		"locations/MYLOCATION/" +
		"keyRings/MYKEYRING/" +
		"cryptoKeys/MYKEY"

	// Construct a *secrets.Keeper.
	keeper := gcpkms.OpenKeeper(client, keyID, nil)
	defer keeper.Close()
}

func Example_openFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/secrets/gcpkms"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	keeper, err := secrets.OpenKeeper(ctx,
		"gcpkms://projects/MYPROJECT/"+
			"locations/MYLOCATION/"+
			"keyRings/MYKEYRING/"+
			"cryptoKeys/MYKEY")
	if err != nil {
		log.Fatal(err)
	}
	defer keeper.Close()
}
