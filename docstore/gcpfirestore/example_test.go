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

package gcpfirestore_test

import (
	"context"
	"log"

	"gocloud.dev/docstore"
	"gocloud.dev/docstore/gcpfirestore"
	"gocloud.dev/gcp"
)

func ExampleOpenCollection() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		log.Fatal(err)
	}
	client, _, err := gcpfirestore.Dial(ctx, creds.TokenSource)
	if err != nil {
		log.Fatal(err)
	}
	resourceID := gcpfirestore.CollectionResourceID("my-project", "my-collection")
	coll, err := gcpfirestore.OpenCollection(client, resourceID, "userID", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer coll.Close()
}

func ExampleOpenCollectionWithNameFunc() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()
	type HighScore struct {
		Game   string
		Player string
	}

	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		log.Fatal(err)
	}
	client, _, err := gcpfirestore.Dial(ctx, creds.TokenSource)
	if err != nil {
		log.Fatal(err)
	}

	// The name of a document is constructed from the Game and Player fields.
	nameFromDocument := func(doc docstore.Document) string {
		hs := doc.(*HighScore)
		return hs.Game + "|" + hs.Player
	}

	resourceID := gcpfirestore.CollectionResourceID("my-project", "my-collection")
	coll, err := gcpfirestore.OpenCollectionWithNameFunc(client, resourceID, nameFromDocument, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer coll.Close()
}

func Example_openCollectionFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/docstore/gcpfirestore"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// docstore.OpenCollection creates a *docstore.Collection from a URL.
	const url = "firestore://projects/my-project/databases/(default)/documents/my-collection?name_field=userID"
	coll, err := docstore.OpenCollection(ctx, url)
	if err != nil {
		log.Fatal(err)
	}
	defer coll.Close()
}
